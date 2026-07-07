package main

import (
	"context"
	"fmt"
	"time"

	"github.com/theory-cloud/tabletheory/v2/pkg/model"
	"github.com/theory-cloud/tabletheory/v2/pkg/releasestate"
)

// ReleaseStateActual is the mutable current-state row for one deployable
// scope. Registry pin fields are protected at the model level, so generic
// update/save helpers cannot mutate them accidentally.
type ReleaseStateActual struct {
	PK                string         `theorydb:"pk" json:"PK"`
	SK                string         `theorydb:"sk" json:"SK"`
	Service           string         `json:"service"`
	Status            string         `json:"status"`
	PinnedReleaseID   string         `json:"pinnedReleaseId"`
	PreviousReleaseID string         `json:"previousReleaseId,omitempty"`
	Provenance        map[string]any `json:"provenance,omitempty"`
	Confidence        map[string]any `json:"confidence,omitempty"`
	Version           int64          `theorydb:"version" json:"version"`
}

func (ReleaseStateActual) WritePolicy() model.WritePolicy {
	return model.WritePolicy{
		Mode:                model.WritePolicyModeMutable,
		ProtectedAttributes: []string{"pinnedReleaseId"},
	}
}

// ReleaseStateEvent is immutable audit history. TableTheory's write_once
// policy lets callers create this row but rejects generic update/save/delete.
type ReleaseStateEvent struct {
	PK        string         `theorydb:"pk" json:"PK"`
	SK        string         `theorydb:"sk" json:"SK"`
	Service   string         `json:"service"`
	ReleaseID string         `json:"releaseId"`
	EventType string         `json:"eventType"`
	At        string         `json:"at"`
	Actor     string         `json:"actor"`
	Evidence  map[string]any `json:"evidence,omitempty"`
}

func (ReleaseStateEvent) WritePolicy() model.WritePolicy {
	return model.WritePolicy{Mode: model.WritePolicyModeWriteOnce}
}

// ReleaseStateOutbox is also immutable. Workers consume these rows to perform
// non-DynamoDB side effects, then append success/failure events.
type ReleaseStateOutbox struct {
	PK             string `theorydb:"pk" json:"PK"`
	SK             string `theorydb:"sk" json:"SK"`
	Operation      string `json:"operation"`
	IdempotencyKey string `json:"idempotencyKey"`
	RequestedState string `json:"requestedState"`
	NextAttemptAt  string `json:"nextAttemptAt"`
}

func (ReleaseStateOutbox) WritePolicy() model.WritePolicy {
	return model.WritePolicy{Mode: model.WritePolicyModeWriteOnce}
}

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

	provenance, confidence := deployAuthorityMetadata(cmd)
	if err := releasestate.ValidateDeployAuthorityMetadata(map[string]any{
		"provenance": provenance,
		"confidence": confidence,
	}); err != nil {
		panic(err)
	}

	actual := actualKey(cmd.Service)
	event := releaseEvent(cmd, provenance)
	outbox := aliasOutbox(cmd)

	fmt.Printf("actual=%s/%s event=%s outbox=%s\n", actual.PK, actual.SK, event.SK, outbox.SK)
}

// promoteRelease shows the transactional TableTheory write. The actual-state
// update and event append commit together; Lambda alias or CodePipeline side
// effects must be driven separately from outbox/reconciliation.
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
