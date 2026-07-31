package query

import (
	"errors"
	"fmt"

	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	theorydbErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
)

// Create creates a new item
func (q *Query) Create() error {
	if err := q.checkBuilderError(); err != nil {
		return err
	}
	// Marshal the model to AttributeValues
	item, err := q.marshalItem(q.model)
	if err != nil {
		return fmt.Errorf("failed to marshal item: %w", err)
	}

	// Build PutItem request
	compiled := &core.CompiledQuery{
		Operation: "PutItem",
		TableName: q.metadata.TableName(),
	}

	conditionBuilder := q.newBuilder()
	if q.requiresCreateNotExistsCondition() {
		if defaultErr := q.addDefaultCondition(conditionBuilder); defaultErr != nil {
			return defaultErr
		}
	}

	conditionExpr, names, values, err := q.buildConditionExpression(conditionBuilder, false, false, false)
	if err != nil {
		return err
	}
	if conditionExpr != "" {
		compiled.ConditionExpression = conditionExpr
	}
	if len(names) > 0 {
		compiled.ExpressionAttributeNames = names
	}
	if len(values) > 0 {
		compiled.ExpressionAttributeValues = values
	}

	// Execute through a specialized PutItem executor
	if putExecutor, ok := q.executor.(PutItemExecutor); ok {
		if err := putExecutor.ExecutePutItem(compiled, item); err != nil {
			if errors.Is(err, theorydbErrors.ErrConditionFailed) {
				return fmt.Errorf("%w: item with the same key already exists", theorydbErrors.ErrConditionFailed)
			}
			return err
		}
		q.updateTimestampsInModel()
		return nil
	}

	// Fallback: return error if executor doesn't support PutItem
	return fmt.Errorf("executor does not support PutItem operation")
}

// CreateOrUpdate creates a new item or updates an existing one (upsert)
func (q *Query) CreateOrUpdate() error {
	if err := q.checkBuilderError(); err != nil {
		return err
	}
	if err := q.rejectWriteOnceMutation("upsert"); err != nil {
		return err
	}
	if err := q.rejectProtectedOverwrite("upsert"); err != nil {
		return err
	}
	item, err := q.marshalItem(q.model)
	if err != nil {
		return fmt.Errorf("failed to marshal item: %w", err)
	}

	// Compile the query for PutItem (without condition expression)
	compiled := &core.CompiledQuery{
		Operation: "PutItem",
		TableName: q.metadata.TableName(),
	}

	// Execute through a specialized PutItem executor
	if putExecutor, ok := q.executor.(PutItemExecutor); ok {
		if err := putExecutor.ExecutePutItem(compiled, item); err != nil {
			return err
		}
		q.updateTimestampsInModel()
		return nil
	}

	// Fallback: return error if executor doesn't support PutItem
	return fmt.Errorf("executor does not support PutItem operation")
}
