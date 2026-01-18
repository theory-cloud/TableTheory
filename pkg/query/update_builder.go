// Package query provides update builder functionality for DynamoDB
package query

import (
	"fmt"
	"reflect"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/theory-cloud/tabletheory/internal/expr"
	"github.com/theory-cloud/tabletheory/pkg/core"
	theorydbErrors "github.com/theory-cloud/tabletheory/pkg/errors"
)

// UpdateBuilder provides a fluent API for building complex update expressions
type UpdateBuilder struct {
	buildErr     error
	query        *Query
	expr         *expr.Builder
	keyValues    map[string]any
	returnValues string
	conditions   []updateCondition
}

type updateCondition struct {
	field    string
	operator string
	value    any
	logicOp  string
}

// NewUpdateBuilder creates a new UpdateBuilder with the given query
func NewUpdateBuilder(q *Query) core.UpdateBuilder {
	return &UpdateBuilder{
		query:        q,
		expr:         expr.NewBuilder(),
		keyValues:    make(map[string]any),
		returnValues: "NONE", // Default
	}
}

// mapFieldToDynamoDBName maps a Go field name to its DynamoDB attribute name
func (ub *UpdateBuilder) mapFieldToDynamoDBName(field string) string {
	if ub.query.metadata != nil {
		if fieldMeta := ub.query.metadata.AttributeMetadata(field); fieldMeta != nil {
			return fieldMeta.DynamoDBName
		}
	}
	return field
}

// Set adds a SET expression to update a field
func (ub *UpdateBuilder) Set(field string, value any) core.UpdateBuilder {
	dbFieldName := ub.mapFieldToDynamoDBName(field)
	if err := ub.expr.AddUpdateSet(dbFieldName, value); err != nil && ub.buildErr == nil {
		ub.buildErr = fmt.Errorf("Set(%s): %w", field, err)
	}
	return ub
}

// SetIfNotExists sets a field only if it doesn't exist
func (ub *UpdateBuilder) SetIfNotExists(field string, value any, defaultValue any) core.UpdateBuilder {
	dbFieldName := ub.mapFieldToDynamoDBName(field)
	// DynamoDB if_not_exists function syntax: SET field = if_not_exists(field, default_value)
	// The 'value' parameter is ignored as DynamoDB if_not_exists only checks existence, not value comparison
	err := ub.expr.AddUpdateFunction(dbFieldName, "if_not_exists", dbFieldName, defaultValue)
	if err != nil && ub.buildErr == nil {
		ub.buildErr = fmt.Errorf("SetIfNotExists(%s): %w", field, err)
	}
	return ub
}

// Add increments a numeric field (atomic counter)
func (ub *UpdateBuilder) Add(field string, value any) core.UpdateBuilder {
	dbFieldName := ub.mapFieldToDynamoDBName(field)
	if err := ub.expr.AddUpdateAdd(dbFieldName, value); err != nil && ub.buildErr == nil {
		ub.buildErr = fmt.Errorf("Add(%s): %w", field, err)
	}
	return ub
}

// Increment is an alias for Add with value 1
func (ub *UpdateBuilder) Increment(field string) core.UpdateBuilder {
	return ub.Add(field, 1)
}

// Decrement is an alias for Add with value -1
func (ub *UpdateBuilder) Decrement(field string) core.UpdateBuilder {
	return ub.Add(field, -1)
}

// Remove removes an attribute from the item
func (ub *UpdateBuilder) Remove(field string) core.UpdateBuilder {
	dbFieldName := ub.mapFieldToDynamoDBName(field)
	if err := ub.expr.AddUpdateRemove(dbFieldName); err != nil && ub.buildErr == nil {
		ub.buildErr = fmt.Errorf("Remove(%s): %w", field, err)
	}
	return ub
}

