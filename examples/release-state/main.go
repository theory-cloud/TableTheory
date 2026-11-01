package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"

	"github.com/theory-cloud/tabletheory/v6"
	"github.com/theory-cloud/tabletheory/v6/pkg/core"
	"github.com/theory-cloud/tabletheory/v6/pkg/model"
	"github.com/theory-cloud/tabletheory/v6/pkg/releasestate"
	"github.com/theory-cloud/tabletheory/v6/pkg/session"
)

// releaseStateTable is runtime-specific so the Go, TypeScript and Python rows
// can run against one harness database without sharing seed state. Their
// transition assertions and printed parity line are identical.
const releaseStateTable = "release_state_contract_go"

// ReleaseStateActual is the mutable current-state row for one deployable
// scope. Registry pin fields are protected at the model level, so generic
// update/save helpers cannot mutate them accidentally.
type ReleaseStateActual struct {
	PK                string         `theorydb:"pk,attr:PK" json:"PK"`
	SK                string         `theorydb:"sk,attr:SK" json:"SK"`
	Service           string         `theorydb:"attr:service" json:"service"`
	Status            string         `theorydb:"attr:status" json:"status"`
	PinnedReleaseID   string         `theorydb:"attr:pinnedReleaseId" json:"pinnedReleaseId"`
	PreviousReleaseID string         `theorydb:"attr:previousReleaseId" json:"previousReleaseId,omitempty"`
	Provenance        map[string]any `theorydb:"attr:provenance" json:"provenance,omitempty"`
	Confidence        map[string]any `theorydb:"attr:confidence" json:"confidence,omitempty"`
	Version           int64          `theorydb:"version,attr:version" json:"version"`
}

func (ReleaseStateActual) WritePolicy() model.WritePolicy {
	return model.WritePolicy{
		Mode:                model.WritePolicyModeMutable,
		ProtectedAttributes: []string{"pinnedReleaseId"},
	}
}

// TableName keeps the actual, event and outbox rows in one parity table shared
// with the TypeScript and Python drivers.
func (ReleaseStateActual) TableName() string { return releaseStateTable }

// ReleaseStateEvent is immutable audit history. TableTheory's write_once
// policy lets callers create this row but rejects generic update/save/delete.
type ReleaseStateEvent struct {
	PK        string         `theorydb:"pk,attr:PK" json:"PK"`
	SK        string         `theorydb:"sk,attr:SK" json:"SK"`
	Service   string         `theorydb:"attr:service" json:"service"`
	ReleaseID string         `theorydb:"attr:releaseId" json:"releaseId"`
	EventType string         `theorydb:"attr:eventType" json:"eventType"`
	At        string         `theorydb:"attr:at" json:"at"`
	Actor     string         `theorydb:"attr:actor" json:"actor"`
	Evidence  map[string]any `theorydb:"attr:evidence" json:"evidence,omitempty"`
}

func (ReleaseStateEvent) WritePolicy() model.WritePolicy {
	return model.WritePolicy{Mode: model.WritePolicyModeWriteOnce}
}

// TableName keeps the actual, event and outbox rows in one parity table.
func (ReleaseStateEvent) TableName() string { return releaseStateTable }

// ReleaseStateOutbox is also immutable. Workers consume these rows to perform
// non-DynamoDB side effects, then append success/failure events.
type ReleaseStateOutbox struct {
	PK             string `theorydb:"pk,attr:PK" json:"PK"`
	SK             string `theorydb:"sk,attr:SK" json:"SK"`
	Operation      string `theorydb:"attr:operation" json:"operation"`
	IdempotencyKey string `theorydb:"attr:idempotencyKey" json:"idempotencyKey"`
	RequestedState string `theorydb:"attr:requestedState" json:"requestedState"`
	NextAttemptAt  string `theorydb:"attr:nextAttemptAt" json:"nextAttemptAt"`
}

func (ReleaseStateOutbox) WritePolicy() model.WritePolicy {
	return model.WritePolicy{Mode: model.WritePolicyModeWriteOnce}
}

// TableName keeps the actual, event and outbox rows in one parity table.
func (ReleaseStateOutbox) TableName() string { return releaseStateTable }

