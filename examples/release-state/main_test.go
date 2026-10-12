package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v4/pkg/core"
)

// recordingTransactionWriter models DynamoDB TransactWriteItems: every
// operation belongs to one transaction that commits in full or not at all.
type recordingTransactionWriter struct {
	builder   *recordingTransactionBuilder
	failNext  error
	committed []string
}

func (w *recordingTransactionWriter) TransactWrite(_ context.Context, fn func(core.TransactionBuilder) error) error {
	if w.builder == nil {
		w.builder = &recordingTransactionBuilder{}
	}
	if err := fn(w.builder); err != nil {
		return err
	}
	if w.failNext != nil {
		return w.failNext
	}
	w.committed = append([]string(nil), w.builder.ops...)
	return nil
}

type recordingTransactionBuilder struct {
	ops []string
}

func (b *recordingTransactionBuilder) Put(any, ...core.TransactCondition) core.TransactionBuilder {
	b.ops = append(b.ops, "put")
	return b
}

func (b *recordingTransactionBuilder) Create(any, ...core.TransactCondition) core.TransactionBuilder {
	b.ops = append(b.ops, "create")
	return b
}

func (b *recordingTransactionBuilder) Update(any, []string, ...core.TransactCondition) core.TransactionBuilder {
	b.ops = append(b.ops, "update")
	return b
}

func (b *recordingTransactionBuilder) UpdateWithBuilder(any, func(core.UpdateBuilder) error, ...core.TransactCondition) core.TransactionBuilder {
	b.ops = append(b.ops, "update_builder")
	return b
}

func (b *recordingTransactionBuilder) Delete(any, ...core.TransactCondition) core.TransactionBuilder {
	b.ops = append(b.ops, "delete")
	return b
}

func (b *recordingTransactionBuilder) ConditionCheck(any, ...core.TransactCondition) core.TransactionBuilder {
	b.ops = append(b.ops, "condition")
	return b
}

func (b *recordingTransactionBuilder) WithContext(context.Context) core.TransactionBuilder {
	return b
}

func (b *recordingTransactionBuilder) Execute() error { return nil }

func (b *recordingTransactionBuilder) ExecuteWithContext(context.Context) error { return nil }

func promoteCommand() PromoteCommand {
	return PromoteCommand{
		Service:         "service-a",
		ReleaseID:       "rel_002",
		PreviousRelease: "rel_001",
		Actor:           "operator@example.com",
		ExpectedVersion: 7,
		ObservedAt:      time.Date(2026, 4, 24, 19, 0, 0, 0, time.UTC),
	}
}

func TestPromoteReleaseCommitsActualEventAndOutboxInOneTransaction(t *testing.T) {
	writer := &recordingTransactionWriter{}

	require.NoError(t, promoteRelease(context.Background(), writer, promoteCommand()))
	require.Equal(t, []string{"update_builder", "create", "create"}, writer.committed)
}

func TestPromoteReleaseInjectedFailureLeavesNoPartialState(t *testing.T) {
	writer := &recordingTransactionWriter{failNext: errors.New("TransactionCanceledException")}

	require.Error(t, promoteRelease(context.Background(), writer, promoteCommand()))
	require.Empty(t, writer.committed)
}