// Delete removes elements from a set
func (ub *UpdateBuilder) Delete(field string, value any) core.UpdateBuilder {
	dbFieldName := ub.mapFieldToDynamoDBName(field)

	// DynamoDB DELETE action is for removing elements from a set
	// Ensure the value is properly formatted as a set
	var setValue any

	// Convert single values to slices for set operations
	switch v := value.(type) {
	case string:
		setValue = []string{v}
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		setValue = []any{v}
	case []byte:
		setValue = [][]byte{v}
	case []string, []int, []float64, [][]byte:
		setValue = v
	default:
		// For other types, try to convert to a slice if it's not already
		rv := reflect.ValueOf(value)
		if rv.Kind() == reflect.Slice {
			setValue = value
		} else {
			// Wrap single value in a slice
			setValue = []any{value}
		}
	}

	if err := ub.expr.AddUpdateDelete(dbFieldName, setValue); err != nil && ub.buildErr == nil {
		ub.buildErr = fmt.Errorf("Delete(%s): %w", field, err)
	}
	return ub
}

// AppendToList appends values to the end of a list
func (ub *UpdateBuilder) AppendToList(field string, values any) core.UpdateBuilder {
	dbFieldName := ub.mapFieldToDynamoDBName(field)
	// Use list_append function to append values
	// list_append(field, values) appends to the end
	err := ub.expr.AddUpdateFunction(dbFieldName, "list_append", dbFieldName, values)
	if err != nil && ub.buildErr == nil {
		ub.buildErr = fmt.Errorf("AppendToList(%s): %w", field, err)
	}
	return ub
}

// PrependToList prepends values to the beginning of a list
func (ub *UpdateBuilder) PrependToList(field string, values any) core.UpdateBuilder {
	dbFieldName := ub.mapFieldToDynamoDBName(field)
	// Use list_append function to prepend values
	// list_append(values, field) prepends to the beginning
	err := ub.expr.AddUpdateFunction(dbFieldName, "list_append", values, dbFieldName)
	if err != nil && ub.buildErr == nil {
		ub.buildErr = fmt.Errorf("PrependToList(%s): %w", field, err)
	}
	return ub
}

// RemoveFromListAt removes an element from a list at a specific index
func (ub *UpdateBuilder) RemoveFromListAt(field string, index int) core.UpdateBuilder {
	dbFieldName := ub.mapFieldToDynamoDBName(field)
	if err := ub.expr.AddUpdateRemove(fmt.Sprintf("%s[%d]", dbFieldName, index)); err != nil && ub.buildErr == nil {
		ub.buildErr = fmt.Errorf("RemoveFromListAt(%s): %w", field, err)
	}
	return ub
}

// SetListElement sets a specific element in a list
func (ub *UpdateBuilder) SetListElement(field string, index int, value any) core.UpdateBuilder {
	dbFieldName := ub.mapFieldToDynamoDBName(field)
	if err := ub.expr.AddUpdateSet(fmt.Sprintf("%s[%d]", dbFieldName, index), value); err != nil && ub.buildErr == nil {
		ub.buildErr = fmt.Errorf("SetListElement(%s): %w", field, err)
	}
	return ub
}

// Condition adds a condition that must be met for the update to succeed
func (ub *UpdateBuilder) Condition(field string, operator string, value any) core.UpdateBuilder {
	if ub.buildErr == nil && ub.query != nil && ub.query.metadata != nil {
		if meta := ub.query.metadata.AttributeMetadata(field); meta != nil && len(meta.Tags) > 0 {
			if _, ok := meta.Tags["encrypted"]; ok {
				name := meta.Name
				if name == "" {
					name = field
				}
				ub.buildErr = fmt.Errorf("%w: %s", theorydbErrors.ErrEncryptedFieldNotQueryable, name)
				return ub
			}
		}
	}

	ub.conditions = append(ub.conditions, updateCondition{
		field:    field,
		operator: operator,
		value:    value,
		logicOp:  "AND",
	})
	return ub
}

