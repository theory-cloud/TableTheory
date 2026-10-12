package releasestate

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v4/pkg/core"
	theorydbErrors "github.com/theory-cloud/tabletheory/v4/pkg/errors"
)

type transitionActual struct {
	PK string `theorydb:"pk"`
	SK string `theorydb:"sk"`
}

type transitionEvent struct {
	PK string `theorydb:"pk"`
	SK string `theorydb:"sk"`
}

type transitionOutbox struct {
	PK string `theorydb:"pk"`
	SK string `theorydb:"sk"`
}

type transitionNoPartition struct {
	SK string `theorydb:"sk"`
}

type transitionNonStringPartition struct {
	SK string `theorydb:"sk"`
	PK int64  `theorydb:"pk"`
}

type transitionIndexedActual struct {
	PK   string `theorydb:"pk"`
	GSI1 string `theorydb:"index:gsi1,pk"`
	SK   string `theorydb:"sk"`
}

type transitionIndexOnly struct {
	GSI1 string `theorydb:"index:gsi1,pk"`
	SK   string `theorydb:"sk"`
}

type fakeTransactionWriter struct {
	builder *fakeTransactionBuilder
	called  bool
}

func (w *fakeTransactionWriter) TransactWrite(ctx context.Context, fn func(core.TransactionBuilder) error) error {
	w.called = true
	if ctx == nil {
		return errors.New("context is required")
	}
	if w.builder == nil {
		w.builder = &fakeTransactionBuilder{}
	}
	return fn(w.builder)
}

type fakeTransactionBuilder struct {
	updateModel any
	createModel any
	updateFn    func(core.UpdateBuilder) error
	ops         []string
}

func (b *fakeTransactionBuilder) Put(model any, conditions ...core.TransactCondition) core.TransactionBuilder {
	b.ops = append(b.ops, "put")
	return b
}

func (b *fakeTransactionBuilder) Create(model any, conditions ...core.TransactCondition) core.TransactionBuilder {
	b.ops = append(b.ops, "create")
	b.createModel = model
	return b
}

func (b *fakeTransactionBuilder) Update(model any, fields []string, conditions ...core.TransactCondition) core.TransactionBuilder {
	b.ops = append(b.ops, "update")
	return b
}

func (b *fakeTransactionBuilder) UpdateWithBuilder(model any, updateFn func(core.UpdateBuilder) error, conditions ...core.TransactCondition) core.TransactionBuilder {
	b.ops = append(b.ops, "update_builder")
	b.updateModel = model
	b.updateFn = updateFn
	return b
}

func (b *fakeTransactionBuilder) Delete(model any, conditions ...core.TransactCondition) core.TransactionBuilder {
	b.ops = append(b.ops, "delete")
	return b
}

func (b *fakeTransactionBuilder) ConditionCheck(model any, conditions ...core.TransactCondition) core.TransactionBuilder {
	b.ops = append(b.ops, "condition")
	return b
}

func (b *fakeTransactionBuilder) WithContext(ctx context.Context) core.TransactionBuilder { return b }
func (b *fakeTransactionBuilder) Execute() error                                          { return nil }
func (b *fakeTransactionBuilder) ExecuteWithContext(ctx context.Context) error            { return nil }

type fakeUpdateBuilder struct {
	sets           map[string]any
	adds           map[string]any
	versionChecked *int64
}

func newFakeUpdateBuilder() *fakeUpdateBuilder {
	return &fakeUpdateBuilder{sets: make(map[string]any), adds: make(map[string]any)}
}

func (b *fakeUpdateBuilder) Set(field string, value any) core.UpdateBuilder {
	b.sets[field] = value
	return b
}

func (b *fakeUpdateBuilder) SetIfNotExists(field string, value any, defaultValue any) core.UpdateBuilder {
	return b
}

func (b *fakeUpdateBuilder) Add(field string, value any) core.UpdateBuilder {
	b.adds[field] = value
	return b
}

