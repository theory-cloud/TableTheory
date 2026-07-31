package query

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/theory-cloud/tabletheory/v3/internal/expr"
	"github.com/theory-cloud/tabletheory/v3/internal/fieldcodec"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	theorydbErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/marshal"
	"github.com/theory-cloud/tabletheory/v3/pkg/model"
)

// Query represents a DynamoDB query builder
type Query struct {
	builderErr              error
	executor                QueryExecutor
	metadata                core.ModelMetadata
	rawMetadata             *model.Metadata
	converter               AttributeValueConverter
	marshaler               marshal.MarshalerInterface
	ctx                     context.Context
	model                   any
	exclusive               map[string]types.AttributeValue
	retryConfig             *RetryConfig
	totalSegments           *int32
	segment                 *int32
	builder                 *expr.Builder
	offset                  *int
	orderBy                 OrderBy
	index                   string
	projection              []string
	rawFilters              []RawFilter
	filters                 []Filter
	rawConditionExpressions []conditionExpression
	writeConditions         []Condition
	conditions              []Condition
	limit                   int
	consistentRead          bool
}

// Condition represents a query condition
type Condition struct {
	Value    any
	Field    string
	Operator string
}

type conditionExpression struct {
	Values     map[string]any
	Expression string
}

// AttributeValueConverter allows TableTheory callers to inject custom converter behavior.
// It intentionally mirrors the relevant subset of `pkg/types.Converter` without requiring
// callers to depend on that concrete type.
type AttributeValueConverter interface {
	HasCustomConverter(typ reflect.Type) bool
	ToAttributeValue(value any) (types.AttributeValue, error)
	FromAttributeValue(av types.AttributeValue, target any) error
	ConvertToSet(slice any, isSet bool) (types.AttributeValue, error)
}

type rawMetadataProvider interface {
	RawMetadata() *model.Metadata
}

type executorContextSetter interface {
	SetContext(ctx context.Context)
}

// normalizeCondition resolves a condition's field to its canonical DynamoDB attribute name
// and returns the normalized condition along with the Go field name and DynamoDB attribute name.
func (q *Query) normalizeCondition(cond Condition) (Condition, string, string) {
	normalized := cond
	goField := cond.Field
	attrName := cond.Field

	if q.metadata != nil {
		if meta := q.metadata.AttributeMetadata(cond.Field); meta != nil {
			goField = meta.Name
			if meta.DynamoDBName != "" {
				attrName = meta.DynamoDBName
			} else {
				attrName = meta.Name
			}
			normalized.Field = attrName
		}
	}

	return normalized, goField, attrName
}

func (q *Query) rejectEncryptedConditionField(field string) error {
	if q == nil || q.metadata == nil || field == "" {
		return nil
	}

	meta := q.metadata.AttributeMetadata(field)
	if meta == nil || len(meta.Tags) == 0 {
		return nil
	}

	if _, ok := meta.Tags["encrypted"]; !ok {
		return nil
	}

	name := meta.Name
	if name == "" {
		name = field
	}

	return fmt.Errorf("%w: %s", theorydbErrors.ErrEncryptedFieldNotQueryable, name)
}

// addPrimaryKeyCondition appends a condition targeting the table primary key
func (q *Query) addPrimaryKeyCondition(operator string) {
	if q.metadata == nil {
		q.recordBuilderError(fmt.Errorf("metadata is required for conditional helpers"))
		return
	}

	primaryKey := q.metadata.PrimaryKey()
	if primaryKey.PartitionKey == "" {
		q.recordBuilderError(fmt.Errorf("partition key is required for conditional helpers"))
		return
	}

	attrName := q.resolveAttributeName(primaryKey.PartitionKey)
	q.writeConditions = append(q.writeConditions, Condition{
		Field:    attrName,
		Operator: operator,
	})

	if primaryKey.SortKey != "" && operator == "attribute_exists" {
		// attribute_exists(sortKey) ensures full item presence for composite keys
		sortAttr := q.resolveAttributeName(primaryKey.SortKey)
		q.writeConditions = append(q.writeConditions, Condition{
			Field:    sortAttr,
			Operator: operator,
		})
	}
}