// OrCondition adds a condition with OR logic
func (ub *UpdateBuilder) OrCondition(field string, operator string, value any) core.UpdateBuilder {
	if ub.buildErr == nil && ub.query != nil && ub.query.metadata != nil {
		if meta := ub.query.metadata.AttributeMetadata(field); meta != nil && len(meta.Tags) > 0 {
			if _, ok := meta.Tags["encrypted"]; ok {
				name := meta.Name
				if name == "" {
					name = field
				}
				ub.buildErr = fmt.Errorf("%w: %s", theorydbErrors.ErrEncryptedFieldNotQueryable, name)
				return ub
			}
		}
	}

	ub.conditions = append(ub.conditions, updateCondition{
		field:    field,
		operator: operator,
		value:    value,
		logicOp:  "OR",
	})
	return ub
}

// ConditionExists adds a condition that the field must exist
func (ub *UpdateBuilder) ConditionExists(field string) core.UpdateBuilder {
	return ub.Condition(field, "attribute_exists", nil)
}

// ConditionNotExists adds a condition that the field must not exist
func (ub *UpdateBuilder) ConditionNotExists(field string) core.UpdateBuilder {
	return ub.Condition(field, "attribute_not_exists", nil)
}

// ConditionVersion adds optimistic locking based on version field
func (ub *UpdateBuilder) ConditionVersion(currentVersion int64) core.UpdateBuilder {
	fieldName := "Version"
	if ub.query.metadata != nil {
		if name := ub.query.metadata.VersionFieldName(); name != "" {
			fieldName = name
		}
	}
	return ub.Condition(fieldName, "=", currentVersion)
}

// ReturnValues sets what values to return after the update
func (ub *UpdateBuilder) ReturnValues(option string) core.UpdateBuilder {
	// Options: NONE, ALL_OLD, UPDATED_OLD, ALL_NEW, UPDATED_NEW
	ub.returnValues = option
	return ub
}

func (ub *UpdateBuilder) populateKeyValues() error {
	if ub.query == nil {
		return fmt.Errorf("query is nil")
	}
	if ub.query.metadata == nil {
		return fmt.Errorf("query metadata is nil")
	}

	primaryKey := ub.query.metadata.PrimaryKey()
	resolveAttr := func(field string) string {
		if field == "" || ub.query.metadata == nil {
			return field
		}
		if meta := ub.query.metadata.AttributeMetadata(field); meta != nil && meta.DynamoDBName != "" {
			return meta.DynamoDBName
		}
		return field
	}

	pkAttr := resolveAttr(primaryKey.PartitionKey)
	skAttr := resolveAttr(primaryKey.SortKey)

	if len(ub.keyValues) > 0 {
		normalized := make(map[string]any, len(ub.keyValues))
		for field, value := range ub.keyValues {
			normalized[resolveAttr(field)] = value
		}
		ub.keyValues = normalized

		if _, ok := ub.keyValues[pkAttr]; !ok {
			return fmt.Errorf("partition key %s is required for update", primaryKey.PartitionKey)
		}
		if primaryKey.SortKey != "" {
			if _, ok := ub.keyValues[skAttr]; !ok {
				return fmt.Errorf("sort key %s is required for update", primaryKey.SortKey)
			}
		}

		return nil
	}

	ub.keyValues = make(map[string]any)
	for _, cond := range ub.query.conditions {
		condAttr := resolveAttr(cond.Field)
		if condAttr == pkAttr || (primaryKey.SortKey != "" && condAttr == skAttr) {
			if cond.Operator != "=" {
				return fmt.Errorf("key condition must use '=' operator")
			}
			ub.keyValues[condAttr] = cond.Value
		}
	}

	if _, ok := ub.keyValues[pkAttr]; !ok {
		return fmt.Errorf("partition key %s is required for update", primaryKey.PartitionKey)
	}

	if primaryKey.SortKey != "" {
		if _, ok := ub.keyValues[skAttr]; !ok {
			return fmt.Errorf("sort key %s is required for update", primaryKey.SortKey)
		}
	}

	return nil
}