type PromoteCommand struct {
	Service         string
	ReleaseID       string
	PreviousRelease string
	Actor           string
	ExpectedVersion int64
	ObservedAt      time.Time
}

func main() {
	cmd := PromoteCommand{
		Service:         "service-a",
		ReleaseID:       "rel_002",
		PreviousRelease: "rel_001",
		Actor:           "operator@example.com",
		ExpectedVersion: 7,
		ObservedAt:      time.Date(2026, 4, 24, 19, 0, 0, 0, time.UTC),
	}

	db := newDB()
	for _, model := range []any{
		&ReleaseStateActual{},
		&ReleaseStateEvent{},
		&ReleaseStateOutbox{},
	} {
		if err := db.EnsureTable(model); err != nil {
			log.Fatalf("ensure release-state table for %T: %v", model, err)
		}
	}

	actual := actualKey(cmd.Service)
	actual.Status = "idle"
	actual.Version = cmd.ExpectedVersion
	if err := db.Model(&actual).Create(); err != nil {
		log.Fatalf("seed actual-state row: %v", err)
	}

	ctx := context.Background()
	if err := promoteRelease(ctx, db, cmd); err != nil {
		log.Fatalf("promoteRelease: %v", err)
	}

	stored, err := readActual(db, actual.PK)
	if err != nil {
		log.Fatalf("read actual-state row: %v", err)
	}
	if stored.Status != "active" || stored.PreviousReleaseID != cmd.PreviousRelease {
		log.Fatalf("actual state = %+v, want status=active previousReleaseId=%s", stored, cmd.PreviousRelease)
	}
	if stored.Version != cmd.ExpectedVersion+1 {
		log.Fatalf("actual version = %d, want %d", stored.Version, cmd.ExpectedVersion+1)
	}

	eventSK := fmt.Sprintf("EVENT#%s#%s", cmd.ObservedAt.Format(time.RFC3339), cmd.ReleaseID)
	events, err := readEvents(db, actual.PK, eventSK)
	if err != nil {
		log.Fatalf("read event row: %v", err)
	}
	if len(events) != 1 || events[0].EventType != "promoted" || events[0].ReleaseID != cmd.ReleaseID {
		log.Fatalf("event rows = %+v, want one promoted row for %s", events, cmd.ReleaseID)
	}

	outboxSK := fmt.Sprintf("OUTBOX#lambda-alias#%s", cmd.ReleaseID)
	outboxes, err := readOutboxes(db, actual.PK, outboxSK)
	if err != nil {
		log.Fatalf("read outbox row: %v", err)
	}
	if len(outboxes) != 1 || outboxes[0].Operation != "lambda_alias_update" {
		log.Fatalf("outbox rows = %+v, want one lambda_alias_update row", outboxes)
	}

	// A stale optimistic-lock version must be rejected by the same transaction
	// path, proving the condition is live rather than decorative.
	if err := promoteRelease(ctx, db, cmd); err == nil {
		log.Fatal("stale promoteRelease succeeded, want a condition failure")
	}

	fmt.Printf(
		"release-state: actual=%s/%s version=%d status=%s event=%s outbox=%s\n",
		stored.PK, stored.SK, stored.Version, stored.Status, eventSK, outboxSK,
	)
}

func newDB() core.ExtendedDB {
	cfg := session.Config{
		Region:   "us-east-1",
		Endpoint: localEndpoint(),
		AWSConfigOptions: []func(*config.LoadOptions) error{
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")),
		},
	}
	db, err := tabletheory.New(cfg)
	if err != nil {
		log.Fatalf("failed to create TableTheory client: %v", err)
	}
	return db
}

func readActual(db core.ExtendedDB, partitionKey string) (ReleaseStateActual, error) {
	var rows []ReleaseStateActual
	if err := db.Model(&ReleaseStateActual{}).
		Where("PK", "=", partitionKey).
		Where("SK", "=", "ACTUAL").
		All(&rows); err != nil {
		return ReleaseStateActual{}, err
	}
	if len(rows) != 1 {
		return ReleaseStateActual{}, fmt.Errorf("actual-state rows = %d, want 1", len(rows))
	}
	return rows[0], nil
}

