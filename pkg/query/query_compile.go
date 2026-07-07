package query

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/theory-cloud/tabletheory/v2/internal/expr"
	"github.com/theory-cloud/tabletheory/v2/internal/numutil"
	"github.com/theory-cloud/tabletheory/v2/pkg/core"
	theorydbErrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	"github.com/theory-cloud/tabletheory/v2/pkg/index"
)

func (q *Query) selectBestIndex() (*core.IndexSchema, error) {
	// Get all indexes including the primary index
	rawIndexes := make([]core.IndexSchema, 0, len(q.metadata.Indexes())+1)

	// Add the primary index (name is empty)
	primaryKey := q.metadata.PrimaryKey()
	rawIndexes = append(rawIndexes, core.IndexSchema{
		Name:         "",
		Type:         "PRIMARY",
		PartitionKey: primaryKey.PartitionKey,
		SortKey:      primaryKey.SortKey,
	})

	// Add GSIs and LSIs
	rawIndexes = append(rawIndexes, q.metadata.Indexes()...)

	// Keep Go field names; Compile() resolves to DynamoDB names when needed
	selector := index.NewSelector(rawIndexes)

	// Convert our conditions to index.Condition type
	indexConditions := make([]index.Condition, len(q.conditions))
	for i, cond := range q.conditions {
		normalized, goField, attrName := q.normalizeCondition(cond)

		fieldForIndex := goField
		if fieldForIndex == "" {
			fieldForIndex = attrName
		}
		if fieldForIndex == "" {
			fieldForIndex = normalized.Field
		}

		indexConditions[i] = index.Condition{
			Field:    fieldForIndex,
			Operator: normalized.Operator,
			Value:    normalized.Value,
		}
	}

	// Analyze conditions to find required keys
	requiredKeys := index.AnalyzeConditions(indexConditions)

	// Use the selector to find the best index
	return selector.SelectOptimal(requiredKeys, nil)
}

// Compile compiles the query into executable form
func (q *Query) Compile() (*core.CompiledQuery, error) {
	builder := q.effectiveBuilder()

	compiled := &core.CompiledQuery{
		TableName: q.metadata.TableName(),
	}

	if err := q.compileOperation(builder, compiled); err != nil {
		return nil, err
	}
	if q.consistentRead && q.usesGlobalSecondaryIndex(compiled.IndexName) {
		return nil, fmt.Errorf("%w: consistent reads are not supported on GSIs", theorydbErrors.ErrInvalidOperator)
	}

	q.applyProjections(builder)
	q.applyExpressionComponents(compiled, builder)
	q.applyCompiledSettings(compiled)

	return compiled, nil
}

func (q *Query) compileOperation(builder *expr.Builder, compiled *core.CompiledQuery) error {
	if q.index != "" {
		return q.compileWithExplicitIndex(builder, compiled, q.index)
	}
	return q.compileWithBestIndex(builder, compiled)
}

func (q *Query) compileWithExplicitIndex(builder *expr.Builder, compiled *core.CompiledQuery, name string) error {
	compiled.IndexName = name

	keys := q.keyNamesForIndex(q.indexSchemaByName(name))
	keyConditions, filterConditions := q.partitionConditionsForKeys(keys)
	if q.hasPartitionKeyCondition(keyConditions, keys.pkAttr) {
		compiled.Operation = operationQuery
		return q.applyKeyAndFilterConditions(builder, keyConditions, filterConditions)
	}

	compiled.Operation = operationScan
	return q.applyScanConditions(builder)
}

func (q *Query) compileWithBestIndex(builder *expr.Builder, compiled *core.CompiledQuery) error {
	bestIndex, err := q.selectBestIndex()
	if err != nil {
		return err
	}

	if bestIndex != nil {
		compiled.Operation = operationQuery
		if bestIndex.Name != "" {
			compiled.IndexName = bestIndex.Name
		}
		return q.applyQueryConditions(builder, bestIndex)
	}

	compiled.Operation = operationScan
	return q.applyScanConditions(builder)
}