// resolveAttributeName maps a Go struct field to its DynamoDB attribute name
func (q *Query) resolveAttributeName(field string) string {
	if q.metadata == nil || field == "" {
		return field
	}

	if meta := q.metadata.AttributeMetadata(field); meta != nil {
		if meta.DynamoDBName != "" {
			return meta.DynamoDBName
		}
		if meta.Name != "" {
			return meta.Name
		}
	}
	return field
}

func (q *Query) resolveGoFieldName(field string) string {
	if q.metadata == nil || field == "" {
		return field
	}
	if meta := q.metadata.AttributeMetadata(field); meta != nil && meta.Name != "" {
		return meta.Name
	}
	return field
}

func (q *Query) attributeMetadata(field string) *core.AttributeMetadata {
	if q == nil || q.metadata == nil || field == "" {
		return nil
	}
	return q.metadata.AttributeMetadata(field)
}

func (q *Query) rawFieldMetadata(field string) *model.FieldMetadata {
	if q == nil || q.rawMetadata == nil || field == "" {
		return nil
	}

	if meta := q.rawMetadata.Fields[field]; meta != nil {
		return meta
	}
	if meta := q.rawMetadata.FieldsByDBName[field]; meta != nil {
		return meta
	}

	attrMeta := q.attributeMetadata(field)
	if attrMeta == nil {
		return nil
	}

	if attrMeta.Name != "" {
		if meta := q.rawMetadata.Fields[attrMeta.Name]; meta != nil {
			return meta
		}
	}
	if attrMeta.DynamoDBName != "" {
		if meta := q.rawMetadata.FieldsByDBName[attrMeta.DynamoDBName]; meta != nil {
			return meta
		}
	}

	return nil
}

func (q *Query) normalizeJSONFieldValue(field string, value any) (any, error) {
	attrMeta := q.attributeMetadata(field)
	if attrMeta == nil || !fieldcodec.HasJSONTag(attrMeta.Tags) {
		return value, nil
	}

	var fieldType reflect.Type
	if rawMeta := q.rawFieldMetadata(field); rawMeta != nil {
		fieldType = rawMeta.Type
	}

	return fieldcodec.NormalizeJSONFieldValue(fieldType, value)
}

func cloneConditionValues(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(values))
	for k, v := range values {
		cloned[k] = v
	}
	return cloned
}

func (q *Query) buildConditionExpression(builder *expr.Builder, includeWhereConditions bool, skipKeyConditions bool, defaultIfEmpty bool) (string, map[string]string, map[string]types.AttributeValue, error) {
	if builder == nil {
		builder = q.newBuilder()
	}
	hasCondition, err := q.addWriteConditions(builder)
	if err != nil {
		return "", nil, nil, err
	}

	if includeWhereConditions {
		added, whereErr := q.addWhereConditions(builder, skipKeyConditions)
		if whereErr != nil {
			return "", nil, nil, whereErr
		}
		hasCondition = hasCondition || added
	}

	if defaultIfEmpty && !hasCondition && len(q.rawConditionExpressions) == 0 {
		if defaultErr := q.addDefaultCondition(builder); defaultErr != nil {
			return "", nil, nil, defaultErr
		}
	}

	components := builder.Build()
	conditionExpr := components.ConditionExpression
	names := components.ExpressionAttributeNames
	values := components.ExpressionAttributeValues

	mergedExpr, mergedValues, err := mergeConditionExpressions(conditionExpr, values, q.rawConditionExpressions, q.converter)
	if err != nil {
		return "", nil, nil, err
	}

	return mergedExpr, names, mergedValues, nil
}

func (q *Query) addWriteConditions(builder *expr.Builder) (bool, error) {
	hasCondition := false
	for _, cond := range q.writeConditions {
		if cond.Field == "" {
			return false, fmt.Errorf("condition field cannot be empty")
		}
		if err := q.rejectEncryptedConditionField(cond.Field); err != nil {
			return false, err
		}
		if err := builder.AddConditionExpression(cond.Field, cond.Operator, cond.Value); err != nil {
			return false, fmt.Errorf("failed to add condition for %s: %w", cond.Field, err)
		}
		hasCondition = true
	}
	return hasCondition, nil
}