// Execute performs the update operation
func (ub *UpdateBuilder) Execute() error {
	// Check for any errors that occurred during building
	if ub.buildErr != nil {
		return ub.buildErr
	}

	if err := ub.populateKeyValues(); err != nil {
		return err
	}

	// Add conditions to expression builder
	for _, cond := range ub.conditions {
		// Map field name to DynamoDB attribute name
		fieldName := cond.field
		if fieldMeta := ub.query.metadata.AttributeMetadata(cond.field); fieldMeta != nil {
			fieldName = fieldMeta.DynamoDBName
		}

		err := ub.expr.AddConditionExpression(fieldName, cond.operator, cond.value)
		if err != nil {
			return fmt.Errorf("failed to add condition: %w", err)
		}
	}

	// Build the expression components
	// Build the expression components
	components := ub.expr.Build()
	updateExpr := components.UpdateExpression
	exprAttrNames := components.ExpressionAttributeNames
	if exprAttrNames == nil {
		exprAttrNames = make(map[string]string)
	}
	exprAttrValues := components.ExpressionAttributeValues
	if exprAttrValues == nil {
		exprAttrValues = make(map[string]types.AttributeValue)
	}
	updateCondExpr := components.ConditionExpression

	combinedBuilder := ub.expr.Clone()
	combinedBuilder.ResetConditions()

	queryCondExpr, queryCondNames, queryCondValues, err := ub.query.buildConditionExpression(combinedBuilder, false, false, false)
	if err != nil {
		return fmt.Errorf("failed to build query conditions: %w", err)
	}

	finalCondExpr := ""
	if updateCondExpr != "" && queryCondExpr != "" {
		finalCondExpr = fmt.Sprintf("(%s) AND (%s)", updateCondExpr, queryCondExpr)
	} else if updateCondExpr != "" {
		finalCondExpr = updateCondExpr
	} else if queryCondExpr != "" {
		finalCondExpr = queryCondExpr
	}

	for k, v := range queryCondNames {
		exprAttrNames[k] = v
	}
	for k, v := range queryCondValues {
		exprAttrValues[k] = v
	}

	// Compile the update query
	compiled := &core.CompiledQuery{
		Operation:                "UpdateItem",
		TableName:                ub.query.metadata.TableName(),
		UpdateExpression:         updateExpr,
		ConditionExpression:      finalCondExpr,
		ExpressionAttributeNames: exprAttrNames,
		ReturnValues:             ub.returnValues,
	}

	// Only include ExpressionAttributeValues if it's not empty
	if len(exprAttrValues) > 0 {
		compiled.ExpressionAttributeValues = exprAttrValues
	}

	// Convert key to AttributeValues
	keyAV := make(map[string]types.AttributeValue)
	for k, v := range ub.keyValues {
		av, err := expr.ConvertToAttributeValue(v)
		if err != nil {
			return fmt.Errorf("failed to convert key value: %w", err)
		}
		keyAV[k] = av
	}

	// Execute update through executor
	if updateExecutor, ok := ub.query.executor.(UpdateItemExecutor); ok {
		return updateExecutor.ExecuteUpdateItem(compiled, keyAV)
	}

	return fmt.Errorf("executor does not support UpdateItem operation")
}