func (q *Query) usesGlobalSecondaryIndex(name string) bool {
	if name == "" {
		return false
	}
	idx := q.indexSchemaByName(name)
	return idx != nil && strings.EqualFold(idx.Type, "GSI")
}

func (q *Query) indexSchemaByName(name string) *core.IndexSchema {
	for _, idx := range q.metadata.Indexes() {
		if idx.Name == name {
			copyIdx := idx
			return &copyIdx
		}
	}
	return nil
}

func (q *Query) hasPartitionKeyCondition(conditions []Condition, pkName string) bool {
	for _, cond := range conditions {
		if strings.EqualFold(cond.Field, pkName) {
			return true
		}
	}
	return false
}

func (q *Query) applyKeyAndFilterConditions(builder *expr.Builder, keyConditions []Condition, filterConditions []Condition) error {
	for _, cond := range keyConditions {
		if err := builder.AddKeyCondition(cond.Field, cond.Operator, cond.Value); err != nil {
			return err
		}
	}
	for _, cond := range filterConditions {
		if err := builder.AddFilterCondition("AND", cond.Field, cond.Operator, cond.Value); err != nil {
			return err
		}
	}
	return nil
}

func (q *Query) partitionConditionsForKeys(keys keyNameSet) ([]Condition, []Condition) {
	keyConditions := make([]Condition, 0)
	filterConditions := make([]Condition, 0)

	for _, original := range q.conditions {
		normalized, goField, attrName := q.normalizeCondition(original)
		condGoName, condAttrName := q.resolveConditionNames(goField, attrName)

		if !keys.isKey(condGoName, condAttrName) {
			filterConditions = append(filterConditions, normalized)
			continue
		}

		operator := strings.ToUpper(strings.TrimSpace(normalized.Operator))
		if keys.isPartitionKey(condGoName, condAttrName) {
			if operator == "=" {
				keyConditions = append(keyConditions, normalized)
			} else {
				filterConditions = append(filterConditions, normalized)
			}
			continue
		}

		switch operator {
		case "=", "<", "<=", ">", ">=", "BETWEEN", "BEGINS_WITH":
			keyConditions = append(keyConditions, normalized)
		default:
			filterConditions = append(filterConditions, normalized)
		}
	}

	return keyConditions, filterConditions
}

func (q *Query) effectiveBuilder() *expr.Builder {
	if q.builder != nil {
		return q.builder.Clone()
	}
	return q.newBuilder()
}

func (q *Query) newBuilder() *expr.Builder {
	if q.converter != nil {
		return expr.NewBuilderWithConverter(q.converter)
	}
	return expr.NewBuilder()
}

func (q *Query) toAttributeValue(value any) (types.AttributeValue, error) {
	if q != nil && q.converter != nil {
		return q.converter.ToAttributeValue(value)
	}
	return expr.ConvertToAttributeValue(value)
}

func (q *Query) fillKeyValuesFromModel(pkGo, skGo string, pkValue *any, pkFound *bool, skValue *any, skFound *bool) {
	if q == nil || q.model == nil || pkValue == nil || pkFound == nil || skValue == nil || skFound == nil {
		return
	}
	if *pkFound && (skGo == "" || *skFound) {
		return
	}

	modelValue, ok := q.modelStructValue()
	if !ok {
		return
	}

	q.fillKeyValuesFromRawMetadata(modelValue, skGo, pkValue, pkFound, skValue, skFound)
	q.fillKeyValuesByName(modelValue, pkGo, skGo, pkValue, pkFound, skValue, skFound)
}

func (q *Query) modelStructValue() (reflect.Value, bool) {
	modelValue := reflect.ValueOf(q.model)
	if !modelValue.IsValid() {
		return reflect.Value{}, false
	}
	if modelValue.Kind() == reflect.Ptr {
		if modelValue.IsNil() {
			return reflect.Value{}, false
		}
		modelValue = modelValue.Elem()
	}
	if !modelValue.IsValid() || modelValue.Kind() != reflect.Struct {
		return reflect.Value{}, false
	}
	return modelValue, true
}

