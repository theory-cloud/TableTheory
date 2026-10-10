// Package releasestate contains opt-in helpers for release-state registry
// records. The helpers compose existing TableTheory write-policy and
// transaction primitives; they do not weaken model-level immutability or
// protected-field enforcement.
package releasestate

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/theory-cloud/tabletheory/v4/pkg/core"
	theorydbErrors "github.com/theory-cloud/tabletheory/v4/pkg/errors"
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
	// Outbox, when non-nil, is an immutable write-once outbox row created in the
	// same transaction as the actual-state update and event append. Callers use
	// it to record non-DynamoDB side-effect intent atomically with the release
	// state that authorizes it.
	Outbox any
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
	if err := validateTransitionInput(input); err != nil {
		return err
	}
	return db.TransactWrite(ctx, func(tx core.TransactionBuilder) error {
		return AddTransitionAppendEvent(tx, input)
	})
}

func validateTransitionInput(input TransitionAppendEventInput) error {
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

	return validateEventBindsToActual(input.Actual, input.Event)
}

// AddTransitionAppendEvent adds the release-state actual-row transition and
// event append to an existing transaction builder. Callers that need to
// transactionally compose additional internal DynamoDB writes can use this
// helper before executing the builder.
func AddTransitionAppendEvent(tx core.TransactionBuilder, input TransitionAppendEventInput) error {
	if tx == nil {
		return fmt.Errorf("%w: transaction builder is required", theorydbErrors.ErrInvalidModel)
	}
	if err := validateTransitionInput(input); err != nil {
		return err
	}

	versionField := input.VersionField
	if versionField == "" {
		versionField = defaultVersionField
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

	if input.Outbox != nil {
		tx.Create(input.Outbox)
	}

	return nil
}

// validateEventBindsToActual rejects an event whose partition-key value does
// not match the actual-state row the transition updates, so an event can only
// ever describe the exact release partition that changed.
func validateEventBindsToActual(actual any, event any) error {
	actualPartition, err := partitionKeyValue(actual)
	if err != nil {
		return err
	}
	eventPartition, err := partitionKeyValue(event)
	if err != nil {
		return err
	}
	if actualPartition != eventPartition {
		return fmt.Errorf(
			"%w: event release partition %q does not match actual row partition %q",
			theorydbErrors.ErrInvalidModel, eventPartition, actualPartition,
		)
	}
	return nil
}

var partitionKeyFields sync.Map // reflect.Type -> []int

// partitionKeyValue reads a model's DynamoDB partition-key value. It fails
// closed when the model defines no top-level string partition key.
func partitionKeyValue(modelInstance any) (string, error) {
	value := reflect.ValueOf(modelInstance)
	for value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return "", fmt.Errorf("%w: release-state row must not be a nil pointer", theorydbErrors.ErrInvalidModel)
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return "", fmt.Errorf("%w: release-state row must be a struct", theorydbErrors.ErrInvalidModel)
	}

	path, found := partitionKeyFieldPath(value.Type())
	if !found {
		return "", fmt.Errorf("%w: release-state row partition key is required", theorydbErrors.ErrInvalidModel)
	}

	field := value.FieldByIndex(path)
	if field.Kind() != reflect.String {
		return "", fmt.Errorf("%w: release-state partition key must be a string", theorydbErrors.ErrInvalidModel)
	}
	return field.String(), nil
}

func partitionKeyFieldPath(modelType reflect.Type) ([]int, bool) {
	if cached, ok := partitionKeyFields.Load(modelType); ok {
		if path, isPath := cached.([]int); isPath {
			return path, true
		}
	}
	for i := 0; i < modelType.NumField(); i++ {
		field := modelType.Field(i)
		if !field.IsExported() {
			continue
		}
		if !hasPrimaryKeyTag(field.Tag.Get("theorydb")) {
			continue
		}
		path := []int{i}
		partitionKeyFields.Store(modelType, path)
		return path, true
	}
	return nil, false
}

// hasPrimaryKeyTag reports whether a theorydb tag marks the table partition
// key. Modifiers that belong to an index/lsi clause (for example
// `index:gsi1,pk`) do not mark the table partition key.
func hasPrimaryKeyTag(tag string) bool {
	inIndexClause := false
	for _, raw := range strings.Split(tag, ",") {
		part := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(part, "index:"), strings.HasPrefix(part, "lsi:"):
			inIndexClause = true
		case part == "pk" && !inIndexClause:
			return true
		}
	}
	return false
}