func (b *fakeUpdateBuilder) Increment(field string) core.UpdateBuilder { return b.Add(field, 1) }
func (b *fakeUpdateBuilder) Decrement(field string) core.UpdateBuilder { return b.Add(field, -1) }
func (b *fakeUpdateBuilder) Remove(field string) core.UpdateBuilder    { return b }
func (b *fakeUpdateBuilder) Delete(field string, value any) core.UpdateBuilder {
	return b
}
func (b *fakeUpdateBuilder) AppendToList(field string, values any) core.UpdateBuilder  { return b }
func (b *fakeUpdateBuilder) PrependToList(field string, values any) core.UpdateBuilder { return b }
func (b *fakeUpdateBuilder) RemoveFromListAt(field string, index int) core.UpdateBuilder {
	return b
}
func (b *fakeUpdateBuilder) SetListElement(field string, index int, value any) core.UpdateBuilder {
	return b
}
func (b *fakeUpdateBuilder) Condition(field string, operator string, value any) core.UpdateBuilder {
	return b
}
func (b *fakeUpdateBuilder) OrCondition(field string, operator string, value any) core.UpdateBuilder {
	return b
}
func (b *fakeUpdateBuilder) ConditionExists(field string) core.UpdateBuilder    { return b }
func (b *fakeUpdateBuilder) ConditionNotExists(field string) core.UpdateBuilder { return b }
func (b *fakeUpdateBuilder) ConditionVersion(currentVersion int64) core.UpdateBuilder {
	b.versionChecked = &currentVersion
	return b
}
func (b *fakeUpdateBuilder) ReturnValues(option string) core.UpdateBuilder { return b }
func (b *fakeUpdateBuilder) Execute() error                                { return nil }
func (b *fakeUpdateBuilder) ExecuteWithResult(result any) error            { return nil }

func TestTransitionAppendEventAddsTransactionalUpdateAndCreate(t *testing.T) {
	expectedVersion := int64(7)
	writer := &fakeTransactionWriter{builder: &fakeTransactionBuilder{}}

	err := TransitionAppendEvent(context.Background(), writer, TransitionAppendEventInput{
		Actual:          &transitionActual{PK: "RELEASE#svc", SK: "ACTUAL"},
		Event:           &transitionEvent{PK: "RELEASE#svc", SK: "EVENT#1"},
		Set:             map[string]any{"status": "active", "previousReleaseId": "rel_001"},
		ExpectedVersion: &expectedVersion,
	})
	require.NoError(t, err)
	require.True(t, writer.called)
	require.Equal(t, []string{"update_builder", "create"}, writer.builder.ops)

	ub := newFakeUpdateBuilder()
	require.NoError(t, writer.builder.updateFn(ub))
	require.Equal(t, "active", ub.sets["status"])
	require.Equal(t, "rel_001", ub.sets["previousReleaseId"])
	require.Equal(t, int64(1), ub.adds["version"])
	require.NotNil(t, ub.versionChecked)
	require.Equal(t, expectedVersion, *ub.versionChecked)
}

func TestAddTransitionAppendEventValidatesInput(t *testing.T) {
	err := TransitionAppendEvent(context.Background(), nil, TransitionAppendEventInput{})
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)

	err = AddTransitionAppendEvent(&fakeTransactionBuilder{}, TransitionAppendEventInput{})
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)

	err = AddTransitionAppendEvent(&fakeTransactionBuilder{}, TransitionAppendEventInput{
		Actual: &transitionActual{PK: "RELEASE#svc", SK: "ACTUAL"},
		Event:  &transitionEvent{PK: "RELEASE#svc", SK: "EVENT#1"},
	})
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidOperator)

	err = AddTransitionAppendEvent(&fakeTransactionBuilder{}, TransitionAppendEventInput{
		Actual: &transitionActual{PK: "RELEASE#svc", SK: "ACTUAL"},
		Event:  &transitionEvent{PK: "RELEASE#svc", SK: "EVENT#1"},
		Set:    map[string]any{"version": 2},
	})
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)
}