func (q *Query) fillKeyValuesFromRawMetadata(modelValue reflect.Value, skGo string, pkValue *any, pkFound *bool, skValue *any, skFound *bool) {
	if q.rawMetadata == nil || q.rawMetadata.PrimaryKey == nil {
		return
	}

	if q.rawMetadata.PrimaryKey.PartitionKey != nil && !*pkFound {
		field := modelValue.FieldByIndex(q.rawMetadata.PrimaryKey.PartitionKey.IndexPath)
		if field.IsValid() && !field.IsZero() {
			*pkValue = field.Interface()
			*pkFound = true
		}
	}

	if skGo != "" && q.rawMetadata.PrimaryKey.SortKey != nil && !*skFound {
		field := modelValue.FieldByIndex(q.rawMetadata.PrimaryKey.SortKey.IndexPath)
		if field.IsValid() && !field.IsZero() {
			*skValue = field.Interface()
			*skFound = true
		}
	}
}

func (q *Query) fillKeyValuesByName(modelValue reflect.Value, pkGo, skGo string, pkValue *any, pkFound *bool, skValue *any, skFound *bool) {
	if !*pkFound {
		field, _, ok := q.findVisibleFieldByNames(modelValue, pkGo)
		if ok && field.IsValid() && !field.IsZero() {
			*pkValue = field.Interface()
			*pkFound = true
		}
	}

	if skGo != "" && !*skFound {
		field, _, ok := q.findVisibleFieldByNames(modelValue, skGo)
		if ok && field.IsValid() && !field.IsZero() {
			*skValue = field.Interface()
			*skFound = true
		}
	}
}

func (q *Query) buildPrimaryKeyMap(operation string) (map[string]types.AttributeValue, error) {
	pkGo, pkAttr, skGo, skAttr, err := q.resolvePrimaryKeyNames(operation)
	if err != nil {
		return nil, err
	}

	pkValue, pkFound, skValue, skFound, err := q.extractPrimaryKeyValuesFromConditions(pkGo, pkAttr, skGo, skAttr)
	if err != nil {
		return nil, err
	}

	q.fillKeyValuesFromModel(pkGo, skGo, &pkValue, &pkFound, &skValue, &skFound)

	if err := validatePrimaryKeyValues(operation, pkGo, skGo, pkFound, skFound); err != nil {
		return nil, err
	}

	return q.buildPrimaryKeyAttributeValues(pkAttr, pkValue, skAttr, skValue, skGo != "")
}

func (q *Query) resolvePrimaryKeyNames(operation string) (string, string, string, string, error) {
	if q == nil {
		return "", "", "", "", fmt.Errorf("query cannot be nil")
	}
	if q.metadata == nil {
		return "", "", "", "", fmt.Errorf("model metadata is required for %s operations", operation)
	}

	schema := q.metadata.PrimaryKey()
	if schema.PartitionKey == "" {
		return "", "", "", "", fmt.Errorf("partition key is required for %s", operation)
	}

	pkGo := schema.PartitionKey
	pkAttr := q.resolveAttributeName(pkGo)
	skGo := schema.SortKey
	skAttr := ""
	if skGo != "" {
		skAttr = q.resolveAttributeName(skGo)
	}

	return pkGo, pkAttr, skGo, skAttr, nil
}

func (q *Query) extractPrimaryKeyValuesFromConditions(pkGo, pkAttr, skGo, skAttr string) (any, bool, any, bool, error) {
	var pkValue any
	var skValue any
	pkFound := false
	skFound := false

	for _, cond := range q.conditions {
		_, goField, attrName := q.normalizeCondition(cond)

		if strings.EqualFold(goField, pkGo) || strings.EqualFold(attrName, pkAttr) {
			if strings.TrimSpace(cond.Operator) != "=" {
				return nil, false, nil, false, fmt.Errorf("key condition must use '=' operator")
			}
			pkValue = cond.Value
			pkFound = true
			continue
		}

		if skGo != "" && (strings.EqualFold(goField, skGo) || strings.EqualFold(attrName, skAttr)) {
			if strings.TrimSpace(cond.Operator) != "=" {
				return nil, false, nil, false, fmt.Errorf("key condition must use '=' operator")
			}
			skValue = cond.Value
			skFound = true
		}
	}

	return pkValue, pkFound, skValue, skFound, nil
}

