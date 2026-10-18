package releasestate

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	theorydbErrors "github.com/theory-cloud/tabletheory/v6/pkg/errors"
)

type bindingActual struct {
	PK              string `theorydb:"pk,attr:PK"`
	SK              string `theorydb:"sk,attr:SK"`
	PinnedReleaseID string `theorydb:"attr:pinnedReleaseId"`
	Status          string `theorydb:"attr:status"`
	Version         int64  `theorydb:"version,attr:version"`
}

type bindingEvent struct {
	PK        string `theorydb:"pk,attr:PK"`
	SK        string `theorydb:"sk,attr:SK"`
	ReleaseID string `theorydb:"attr:releaseId"`
	EventType string `theorydb:"attr:eventType"`
	Version   int64  `theorydb:"attr:version"`
}

type bindingOutbox struct {
	PK        string `theorydb:"pk,attr:PK"`
	SK        string `theorydb:"sk,attr:SK"`
	ReleaseID string `theorydb:"attr:releaseId"`
	Operation string `theorydb:"attr:operation"`
}

type bindingKeyedOutbox struct {
	OutboxID  string `theorydb:"pk,attr:outboxId"`
	SK        string `theorydb:"sk,attr:SK"`
	ReleaseID string `theorydb:"attr:releaseId"`
}

func bindingEventOf(t *testing.T, input TransitionAppendEventInput) *bindingEvent {
	t.Helper()
	event, ok := input.Event.(*bindingEvent)
	require.True(t, ok)
	return event
}

func bindingActualOf(t *testing.T, input TransitionAppendEventInput) *bindingActual {
	t.Helper()
	actual, ok := input.Actual.(*bindingActual)
	require.True(t, ok)
	return actual
}

func bindingInput() TransitionAppendEventInput {
	return TransitionAppendEventInput{
		Actual: &bindingActual{PK: "RELEASE#service-a", SK: "ACTUAL"},
		Event: &bindingEvent{
			PK:        "RELEASE#service-a",
			SK:        "EVENT#1",
			ReleaseID: "rel_002",
			EventType: "active",
		},
		Set: map[string]any{
			"pinnedReleaseId": "rel_002",
			"status":          "active",
		},
	}
}

func TestTransitionEventBindingAcceptsMatchingTransition(t *testing.T) {
	input := bindingInput()
	input.EventBinding = map[string]string{
		"releaseId": "set:pinnedReleaseId",
		"eventType": "set:status",
	}

	writer := &fakeTransactionWriter{}
	require.NoError(t, TransitionAppendEvent(context.Background(), writer, input))
	require.True(t, writer.called)
}

func TestTransitionEventBindingRejectsMismatchedTransition(t *testing.T) {
	input := bindingInput()
	input.EventBinding = map[string]string{"releaseId": "set:pinnedReleaseId"}
	bindingEventOf(t, input).ReleaseID = "rel_001"

	writer := &fakeTransactionWriter{}
	err := TransitionAppendEvent(context.Background(), writer, input)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)
	require.False(t, writer.called, "a mismatched event must not reach the transaction")
}

func TestTransitionEventBindingRejectsMissingAttribute(t *testing.T) {
	input := bindingInput()
	input.EventBinding = map[string]string{"missing": "set:pinnedReleaseId"}

	err := TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)
}

func TestTransitionEventBindingResolvesActualFields(t *testing.T) {
	input := bindingInput()
	bindingActualOf(t, input).PinnedReleaseID = "rel_001"
	bindingEventOf(t, input).ReleaseID = "rel_001"
	input.EventBinding = map[string]string{"releaseId": "actual:pinnedReleaseId"}

	require.NoError(t, TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input))
}

func TestTransitionEventBindingSupportsNextVersion(t *testing.T) {
	expected := int64(4)
	input := bindingInput()
	input.ExpectedVersion = &expected
	bindingEventOf(t, input).Version = 5
	input.EventBinding = map[string]string{"version": "nextVersion"}

	require.NoError(t, TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input))

	bindingEventOf(t, input).Version = 4
	err := TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)

	input.ExpectedVersion = nil
	err = TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)
}

func TestTransitionEventBindingRejectsUnknownSource(t *testing.T) {
	input := bindingInput()
	input.EventBinding = map[string]string{"releaseId": "policy:whatever"}

	err := TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)
}

func TestTransitionOutboxBindingRejectsMismatchedTransition(t *testing.T) {
	input := bindingInput()
	input.Outbox = &bindingOutbox{
		PK:        "RELEASE#service-a",
		SK:        "OUTBOX#1",
		ReleaseID: "rel_001",
	}
	input.OutboxBinding = map[string]string{"releaseId": "set:pinnedReleaseId"}

	writer := &fakeTransactionWriter{}
	err := TransitionAppendEvent(context.Background(), writer, input)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)
	require.False(t, writer.called)
}

func TestTransitionOutboxPartitionMustMatchActual(t *testing.T) {
	input := bindingInput()
	input.Outbox = &bindingOutbox{PK: "RELEASE#service-b", SK: "OUTBOX#1"}

	writer := &fakeTransactionWriter{}
	err := TransitionAppendEvent(context.Background(), writer, input)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)
	require.False(t, writer.called)
}