func TestTransitionAppendEventRejectsMismatchedEventPartition(t *testing.T) {
	writer := &fakeTransactionWriter{builder: &fakeTransactionBuilder{}}

	err := TransitionAppendEvent(context.Background(), writer, TransitionAppendEventInput{
		Actual: &transitionActual{PK: "RELEASE#service-a", SK: "ACTUAL"},
		Event:  &transitionEvent{PK: "RELEASE#service-b", SK: "EVENT#1"},
		Set:    map[string]any{"status": "active"},
	})
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)
	require.False(t, writer.called)
	require.Empty(t, writer.builder.ops)

	builder := &fakeTransactionBuilder{}
	err = AddTransitionAppendEvent(builder, TransitionAppendEventInput{
		Actual: &transitionActual{PK: "RELEASE#service-a", SK: "ACTUAL"},
		Event:  &transitionEvent{PK: "RELEASE#service-b", SK: "EVENT#1"},
		Set:    map[string]any{"status": "active"},
	})
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)
	require.Empty(t, builder.ops)
}

func TestTransitionAppendEventBindsEventToActualPartitionAcrossRetry(t *testing.T) {
	expectedVersion := int64(3)
	writer := &fakeTransactionWriter{builder: &fakeTransactionBuilder{}}
	input := TransitionAppendEventInput{
		Actual:          &transitionActual{PK: "RELEASE#service-a", SK: "ACTUAL"},
		Event:           &transitionEvent{PK: "RELEASE#service-a", SK: "EVENT#2026-04-24T19:05:00Z"},
		Set:             map[string]any{"status": "active"},
		ExpectedVersion: &expectedVersion,
	}

	require.NoError(t, TransitionAppendEvent(context.Background(), writer, input))
	require.Equal(t, []string{"update_builder", "create"}, writer.builder.ops)

	// A retry re-submits the same deterministic transaction, and the
	// optimistic-conflict condition is preserved.
	retry := &fakeTransactionWriter{builder: &fakeTransactionBuilder{}}
	require.NoError(t, TransitionAppendEvent(context.Background(), retry, input))
	require.Equal(t, writer.builder.ops, retry.builder.ops)

	builder := &fakeTransactionBuilder{}
	require.NoError(t, AddTransitionAppendEvent(builder, input))
	ub := newFakeUpdateBuilder()
	require.NoError(t, builder.updateFn(ub))
	require.NotNil(t, ub.versionChecked)
	require.Equal(t, expectedVersion, *ub.versionChecked)
}

func TestTransitionAppendEventIncludesOutboxInOneTransaction(t *testing.T) {
	writer := &fakeTransactionWriter{builder: &fakeTransactionBuilder{}}

	err := TransitionAppendEvent(context.Background(), writer, TransitionAppendEventInput{
		Actual: &transitionActual{PK: "RELEASE#service-a", SK: "ACTUAL"},
		Event:  &transitionEvent{PK: "RELEASE#service-a", SK: "EVENT#1"},
		Outbox: &transitionOutbox{PK: "RELEASE#service-a", SK: "OUTBOX#lambda-alias#rel_002"},
		Set:    map[string]any{"status": "active"},
	})
	require.NoError(t, err)
	require.True(t, writer.called)
	require.Equal(t, []string{"update_builder", "create", "create"}, writer.builder.ops)
}

func TestPartitionKeyValueFailsClosed(t *testing.T) {
	value, err := partitionKeyValue(&transitionActual{PK: "RELEASE#service-a", SK: "ACTUAL"})
	require.NoError(t, err)
	require.Equal(t, "RELEASE#service-a", value)

	_, err = partitionKeyValue(&transitionNoPartition{SK: "ACTUAL"})
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)

	_, err = partitionKeyValue(&transitionNonStringPartition{PK: 7, SK: "ACTUAL"})
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)

	var nilActual *transitionActual
	_, err = partitionKeyValue(nilActual)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)

	value, err = partitionKeyValue(&transitionIndexedActual{PK: "RELEASE#service-a", GSI1: "gsi-value"})
	require.NoError(t, err)
	require.Equal(t, "RELEASE#service-a", value)

	_, err = partitionKeyValue(&transitionIndexOnly{GSI1: "gsi-value"})
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)
}