// ExecuteWithResult performs the update and returns the result
func (ub *UpdateBuilder) ExecuteWithResult(result any) error {
	// Check for any errors that occurred during building
	if ub.buildErr != nil {
		return ub.buildErr
	}

	// Validate result is a pointer
	resultValue := reflect.ValueOf(result)
	if resultValue.Kind() != reflect.Ptr || resultValue.IsNil() {
		return fmt.Errorf("result must be a non-nil pointer")
	}

	// Set return values to ALL_NEW if not already set
	if ub.returnValues == "NONE" {
		ub.returnValues = "ALL_NEW"
	}

	if err := ub.populateKeyValues(); err != nil {
		return err
	}

	// Add conditions to expression builder
	for _, cond := range ub.conditions {
		// Map field name to DynamoDB attribute name
		fieldName := cond.field
		if fieldMeta := ub.query.metadata.AttributeMetadata(cond.field); fieldMeta != nil {
			fieldName = fieldMeta.DynamoDBName
		}

		err := ub.expr.AddConditionExpressionWithOp(cond.logicOp, fieldName, cond.operator, cond.value)
		if err != nil {
			return fmt.Errorf("failed to add condition: %w", err)
		}
	}

	// Build the expression components
	components := ub.expr.Build()
	updateExpr := components.UpdateExpression
	exprAttrNames := components.ExpressionAttributeNames
	if exprAttrNames == nil {
		exprAttrNames = make(map[string]string)
	}
	exprAttrValues := components.ExpressionAttributeValues
	if exprAttrValues == nil {
		exprAttrValues = make(map[string]types.AttributeValue)
	}
	updateCondExpr := components.ConditionExpression

	combinedBuilder := ub.expr.Clone()
	combinedBuilder.ResetConditions()

	queryCondExpr, queryCondNames, queryCondValues, err := ub.query.buildConditionExpression(combinedBuilder, false, false, false)
	if err != nil {
		return fmt.Errorf("failed to build query conditions: %w", err)
	}

	finalCondExpr := ""
	if updateCondExpr != "" && queryCondExpr != "" {
		finalCondExpr = fmt.Sprintf("(%s) AND (%s)", updateCondExpr, queryCondExpr)
	} else if updateCondExpr != "" {
		finalCondExpr = updateCondExpr
	} else if queryCondExpr != "" {
		finalCondExpr = queryCondExpr
	}

	for k, v := range queryCondNames {
		exprAttrNames[k] = v
	}
	for k, v := range queryCondValues {
		exprAttrValues[k] = v
	}

	// Compile the update query
	compiled := &core.CompiledQuery{
		Operation:                "UpdateItem",
		TableName:                ub.query.metadata.TableName(),
		UpdateExpression:         updateExpr,
		ConditionExpression:      finalCondExpr,
		ExpressionAttributeNames: exprAttrNames,
		ReturnValues:             ub.returnValues,
	}

	// Only include ExpressionAttributeValues if it's not empty
	if len(exprAttrValues) > 0 {
		compiled.ExpressionAttributeValues = exprAttrValues
	}

	// Convert key to AttributeValues
	keyAV := make(map[string]types.AttributeValue)
	for k, v := range ub.keyValues {
		av, err := expr.ConvertToAttributeValue(v)
		if err != nil {
			return fmt.Errorf("failed to convert key value: %w", err)
		}
		keyAV[k] = av
	}

	// Check if executor supports returning results
	if updateExecutor, ok := ub.query.executor.(UpdateItemWithResultExecutor); ok {
		updateResult, err := updateExecutor.ExecuteUpdateItemWithResult(compiled, keyAV)
		if err != nil {
			return err
		}

		// Unmarshal the returned attributes to the result
		if updateResult != nil && len(updateResult.Attributes) > 0 {
			normalized := make(map[string]types.AttributeValue, len(updateResult.Attributes)*2)
			for attrName, attrValue := range updateResult.Attributes {
				normalized[attrName] = attrValue
				if attrMeta := ub.query.metadata.AttributeMetadata(attrName); attrMeta != nil && attrMeta.Name != "" {
					normalized[attrMeta.Name] = attrValue
				}
			}
			mapAV := &types.AttributeValueMemberM{Value: normalized}
			return expr.ConvertFromAttributeValue(mapAV, result)
		}
		return nil
	}

	// Fallback to regular update without result
	if updateExecutor, ok := ub.query.executor.(UpdateItemExecutor); ok {
		return updateExecutor.ExecuteUpdateItem(compiled, keyAV)
	}

	return fmt.Errorf("executor does not support UpdateItem operation")
}