func (q *Query) addWhereConditions(builder *expr.Builder, skipKeyConditions bool) (bool, error) {
	if q.metadata == nil {
		return false, fmt.Errorf("model metadata is required for conditional operations")
	}
	primaryKey := q.metadata.PrimaryKey()

	hasCondition := false
	for _, original := range q.conditions {
		if err := q.rejectEncryptedConditionField(original.Field); err != nil {
			return false, err
		}
		normalized, goField, attrName := q.normalizeCondition(original)
		if skipKeyConditions && q.isKeyField(primaryKey, goField, attrName) {
			continue
		}
		if err := builder.AddConditionExpression(normalized.Field, normalized.Operator, normalized.Value); err != nil {
			return false, fmt.Errorf("failed to add condition for %s: %w", normalized.Field, err)
		}
		hasCondition = true
	}
	return hasCondition, nil
}

func (q *Query) addDefaultCondition(builder *expr.Builder) error {
	if q.metadata == nil {
		return fmt.Errorf("model metadata is required for conditional operations")
	}
	pk := q.metadata.PrimaryKey()
	if pk.PartitionKey == "" {
		return fmt.Errorf("partition key is required for default condition")
	}
	if err := builder.AddConditionExpression(q.resolveAttributeName(pk.PartitionKey), "attribute_not_exists", nil); err != nil {
		return fmt.Errorf("failed to add default condition: %w", err)
	}
	return nil
}

func mergeConditionExpressions(baseExpr string, baseValues map[string]types.AttributeValue, rawExpressions []conditionExpression, converter AttributeValueConverter) (string, map[string]types.AttributeValue, error) {
	mergedExpr := baseExpr
	mergedValues := baseValues

	for _, raw := range rawExpressions {
		if raw.Expression == "" {
			continue
		}
		if mergedExpr == "" {
			mergedExpr = raw.Expression
		} else {
			mergedExpr = fmt.Sprintf("(%s) AND (%s)", mergedExpr, raw.Expression)
		}

		if len(raw.Values) == 0 {
			continue
		}

		if mergedValues == nil {
			mergedValues = make(map[string]types.AttributeValue)
		}

		for key, val := range raw.Values {
			if _, exists := mergedValues[key]; exists {
				return "", nil, fmt.Errorf("duplicate placeholder %s in condition expression", key)
			}
			var av types.AttributeValue
			var err error
			if converter != nil {
				av, err = converter.ToAttributeValue(val)
			} else {
				av, err = expr.ConvertToAttributeValue(val)
			}
			if err != nil {
				return "", nil, fmt.Errorf("failed to convert condition value %s: %w", key, err)
			}
			mergedValues[key] = av
		}
	}

	return mergedExpr, mergedValues, nil
}

func (q *Query) isKeyField(schema core.KeySchema, goField, attrName string) bool {
	if schema.PartitionKey != "" {
		if strings.EqualFold(goField, schema.PartitionKey) || strings.EqualFold(attrName, q.resolveAttributeName(schema.PartitionKey)) {
			return true
		}
	}
	if schema.SortKey != "" {
		if strings.EqualFold(goField, schema.SortKey) || strings.EqualFold(attrName, q.resolveAttributeName(schema.SortKey)) {
			return true
		}
	}
	return false
}

// Filter represents a filter expression
type Filter struct {
	Params     map[string]any
	Expression string
}

// RawFilter represents a raw filter with parameters
type RawFilter struct {
	Expression string
	Params     []core.Param
}

// OrderBy represents ordering configuration
type OrderBy struct {
	Field string
	Order string // "asc" or "desc"
}

// RetryConfig configures retry behavior
type RetryConfig struct {
	MaxRetries   int
	InitialDelay time.Duration
}

// QueryExecutor is the base query executor interface
type QueryExecutor interface {
	ExecuteQuery(input *core.CompiledQuery, dest any) error
	ExecuteScan(input *core.CompiledQuery, dest any) error
}

// PaginatedQueryExecutor extends QueryExecutor with pagination support
type PaginatedQueryExecutor interface {
	QueryExecutor
	ExecuteQueryWithPagination(input *core.CompiledQuery, dest any) (*QueryResult, error)
	ExecuteScanWithPagination(input *core.CompiledQuery, dest any) (*ScanResult, error)
}

// GetItemExecutor extends QueryExecutor with GetItem support.
type GetItemExecutor interface {
	QueryExecutor
	ExecuteGetItem(input *core.CompiledQuery, key map[string]types.AttributeValue, dest any) error
}

// PutItemExecutor extends QueryExecutor with PutItem support
type PutItemExecutor interface {
	QueryExecutor
	ExecutePutItem(input *core.CompiledQuery, item map[string]types.AttributeValue) error
}

