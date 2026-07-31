package query

import (
	"context"
	"fmt"
	"strings"

	"github.com/theory-cloud/tabletheory/v3/pkg/core"
)

func (q *Query) OrFilter(field string, op string, value any) core.Query {
	return q.addFilterCondition("OR", field, op, value)
}

func (q *Query) addFilterGroup(groupOperator string, fn func(core.Query)) core.Query {
	// Initialize builder if not already done
	if q.builder == nil {
		q.builder = q.newBuilder()
	}

	// Create a new sub-query and builder for the group
	subBuilder := q.newBuilder()
	subQuery := &Query{
		model:    q.model,
		metadata: q.metadata,
		executor: q.executor,
		ctx:      q.ctx,
		builder:  subBuilder,
		// Ensure grouped conditions behave identically to the parent query.
		rawMetadata: q.rawMetadata,
		converter:   q.converter,
		marshaler:   q.marshaler,
	}

	// Execute the user's function to build the sub-query
	fn(subQuery)
	if err := subQuery.checkBuilderError(); err != nil {
		q.recordBuilderError(err)
	}

	// Build the components from the sub-query
	components := subBuilder.Build()

	// Add the built group to the main builder
	q.builder.AddGroupFilter(groupOperator, components)
	return q
}

// FilterGroup adds a grouped AND filter condition
func (q *Query) FilterGroup(fn func(core.Query)) core.Query {
	return q.addFilterGroup("AND", fn)
}

// OrFilterGroup adds a grouped OR filter condition
func (q *Query) OrFilterGroup(fn func(core.Query)) core.Query {
	return q.addFilterGroup("OR", fn)
}

// IfNotExists ensures the primary key does not exist prior to write
func (q *Query) IfNotExists() core.Query {
	q.addPrimaryKeyCondition("attribute_not_exists")
	return q
}

// IfExists ensures the primary key exists prior to write
func (q *Query) IfExists() core.Query {
	q.addPrimaryKeyCondition("attribute_exists")
	return q
}

// WithCondition appends an additional write condition
func (q *Query) WithCondition(field, operator string, value any) core.Query {
	if err := q.rejectEncryptedConditionField(field); err != nil {
		q.recordBuilderError(err)
		return q
	}
	normalized, err := q.normalizeJSONFieldValue(field, value)
	if err != nil {
		q.recordBuilderError(err)
		return q
	}
	attrName := q.resolveAttributeName(field)
	q.writeConditions = append(q.writeConditions, Condition{
		Field:    attrName,
		Operator: operator,
		Value:    normalized,
	})
	return q
}

// WithConditionExpression appends a raw condition expression for advanced cases
func (q *Query) WithConditionExpression(exprStr string, values map[string]any) core.Query {
	exprStr = strings.TrimSpace(exprStr)
	if exprStr == "" {
		q.recordBuilderError(fmt.Errorf("condition expression cannot be empty"))
		return q
	}

	q.rawConditionExpressions = append(q.rawConditionExpressions, conditionExpression{
		Expression: exprStr,
		Values:     cloneConditionValues(values),
	})
	return q
}

// recordBuilderError memoizes the first builder error encountered
func (q *Query) recordBuilderError(err error) {
	if err != nil && q.builderErr == nil {
		q.builderErr = err
	}
}

// checkBuilderError returns any previously recorded builder error
func (q *Query) checkBuilderError() error {
	return q.builderErr
}

// UpdateBuilder returns a builder for complex update operations
func (q *Query) UpdateBuilder() core.UpdateBuilder {
	return NewUpdateBuilder(q)
}

// NewWithConditions creates a new Query instance with all necessary fields
func NewWithConditions(model any, metadata core.ModelMetadata, executor QueryExecutor, conditions []Condition, ctx context.Context) *Query { //nolint:revive // context-as-argument: keep signature for compatibility
	if ctx == nil {
		ctx = context.Background()
	}

	q := &Query{
		model:                   model,
		metadata:                metadata,
		executor:                executor,
		conditions:              conditions,
		ctx:                     ctx,
		filters:                 make([]Filter, 0),
		writeConditions:         make([]Condition, 0),
		rawConditionExpressions: make([]conditionExpression, 0),
	}
	if provider, ok := metadata.(rawMetadataProvider); ok {
		q.rawMetadata = provider.RawMetadata()
	}
	q.setExecutorContext(q.ctx)
	return q
}
