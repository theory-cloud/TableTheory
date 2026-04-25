// Package releasestate contains opt-in helpers for release-state registry
// records. The helpers compose existing TableTheory write-policy and
// transaction primitives; they do not weaken model-level immutability or
// protected-field enforcement.
package releasestate

import (
	"context"
	"fmt"

	"github.com/theory-cloud/tabletheory/pkg/core"
	theorydbErrors "github.com/theory-cloud/tabletheory/pkg/errors"
)

const defaultVersionField = "version"

// TransactionWriter is the minimal TableTheory surface required to execute a
// release-state transition. A single DynamoDB TransactWriteItems call is used
// for the actual-state update and event-history append.
//
// External side effects such as Lambda alias flips or CodePipeline executions
// are intentionally outside this helper's atomicity boundary. Callers should
// pair those side effects with explicit retry/reconciliation/outbox behavior.
type TransactionWriter interface {
	TransactWrite(context.Context, func(core.TransactionBuilder) error) error
}

// TransitionAppendEventInput describes one release-state transition:
// update the mutable actual-state row and append one immutable event-history
// row in the same DynamoDB transaction.
type TransitionAppendEventInput struct {
	// Actual is the model instance containing the actual-state row key.
	Actual any
	// Event is the model instance to append to event history. Write-once event
	// models remain protected by their model-level WritePolicy.
	Event any
	// Set contains actual-state attributes to SET during the transition. Keys
	// may be Go field names or DynamoDB attribute names accepted by
	// core.UpdateBuilder.
	Set map[string]any
	// ExpectedVersion, when non-nil, adds an optimistic-lock condition against
	// the model's theorydb:"version" field before incrementing it.
	ExpectedVersion *int64
	// VersionField names the version attribute to increment. Empty defaults to
	// "version", matching the release-state contract fixture.
	VersionField string
}

// TransitionAppendEvent executes a release-state transition with the supplied
// transaction writer.
func TransitionAppendEvent(ctx context.Context, db TransactionWriter, input TransitionAppendEventInput) error {
	if db == nil {
		return fmt.Errorf("%w: release-state transaction writer is required", theorydbErrors.ErrInvalidModel)
	}
	return db.TransactWrite(ctx, func(tx core.TransactionBuilder) error {
		return AddTransitionAppendEvent(tx, input)
	})
}

// AddTransitionAppendEvent adds the release-state actual-row transition and
// event append to an existing transaction builder. Callers that need to
// transactionally compose additional internal DynamoDB writes can use this
// helper before executing the builder.
func AddTransitionAppendEvent(tx core.TransactionBuilder, input TransitionAppendEventInput) error {
	if tx == nil {
		return fmt.Errorf("%w: transaction builder is required", theorydbErrors.ErrInvalidModel)
	}
	if input.Actual == nil {
		return fmt.Errorf("%w: actual model is required", theorydbErrors.ErrInvalidModel)
	}
	if input.Event == nil {
		return fmt.Errorf("%w: event model is required", theorydbErrors.ErrInvalidModel)
	}
	if len(input.Set) == 0 {
		return fmt.Errorf("%w: transition set is required", theorydbErrors.ErrInvalidOperator)
	}

	versionField := input.VersionField
	if versionField == "" {
		versionField = defaultVersionField
	}
	if _, ok := input.Set[versionField]; ok {
		return fmt.Errorf("%w: transition set must not mutate version directly", theorydbErrors.ErrInvalidModel)
	}

	tx.UpdateWithBuilder(input.Actual, func(ub core.UpdateBuilder) error {
		for field, value := range input.Set {
			ub.Set(field, value)
		}
		ub.Add(versionField, int64(1))
		if input.ExpectedVersion != nil {
			ub.ConditionVersion(*input.ExpectedVersion)
		}
		return nil
	}).Create(input.Event)

	return nil
}