func validatePrimaryKeyValues(operation, pkGo, skGo string, pkFound, skFound bool) error {
	if !pkFound {
		return fmt.Errorf("partition key %s is required for %s", pkGo, operation)
	}
	if skGo != "" && !skFound {
		return fmt.Errorf("sort key %s is required for %s", skGo, operation)
	}
	return nil
}

func (q *Query) buildPrimaryKeyAttributeValues(pkAttr string, pkValue any, skAttr string, skValue any, hasSortKey bool) (map[string]types.AttributeValue, error) {
	pkAV, err := q.toAttributeValue(pkValue)
	if err != nil {
		return nil, fmt.Errorf("failed to convert partition key: %w", err)
	}

	key := map[string]types.AttributeValue{
		pkAttr: pkAV,
	}
	if !hasSortKey {
		return key, nil
	}

	skAV, err := q.toAttributeValue(skValue)
	if err != nil {
		return nil, fmt.Errorf("failed to convert sort key: %w", err)
	}
	key[skAttr] = skAV

	return key, nil
}

type keyNameSet struct {
	pkGo   string
	pkAttr string
	skGo   string
	skAttr string
}

func (k keyNameSet) isKey(goName, attrName string) bool {
	return k.isPartitionKey(goName, attrName) || k.isSortKey(goName, attrName)
}

func (k keyNameSet) isPartitionKey(goName, attrName string) bool {
	if k.pkGo == "" {
		return false
	}
	return strings.EqualFold(goName, k.pkGo) || strings.EqualFold(attrName, k.pkAttr)
}

func (k keyNameSet) isSortKey(goName, attrName string) bool {
	if k.skGo == "" {
		return false
	}
	return strings.EqualFold(goName, k.skGo) || strings.EqualFold(attrName, k.skAttr)
}

func (q *Query) applyQueryConditions(builder *expr.Builder, bestIndex *core.IndexSchema) error {
	keys := q.keyNamesForIndex(bestIndex)
	keyConditions, filterConditions := q.splitConditionsByKey(keys)

	for _, cond := range keyConditions {
		if err := builder.AddKeyCondition(cond.Field, cond.Operator, cond.Value); err != nil {
			return err
		}
	}

	for _, cond := range filterConditions {
		if err := builder.AddFilterCondition("AND", cond.Field, cond.Operator, cond.Value); err != nil {
			return err
		}
	}

	return nil
}

func (q *Query) applyScanConditions(builder *expr.Builder) error {
	for _, original := range q.conditions {
		normalized, _, _ := q.normalizeCondition(original)
		if err := builder.AddFilterCondition("AND", normalized.Field, normalized.Operator, normalized.Value); err != nil {
			return err
		}
	}
	return nil
}

func (q *Query) keyNamesForIndex(bestIndex *core.IndexSchema) keyNameSet {
	primaryKey := q.metadata.PrimaryKey()
	primaryPKGo, primaryPKAttr := q.resolveGoAndAttrName(primaryKey.PartitionKey)
	primarySKGo, primarySKAttr := q.resolveGoAndAttrName(primaryKey.SortKey)

	if bestIndex == nil || bestIndex.Name == "" {
		return keyNameSet{
			pkGo:   primaryPKGo,
			pkAttr: primaryPKAttr,
			skGo:   primarySKGo,
			skAttr: primarySKAttr,
		}
	}

	pkGoName, pkAttrName := q.resolveGoAndAttrName(bestIndex.PartitionKey)
	skGoName, skAttrName := q.resolveGoAndAttrName(bestIndex.SortKey)

	if pkGoName == "" {
		pkGoName = primaryPKGo
	}
	if pkAttrName == "" {
		pkAttrName = primaryPKAttr
	}
	if skGoName == "" {
		skGoName = primarySKGo
	}
	if skAttrName == "" {
		skAttrName = primarySKAttr
	}

	return keyNameSet{
		pkGo:   pkGoName,
		pkAttr: pkAttrName,
		skGo:   skGoName,
		skAttr: skAttrName,
	}
}