func TestTransitionOutboxWithDifferentKeyAttributeIsAllowed(t *testing.T) {
	input := bindingInput()
	input.Outbox = &bindingKeyedOutbox{
		OutboxID:  "outbox#1",
		SK:        "OUTBOX#1",
		ReleaseID: "rel_002",
	}
	input.OutboxBinding = map[string]string{"releaseId": "set:pinnedReleaseId"}

	writer := &fakeTransactionWriter{}
	require.NoError(t, TransitionAppendEvent(context.Background(), writer, input))
	require.True(t, writer.called)
}

func TestTransitionOutboxBindingWithoutOutboxIsRejected(t *testing.T) {
	input := bindingInput()
	input.OutboxBinding = map[string]string{"releaseId": "set:pinnedReleaseId"}

	err := TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)
}

type bindingNoPartitionOutbox struct {
	SK        string `theorydb:"sk,attr:SK"`
	Operation string `theorydb:"attr:operation"`
}

func TestTransitionBindingResolvesGoFieldNames(t *testing.T) {
	input := bindingInput()
	bindingActualOf(t, input).PinnedReleaseID = "rel_001"
	bindingEventOf(t, input).ReleaseID = "rel_001"
	input.EventBinding = map[string]string{"releaseId": "actual:PinnedReleaseID"}

	require.NoError(t, TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input))

	input.EventBinding = map[string]string{"releaseId": "actual:Missing"}
	err := TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)
}

func TestTransitionBindingAcceptsNumericEquivalence(t *testing.T) {
	expected := int64(4)
	input := bindingInput()
	input.ExpectedVersion = &expected
	input.Set["status"] = 5
	bindingEventOf(t, input).Version = 5
	input.EventBinding = map[string]string{"version": "set:status"}
	require.NoError(t, TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input))

	input.Set["status"] = uint(5)
	require.NoError(t, TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input))

	// Strings are not numeric identities: "5" and "007" must not satisfy an
	// integer row field, or a string-typed version could erode the binding.
	input.Set["status"] = "5"
	err := TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)

	input.Set["status"] = "not-a-number"
	err = TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)
}

func TestTransitionNextVersionRejectsNumericStringCoercion(t *testing.T) {
	type stringVersionEvent struct {
		PK      string `theorydb:"pk,attr:PK"`
		SK      string `theorydb:"sk,attr:SK"`
		Version string `theorydb:"attr:version"`
	}

	expected := int64(6)
	input := TransitionAppendEventInput{
		Actual:          &bindingActual{PK: "RELEASE#service-a", SK: "ACTUAL"},
		Event:           &stringVersionEvent{PK: "RELEASE#service-a", SK: "EVENT#1", Version: "007"},
		ExpectedVersion: &expected,
		Set:             map[string]any{"status": "active"},
		EventBinding:    map[string]string{"version": "nextVersion"},
	}

	err := TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel, `"007" must not satisfy nextVersion=7`)
}

func TestTransitionBindingEmptyMapDeclaresNothing(t *testing.T) {
	input := bindingInput()
	bindingEventOf(t, input).ReleaseID = "rel_999"
	bindingEventOf(t, input).EventType = "unrelated"
	input.EventBinding = map[string]string{}

	// An empty map is the explicit opt-out: no attribute is declared, so the
	// generically knowable checks (partition) still run and nothing else is
	// compared. A partial map checks exactly the attributes it names.
	require.NoError(t, TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input))

	input.EventBinding = map[string]string{"releaseId": "set:pinnedReleaseId"}
	err := TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)
}

func TestTransitionOutboxWithoutPartitionKeyIsRejected(t *testing.T) {
	input := bindingInput()
	input.Outbox = &bindingNoPartitionOutbox{SK: "OUTBOX#1", Operation: "flip"}

	err := TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)
}

func TestTransitionBindingValueComparison(t *testing.T) {
	type payload struct {
		A int `json:"a"`
	}
	require.True(t, bindingValuesEqual(payload{A: 1}, payload{A: 1}))
	require.False(t, bindingValuesEqual(payload{A: 1}, payload{A: 2}))
	require.True(t, bindingValuesEqual(1.5, 1.5))
	require.False(t, bindingValuesEqual(nil, 1))
}

func TestTransitionBindingRejectsNonStructRows(t *testing.T) {
	input := bindingInput()
	input.Actual = 42

	err := TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)
}

func TestTransitionBindingRejectsNilOutboxBindingWithoutRow(t *testing.T) {
	input := bindingInput()
	input.Outbox = nil
	input.OutboxBinding = map[string]string{"releaseId": "set:pinnedReleaseId"}

	err := TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)
}