func readEvents(db core.ExtendedDB, partitionKey, sk string) ([]ReleaseStateEvent, error) {
	var rows []ReleaseStateEvent
	if err := db.Model(&ReleaseStateEvent{}).
		Where("PK", "=", partitionKey).
		Where("SK", "=", sk).
		All(&rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func readOutboxes(db core.ExtendedDB, partitionKey, sk string) ([]ReleaseStateOutbox, error) {
	var rows []ReleaseStateOutbox
	if err := db.Model(&ReleaseStateOutbox{}).
		Where("PK", "=", partitionKey).
		Where("SK", "=", sk).
		All(&rows); err != nil {
		return nil, err
	}
	return rows, nil
}

// localEndpoint honours the local harness endpoint while keeping the historical
// localhost:8000 default for a hand-started DynamoDB Local.
func localEndpoint() string {
	for _, key := range []string{"DYNAMODB_ENDPOINT", "AWS_ENDPOINT_URL_DYNAMODB"} {
		if endpoint := strings.TrimSpace(os.Getenv(key)); endpoint != "" {
			return endpoint
		}
	}
	return "http://localhost:8000"
}

// promoteRelease shows the transactional TableTheory write. The actual-state
// update, event append, and the conditional outbox create commit together;
// Lambda alias or CodePipeline side effects must be driven separately from
// outbox/reconciliation.
func promoteRelease(ctx context.Context, db releasestate.TransactionWriter, cmd PromoteCommand) error {
	provenance, confidence := deployAuthorityMetadata(cmd)
	if err := releasestate.ValidateDeployAuthorityMetadata(map[string]any{
		"provenance": provenance,
		"confidence": confidence,
	}); err != nil {
		return err
	}

	expected := cmd.ExpectedVersion
	return releasestate.TransitionAppendEvent(ctx, db, releasestate.TransitionAppendEventInput{
		Actual:          actualKey(cmd.Service),
		Event:           releaseEvent(cmd, provenance),
		Outbox:          aliasOutbox(cmd),
		ExpectedVersion: &expected,
		Set: map[string]any{
			"status":            "active",
			"previousReleaseId": cmd.PreviousRelease,
			"provenance":        provenance,
			"confidence":        confidence,
		},
	})
}

func actualKey(service string) ReleaseStateActual {
	return ReleaseStateActual{PK: "RELEASE#" + service, SK: "ACTUAL", Service: service}
}

func releaseEvent(cmd PromoteCommand, provenance map[string]any) ReleaseStateEvent {
	return ReleaseStateEvent{
		PK:        "RELEASE#" + cmd.Service,
		SK:        "EVENT#" + cmd.ObservedAt.Format(time.RFC3339) + "#" + cmd.ReleaseID,
		Service:   cmd.Service,
		ReleaseID: cmd.ReleaseID,
		EventType: "promoted",
		At:        cmd.ObservedAt.Format(time.RFC3339),
		Actor:     cmd.Actor,
		Evidence:  provenance,
	}
}

func aliasOutbox(cmd PromoteCommand) ReleaseStateOutbox {
	return ReleaseStateOutbox{
		PK:             "RELEASE#" + cmd.Service,
		SK:             "OUTBOX#lambda-alias#" + cmd.ReleaseID,
		Operation:      "lambda_alias_update",
		IdempotencyKey: cmd.Service + ":" + cmd.ReleaseID,
		RequestedState: "active",
		NextAttemptAt:  cmd.ObservedAt.Format(time.RFC3339),
	}
}

func deployAuthorityMetadata(cmd PromoteCommand) (map[string]any, map[string]any) {
	ref := "operator://deploy/" + cmd.Service + "/" + cmd.ReleaseID
	observedAt := cmd.ObservedAt.Format(time.RFC3339)
	recordedAt := cmd.ObservedAt.Add(time.Second).Format(time.RFC3339)

	provenance := map[string]any{
		"mode":        "native",
		"system":      "release-control-plane",
		"kind":        "operator_command",
		"ref":         ref,
		"observed_at": observedAt,
		"recorded_at": recordedAt,
		"evidence": []any{
			map[string]any{
				"kind":        "operator_command",
				"source":      "release-control-plane",
				"ref":         ref,
				"observed_at": observedAt,
			},
		},
	}
	confidence := map[string]any{
		"level":   "high",
		"reasons": []any{"operator_command_authority"},
	}
	return provenance, confidence
}