func (q *Query) resolveGoAndAttrName(field string) (string, string) {
	return q.resolveGoFieldName(field), q.resolveAttributeName(field)
}

func (q *Query) splitConditionsByKey(keys keyNameSet) ([]Condition, []Condition) {
	keyConditions := make([]Condition, 0)
	filterConditions := make([]Condition, 0)

	for _, original := range q.conditions {
		normalized, goField, attrName := q.normalizeCondition(original)
		condGoName, condAttrName := q.resolveConditionNames(goField, attrName)

		if keys.isKey(condGoName, condAttrName) {
			keyConditions = append(keyConditions, normalized)
		} else {
			filterConditions = append(filterConditions, normalized)
		}
	}

	return keyConditions, filterConditions
}

func (q *Query) resolveConditionNames(goField, attrName string) (string, string) {
	condGoName := goField
	condAttrName := attrName

	if meta := q.metadata.AttributeMetadata(goField); meta != nil {
		if meta.Name != "" {
			condGoName = meta.Name
		}
		if meta.DynamoDBName != "" {
			condAttrName = meta.DynamoDBName
		} else if condAttrName == "" {
			condAttrName = condGoName
		}
	} else if meta := q.metadata.AttributeMetadata(attrName); meta != nil {
		if meta.Name != "" {
			condGoName = meta.Name
		}
		if meta.DynamoDBName != "" {
			condAttrName = meta.DynamoDBName
		}
	}

	return condGoName, condAttrName
}

func (q *Query) applyProjections(builder *expr.Builder) {
	if len(q.projection) == 0 {
		return
	}
	builder.AddProjection(q.projection...)
}

func (q *Query) applyExpressionComponents(compiled *core.CompiledQuery, builder *expr.Builder) {
	components := builder.Build()
	compiled.KeyConditionExpression = components.KeyConditionExpression
	compiled.FilterExpression = components.FilterExpression
	compiled.ProjectionExpression = components.ProjectionExpression
	compiled.ExpressionAttributeNames = components.ExpressionAttributeNames
	compiled.ExpressionAttributeValues = components.ExpressionAttributeValues
}

func (q *Query) applyCompiledSettings(compiled *core.CompiledQuery) {
	if q.limit > 0 {
		limit := numutil.ClampIntToInt32(q.limit)
		compiled.Limit = &limit
	}

	if strings.EqualFold(q.orderBy.Order, "desc") {
		forward := false
		compiled.ScanIndexForward = &forward
	}

	if len(q.exclusive) > 0 {
		compiled.ExclusiveStartKey = q.exclusive
	}

	if q.consistentRead && compiled.IndexName == "" {
		compiled.ConsistentRead = &q.consistentRead
	}
}

// compileScan compiles a scan operation
func (q *Query) compileScan() (*core.CompiledQuery, error) {
	builder := q.effectiveBuilder()

	compiled := &core.CompiledQuery{
		TableName: q.metadata.TableName(),
		Operation: operationScan,
	}
	if q.index != "" {
		compiled.IndexName = q.index
	}

	// Add filter conditions from Where clauses
	for _, original := range q.conditions {
		normalized, _, _ := q.normalizeCondition(original)
		if err := builder.AddFilterCondition("AND", normalized.Field, normalized.Operator, normalized.Value); err != nil {
			return nil, err
		}
	}

	// Note: Additional filters from Filter/OrFilter calls are already in the builder

	// Add projections
	if len(q.projection) > 0 {
		builder.AddProjection(q.projection...)
	}

	// Build the expressions
	components := builder.Build()
	compiled.FilterExpression = components.FilterExpression
	compiled.ProjectionExpression = components.ProjectionExpression
	compiled.ExpressionAttributeNames = components.ExpressionAttributeNames
	compiled.ExpressionAttributeValues = components.ExpressionAttributeValues

	// Set parameters
	if q.limit > 0 {
		limit := numutil.ClampIntToInt32(q.limit)
		compiled.Limit = &limit
	}

	// Handle offset with pagination
	if q.offset != nil && *q.offset > 0 {
		// Note: DynamoDB doesn't support direct offset, so this would need
		// to be handled by the executor with multiple requests
		compiled.Offset = q.offset
	}

	compiled.ExclusiveStartKey = q.exclusive

	// Set parallel scan parameters if specified
	if q.segment != nil && q.totalSegments != nil {
		compiled.Segment = q.segment
		compiled.TotalSegments = q.totalSegments
	}

	// Set consistent read (only for main table scan, not GSI)
	if q.consistentRead && q.index == "" {
		compiled.ConsistentRead = &q.consistentRead
	}

	return compiled, nil
}

