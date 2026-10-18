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
	input.Event.(*bindingEvent).ReleaseID = "rel_001"

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
	input.Actual.(*bindingActual).PinnedReleaseID = "rel_001"
	input.Event.(*bindingEvent).ReleaseID = "rel_001"
	input.EventBinding = map[string]string{"releaseId": "actual:pinnedReleaseId"}

	require.NoError(t, TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input))
}

func TestTransitionEventBindingSupportsNextVersion(t *testing.T) {
	expected := int64(4)
	input := bindingInput()
	input.ExpectedVersion = &expected
	input.Event.(*bindingEvent).Version = 5
	input.EventBinding = map[string]string{"version": "nextVersion"}

	require.NoError(t, TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input))

	input.Event.(*bindingEvent).Version = 4
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