// UpdateItemExecutor extends QueryExecutor with UpdateItem support
type UpdateItemExecutor interface {
	QueryExecutor
	ExecuteUpdateItem(input *core.CompiledQuery, key map[string]types.AttributeValue) error
}

// UpdateItemWithResultExecutor extends UpdateItemExecutor with result support
type UpdateItemWithResultExecutor interface {
	UpdateItemExecutor
	ExecuteUpdateItemWithResult(input *core.CompiledQuery, key map[string]types.AttributeValue) (*core.UpdateResult, error)
}

// DeleteItemExecutor extends QueryExecutor with DeleteItem support
type DeleteItemExecutor interface {
	QueryExecutor
	ExecuteDeleteItem(input *core.CompiledQuery, key map[string]types.AttributeValue) error
}

// BatchWriteItemExecutor extends QueryExecutor with BatchWriteItem support
type BatchWriteItemExecutor interface {
	QueryExecutor
	ExecuteBatchWriteItem(tableName string, writeRequests []types.WriteRequest) (*core.BatchWriteResult, error)
}

type preparedBatchWriteItemExecutor interface {
	PrepareBatchWriteItems(writeRequests []types.WriteRequest) ([]types.WriteRequest, error)
	ExecuteBatchWriteItemRaw(tableName string, writeRequests []types.WriteRequest) (*core.BatchWriteResult, error)
}

// New creates a new Query instance
func New(model any, metadata core.ModelMetadata, executor QueryExecutor) *Query {
	q := &Query{
		model:                   model,
		metadata:                metadata,
		executor:                executor,
		ctx:                     context.Background(),
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

// Where adds a condition to the query
func (q *Query) Where(field string, op string, value any) core.Query {
	if err := q.rejectEncryptedConditionField(field); err != nil {
		q.recordBuilderError(err)
		return q
	}
	normalized, err := q.normalizeJSONFieldValue(field, value)
	if err != nil {
		q.recordBuilderError(err)
		return q
	}
	q.conditions = append(q.conditions, Condition{
		Field:    field,
		Operator: op,
		Value:    normalized,
	})
	return q
}

// Filter adds a filter expression to the query
func (q *Query) Filter(field string, op string, value any) core.Query {
	return q.addFilterCondition("AND", field, op, value)
}

func (q *Query) addFilterCondition(logicalOperator, field, op string, value any) core.Query {
	if err := q.rejectEncryptedConditionField(field); err != nil {
		q.recordBuilderError(err)
		return q
	}

	if q.builder == nil {
		q.builder = q.newBuilder()
	}

	normalized, err := q.normalizeJSONFieldValue(field, value)
	if err != nil {
		q.recordBuilderError(err)
		return q
	}

	if err := q.builder.AddFilterCondition(logicalOperator, q.resolveAttributeName(field), op, normalized); err != nil {
		q.recordBuilderError(err)
	}
	return q
}

// Index specifies which index to use
func (q *Query) Index(name string) core.Query {
	q.index = name
	return q
}

// Limit sets the maximum number of items to return
func (q *Query) Limit(n int) core.Query {
	q.limit = n
	return q
}

// Offset sets the starting position for the query
func (q *Query) Offset(offset int) core.Query {
	q.offset = &offset
	return q
}

// OrderBy sets the sort order
func (q *Query) OrderBy(field string, order string) core.Query {
	q.orderBy = OrderBy{
		Field: field,
		Order: order,
	}
	return q
}

// Select specifies which fields to return
func (q *Query) Select(fields ...string) core.Query {
	if len(fields) == 0 {
		q.projection = nil
		return q
	}

	resolved := make([]string, 0, len(fields))
	for _, field := range fields {
		resolved = append(resolved, q.resolveAttributeName(field))
	}
	q.projection = resolved
	return q
}

// ConsistentRead enables strongly consistent reads for Query operations
func (q *Query) ConsistentRead() core.Query {
	q.consistentRead = true
	return q
}

// WithRetry configures retry behavior for eventually consistent reads
func (q *Query) WithRetry(maxRetries int, initialDelay time.Duration) core.Query {
	q.retryConfig = &RetryConfig{
		MaxRetries:   maxRetries,
		InitialDelay: initialDelay,
	}
	return q
}

// First executes the query and returns the first result