func (q *Query) compileGetItem() (*core.CompiledQuery, map[string]types.AttributeValue, bool, error) {
	if q == nil {
		return nil, nil, false, fmt.Errorf("query cannot be nil")
	}
	if q.metadata == nil {
		return nil, nil, false, fmt.Errorf("model metadata is required for get item operations")
	}
	if q.index != "" {
		return nil, nil, false, nil
	}
	if q.builder != nil {
		// Filters (Filter/OrFilter/FilterGroup) cannot be applied via GetItem.
		return nil, nil, false, nil
	}

	pkGo, pkAttr, skGo, skAttr, err := q.getItemKeyNames()
	if err != nil {
		return nil, nil, false, err
	}

	pkValue, pkFound, skValue, skFound, ok := q.extractGetItemKeyValuesFromConditions(pkGo, pkAttr, skGo, skAttr)
	if !ok {
		return nil, nil, false, nil
	}

	q.fillKeyValuesFromModel(pkGo, skGo, &pkValue, &pkFound, &skValue, &skFound)

	if !pkFound {
		return nil, nil, false, nil
	}
	if skGo != "" && !skFound {
		return nil, nil, false, nil
	}

	key, err := q.buildPrimaryKeyAttributeValues(pkAttr, pkValue, skAttr, skValue, skGo != "")
	if err != nil {
		return nil, nil, false, err
	}

	compiled := &core.CompiledQuery{
		Operation: "GetItem",
		TableName: q.metadata.TableName(),
	}
	if len(q.projection) > 0 {
		builder := q.newBuilder()
		builder.AddProjection(q.projection...)
		components := builder.Build()
		compiled.ProjectionExpression = components.ProjectionExpression
		compiled.ExpressionAttributeNames = components.ExpressionAttributeNames
	}
	if q.consistentRead {
		compiled.ConsistentRead = &q.consistentRead
	}

	return compiled, key, true, nil
}

func (q *Query) getItemKeyNames() (string, string, string, string, error) {
	schema := q.metadata.PrimaryKey()
	if schema.PartitionKey == "" {
		return "", "", "", "", fmt.Errorf("partition key is required for get item operations")
	}

	pkGo := schema.PartitionKey
	pkAttr := q.resolveAttributeName(pkGo)
	skGo := schema.SortKey
	skAttr := ""
	if skGo != "" {
		skAttr = q.resolveAttributeName(skGo)
	}

	return pkGo, pkAttr, skGo, skAttr, nil
}

func (q *Query) extractGetItemKeyValuesFromConditions(pkGo, pkAttr, skGo, skAttr string) (any, bool, any, bool, bool) {
	var pkValue any
	var skValue any
	pkFound := false
	skFound := false

	for _, cond := range q.conditions {
		_, goField, attrName := q.normalizeCondition(cond)

		if strings.EqualFold(goField, pkGo) || strings.EqualFold(attrName, pkAttr) {
			if strings.TrimSpace(cond.Operator) != "=" {
				return nil, false, nil, false, false
			}
			pkValue = cond.Value
			pkFound = true
			continue
		}

		if skGo != "" && (strings.EqualFold(goField, skGo) || strings.EqualFold(attrName, skAttr)) {
			if strings.TrimSpace(cond.Operator) != "=" {
				return nil, false, nil, false, false
			}
			skValue = cond.Value
			skFound = true
			continue
		}

		// Non-key WHERE conditions must use Query/Scan semantics.
		return nil, false, nil, false, false
	}

	return pkValue, pkFound, skValue, skFound, true
}