func TestTransitionBindingMatchesFieldAliases(t *testing.T) {
	input := bindingInput()
	bindingEventOf(t, input).ReleaseID = "rel_002"
	input.EventBinding = map[string]string{"releaseId": "set:pinnedReleaseId"}
	require.NoError(t, TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input))

	delete(input.Set, "pinnedReleaseId")
	input.Set["PinnedReleaseID"] = "rel_002"
	input.EventBinding = map[string]string{"releaseId": "set:pinnedReleaseId"}
	require.NoError(t, TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input))

	input.EventBinding = map[string]string{"releaseId": "set:PinnedReleaseID"}
	require.NoError(t, TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input))
}

func TestBindingValuesEqualFloats(t *testing.T) {
	require.True(t, bindingValuesEqual(float32(1.5), 1.5))
}

type bindingEmbeddedActual struct {
	PK string `theorydb:"pk,attr:PK"`
	SK string `theorydb:"sk,attr:SK"`
	BindingEmbeddedFields
}

type BindingEmbeddedFields struct {
	PinnedReleaseID string `theorydb:"attr:pinnedReleaseId"`
}

func TestTransitionBindingReadsPromotedEmbeddedFields(t *testing.T) {
	input := TransitionAppendEventInput{
		Actual: &bindingEmbeddedActual{
			PK:                    "RELEASE#service-a",
			SK:                    "ACTUAL",
			BindingEmbeddedFields: BindingEmbeddedFields{PinnedReleaseID: "rel_002"},
		},
		Event: &bindingEvent{
			PK:        "RELEASE#service-a",
			SK:        "EVENT#1",
			ReleaseID: "rel_002",
		},
		Set:          map[string]any{"status": "active"},
		EventBinding: map[string]string{"releaseId": "actual:pinnedReleaseId"},
	}

	require.NoError(t, TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input))
}

type bindingGsiFirst struct {
	GsiPK string `theorydb:"index:gsi1,pk,attr:gsi1pk"`
	PK    string `theorydb:"pk,attr:PK"`
	SK    string `theorydb:"sk,attr:SK"`
}

func TestTransitionOutboxPartitionUsesTablePKNotIndexKey(t *testing.T) {
	input := bindingInput()
	input.Actual = &bindingGsiFirst{
		GsiPK: "RELEASE#service-a",
		PK:    "RELEASE#service-b",
		SK:    "ACTUAL",
	}
	// Keep the event row in the actual row's release partition so the
	// always-on event check passes; only the outbox partition is under test.
	bindingEventOf(t, input).PK = "RELEASE#service-b"
	input.Outbox = &bindingGsiFirst{
		GsiPK: "RELEASE#service-a",
		PK:    "RELEASE#service-z",
		SK:    "OUTBOX#1",
	}

	writer := &fakeTransactionWriter{}
	err := TransitionAppendEvent(context.Background(), writer, input)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel,
		"a matching index key must not stand in for the table partition")
	require.ErrorContains(t, err, "outbox release partition")
	require.False(t, writer.called)
}

type bindingJSONTagActual struct {
	PK              string `theorydb:"pk,attr:PK"`
	SK              string `theorydb:"sk,attr:SK"`
	PinnedReleaseID string `json:"pinnedReleaseId"`
}

func TestTransitionBindingUsesStoredAttributeNamesNotJSONTags(t *testing.T) {
	input := bindingInput()
	input.Actual = &bindingJSONTagActual{
		PK:              "RELEASE#service-a",
		SK:              "ACTUAL",
		PinnedReleaseID: "rel_002",
	}
	input.EventBinding = map[string]string{"releaseId": "actual:pinnedReleaseID"}

	require.NoError(t, TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input))

	input.EventBinding = map[string]string{"releaseId": "actual:pinnedReleaseId"}
	err := TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel,
		"a json tag is not a stored attribute name")
}

type bindingExcludedEmbeddedFields struct {
	DupPK           string `theorydb:"pk,attr:PK"`
	PinnedReleaseID string `theorydb:"attr:pinnedReleaseId"`
}

type bindingExcludedEmbeddedActual struct {
	PK       string                        `theorydb:"pk,attr:PK"`
	SK       string                        `theorydb:"sk,attr:SK"`
	Excluded bindingExcludedEmbeddedFields `theorydb:"-"`
}

func TestTransitionBindingHonorsEmbeddedExclusionTags(t *testing.T) {
	input := bindingInput()
	input.Actual = &bindingExcludedEmbeddedActual{
		PK:       "RELEASE#service-a",
		SK:       "ACTUAL",
		Excluded: bindingExcludedEmbeddedFields{PinnedReleaseID: "rel_002"},
	}
	input.EventBinding = map[string]string{"releaseId": "actual:pinnedReleaseId"}

	err := TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel,
		"an excluded embedded subtree is not part of the row")
}

type bindingDuplicateAttribute struct {
	PK    string `theorydb:"pk,attr:PK"`
	Other string `theorydb:"attr:PK"`
}

func TestDescribeBindingRowRejectsDuplicateAttributeNames(t *testing.T) {
	_, err := describeBindingRow(&bindingDuplicateAttribute{})
	require.ErrorIs(t, err, theorydbErrors.ErrDuplicatePrimaryKey)
}
