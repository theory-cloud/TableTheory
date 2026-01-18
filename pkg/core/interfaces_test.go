package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func mustQuery(v any) Query {
	if v == nil {
		return nil
	}
	q, ok := v.(Query)
	if !ok {
		panic("unexpected type: expected core.Query")
	}
	return q
}

func mustDB(v any) DB {
	if v == nil {
		return nil
	}
	db, ok := v.(DB)
	if !ok {
		panic("unexpected type: expected core.DB")
	}
	return db
}

func mustPaginatedResult(v any) *PaginatedResult {
	if v == nil {
		return nil
	}
	result, ok := v.(*PaginatedResult)
	if !ok {
		panic("unexpected type: expected *core.PaginatedResult")
	}
	return result
}

func mustInt64(v any) int64 {
	n, ok := v.(int64)
	if !ok {
		panic("unexpected type: expected int64")
	}
	return n
}

func mustUpdateBuilder(v any) UpdateBuilder {
	if v == nil {
		return nil
	}
	builder, ok := v.(UpdateBuilder)
	if !ok {
		panic("unexpected type: expected core.UpdateBuilder")
	}
	return builder
}

func mustBatchGetBuilder(v any) BatchGetBuilder {
	if v == nil {
		return nil
	}
	builder, ok := v.(BatchGetBuilder)
	if !ok {
		panic("unexpected type: expected core.BatchGetBuilder")
	}
	return builder
}

func mustKeySchema(v any) KeySchema {
	schema, ok := v.(KeySchema)
	if !ok {
		panic("unexpected type: expected core.KeySchema")
	}
	return schema
}

func mustIndexSchemas(v any) []IndexSchema {
	schemas, ok := v.([]IndexSchema)
	if !ok {
		panic("unexpected type: expected []core.IndexSchema")
	}
	return schemas
}

func mustAttributeMetadata(v any) *AttributeMetadata {
	if v == nil {
		return nil
	}
	meta, ok := v.(*AttributeMetadata)
	if !ok {
		panic("unexpected type: expected *core.AttributeMetadata")
	}
	return meta
}

// MockDB is a mock implementation of the DB interface
type MockDB struct {
	mock.Mock
}

func (m *MockDB) Model(model any) Query {
	args := m.Called(model)
	return mustQuery(args.Get(0))
}

func (m *MockDB) Transaction(fn func(tx *Tx) error) error {
	args := m.Called(fn)
	return args.Error(0)
}

func (m *MockDB) Migrate() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockDB) AutoMigrate(models ...any) error {
	args := m.Called(models)
	return args.Error(0)
}

func (m *MockDB) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockDB) WithContext(ctx context.Context) DB {
	args := m.Called(ctx)
	return mustDB(args.Get(0))
}

// MockQuery is a mock implementation of the Query interface
type MockQuery struct {
	mock.Mock
}

func (m *MockQuery) Where(field string, op string, value any) Query {
	args := m.Called(field, op, value)
	return mustQuery(args.Get(0))
}

func (m *MockQuery) Index(indexName string) Query {
	args := m.Called(indexName)
	return mustQuery(args.Get(0))
}

func (m *MockQuery) Filter(field string, op string, value any) Query {
	args := m.Called(field, op, value)
	return mustQuery(args.Get(0))
}

func (m *MockQuery) OrFilter(field string, op string, value any) Query {
	args := m.Called(field, op, value)
	return mustQuery(args.Get(0))
}

func (m *MockQuery) FilterGroup(fn func(Query)) Query {
	args := m.Called(fn)
	return mustQuery(args.Get(0))
}

func (m *MockQuery) OrFilterGroup(fn func(Query)) Query {
	args := m.Called(fn)
	return mustQuery(args.Get(0))
}

func (m *MockQuery) IfNotExists() Query {
	args := m.Called()
	return mustQuery(args.Get(0))
}

func (m *MockQuery) IfExists() Query {
	args := m.Called()
	return mustQuery(args.Get(0))
}

func (m *MockQuery) WithCondition(field, operator string, value any) Query {
	args := m.Called(field, operator, value)
	return mustQuery(args.Get(0))
}

func (m *MockQuery) WithConditionExpression(expr string, values map[string]any) Query {
	args := m.Called(expr, values)
	return mustQuery(args.Get(0))
}

func (m *MockQuery) OrderBy(field string, order string) Query {
	args := m.Called(field, order)
	return mustQuery(args.Get(0))
}

func (m *MockQuery) Limit(limit int) Query {
	args := m.Called(limit)
	return mustQuery(args.Get(0))
}

func (m *MockQuery) Offset(offset int) Query {
	args := m.Called(offset)
	return mustQuery(args.Get(0))
}

func (m *MockQuery) Select(fields ...string) Query {
	args := m.Called(fields)
	return mustQuery(args.Get(0))
}

func (m *MockQuery) ConsistentRead() Query {
	args := m.Called()
	return mustQuery(args.Get(0))
}

func (m *MockQuery) WithRetry(maxRetries int, initialDelay time.Duration) Query {
	args := m.Called(maxRetries, initialDelay)
	return mustQuery(args.Get(0))
}

func (m *MockQuery) First(dest any) error {
	args := m.Called(dest)
	return args.Error(0)
}

func (m *MockQuery) All(dest any) error {
	args := m.Called(dest)
	return args.Error(0)
}

func (m *MockQuery) AllPaginated(dest any) (*PaginatedResult, error) {
	args := m.Called(dest)
	return mustPaginatedResult(args.Get(0)), args.Error(1)
}

func (m *MockQuery) Count() (int64, error) {
	args := m.Called()
	return mustInt64(args.Get(0)), args.Error(1)
}

func (m *MockQuery) Create() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockQuery) CreateOrUpdate() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockQuery) Update(fields ...string) error {
	args := m.Called(fields)
	return args.Error(0)
}

func (m *MockQuery) UpdateBuilder() UpdateBuilder {
	args := m.Called()
	return mustUpdateBuilder(args.Get(0))
}

func (m *MockQuery) Delete() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockQuery) Scan(dest any) error {
	args := m.Called(dest)
	return args.Error(0)
}

func (m *MockQuery) ParallelScan(segment int32, totalSegments int32) Query {
	args := m.Called(segment, totalSegments)
	return mustQuery(args.Get(0))
}

func (m *MockQuery) ScanAllSegments(dest any, totalSegments int32) error {
	args := m.Called(dest, totalSegments)
	return args.Error(0)
}

func (m *MockQuery) BatchGet(keys []any, dest any) error {
	args := m.Called(keys, dest)
	return args.Error(0)
}

func (m *MockQuery) BatchGetWithOptions(keys []any, dest any, opts *BatchGetOptions) error {
	args := m.Called(keys, dest, opts)
	return args.Error(0)
}

func (m *MockQuery) BatchGetBuilder() BatchGetBuilder {
	args := m.Called()
	return mustBatchGetBuilder(args.Get(0))
}

func (m *MockQuery) BatchCreate(items any) error {
	args := m.Called(items)
	return args.Error(0)
}

func (m *MockQuery) BatchDelete(keys []any) error {
	args := m.Called(keys)
	return args.Error(0)
}

func (m *MockQuery) BatchWrite(putItems []any, deleteKeys []any) error {
	args := m.Called(putItems, deleteKeys)
	return args.Error(0)
}

func (m *MockQuery) BatchUpdateWithOptions(items []any, fields []string, options ...any) error {
	args := m.Called(items, fields, options)
	return args.Error(0)
}

func (m *MockQuery) Cursor(cursor string) Query {
	args := m.Called(cursor)
	return mustQuery(args.Get(0))
}

func (m *MockQuery) SetCursor(cursor string) error {
	args := m.Called(cursor)
	return args.Error(0)
}

func (m *MockQuery) WithContext(ctx context.Context) Query {
	args := m.Called(ctx)
	return mustQuery(args.Get(0))
}

// MockUpdateBuilder is a mock implementation of the UpdateBuilder interface
type MockUpdateBuilder struct {
	mock.Mock
}

func (m *MockUpdateBuilder) Set(field string, value any) UpdateBuilder {
	args := m.Called(field, value)
	return mustUpdateBuilder(args.Get(0))
}

func (m *MockUpdateBuilder) SetIfNotExists(field string, value any, defaultValue any) UpdateBuilder {
	args := m.Called(field, value, defaultValue)
	return mustUpdateBuilder(args.Get(0))
}

func (m *MockUpdateBuilder) Add(field string, value any) UpdateBuilder {
	args := m.Called(field, value)
	return mustUpdateBuilder(args.Get(0))
}

func (m *MockUpdateBuilder) Increment(field string) UpdateBuilder {
	args := m.Called(field)
	return mustUpdateBuilder(args.Get(0))
}

func (m *MockUpdateBuilder) Decrement(field string) UpdateBuilder {
	args := m.Called(field)
	return mustUpdateBuilder(args.Get(0))
}

func (m *MockUpdateBuilder) Remove(field string) UpdateBuilder {
	args := m.Called(field)
	return mustUpdateBuilder(args.Get(0))
}

func (m *MockUpdateBuilder) Delete(field string, value any) UpdateBuilder {
	args := m.Called(field, value)
	return mustUpdateBuilder(args.Get(0))
}

func (m *MockUpdateBuilder) AppendToList(field string, values any) UpdateBuilder {
	args := m.Called(field, values)
	return mustUpdateBuilder(args.Get(0))
}

func (m *MockUpdateBuilder) PrependToList(field string, values any) UpdateBuilder {
	args := m.Called(field, values)
	return mustUpdateBuilder(args.Get(0))
}

func (m *MockUpdateBuilder) RemoveFromListAt(field string, index int) UpdateBuilder {
	args := m.Called(field, index)
	return mustUpdateBuilder(args.Get(0))
}

func (m *MockUpdateBuilder) SetListElement(field string, index int, value any) UpdateBuilder {
	args := m.Called(field, index, value)
	return mustUpdateBuilder(args.Get(0))
}

func (m *MockUpdateBuilder) Condition(field string, operator string, value any) UpdateBuilder {
	return m
}

func (m *MockUpdateBuilder) OrCondition(field string, operator string, value any) UpdateBuilder {
	return m
}

func (m *MockUpdateBuilder) ConditionExists(field string) UpdateBuilder {
	args := m.Called(field)
	return mustUpdateBuilder(args.Get(0))
}

func (m *MockUpdateBuilder) ConditionNotExists(field string) UpdateBuilder {
	args := m.Called(field)
	return mustUpdateBuilder(args.Get(0))
}

func (m *MockUpdateBuilder) ConditionVersion(currentVersion int64) UpdateBuilder {
	args := m.Called(currentVersion)
	return mustUpdateBuilder(args.Get(0))
}

func (m *MockUpdateBuilder) ReturnValues(option string) UpdateBuilder {
	args := m.Called(option)
	return mustUpdateBuilder(args.Get(0))
}

func (m *MockUpdateBuilder) Execute() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockUpdateBuilder) ExecuteWithResult(result any) error {
	args := m.Called(result)
	return args.Error(0)
}

// TestTx tests the Tx transaction type
func TestTx(t *testing.T) {
	t.Run("Model", func(t *testing.T) {
		mockDB := new(MockDB)
		tx := &Tx{db: mockDB}
		mockQuery := new(MockQuery)
		model := struct{ ID string }{ID: "123"}

		mockDB.On("Model", model).Return(mockQuery)

		result := tx.Model(model)
		assert.Equal(t, mockQuery, result)
		mockDB.AssertExpectations(t)
	})

	t.Run("Create", func(t *testing.T) {
		mockDB := new(MockDB)
		tx := &Tx{db: mockDB}
		mockQuery := new(MockQuery)
		model := struct{ ID string }{ID: "123"}

		mockDB.On("Model", model).Return(mockQuery)
		mockQuery.On("Create").Return(nil)

		err := tx.Create(model)
		assert.NoError(t, err)
		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("Create with error", func(t *testing.T) {
		mockDB := new(MockDB)
		tx := &Tx{db: mockDB}
		mockQuery := new(MockQuery)
		model := struct{ ID string }{ID: "123"}
		expectedErr := errors.New("create failed")

		mockDB.On("Model", model).Return(mockQuery)
		mockQuery.On("Create").Return(expectedErr)

		err := tx.Create(model)
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("Update", func(t *testing.T) {
		mockDB := new(MockDB)
		tx := &Tx{db: mockDB}
		mockQuery := new(MockQuery)
		model := struct{ ID string }{ID: "123"}
		fields := []string{"name", "email"}

		mockDB.On("Model", model).Return(mockQuery)
		mockQuery.On("Update", fields).Return(nil)

		err := tx.Update(model, fields...)
		assert.NoError(t, err)
		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("Update without fields", func(t *testing.T) {
		mockDB := new(MockDB)
		tx := &Tx{db: mockDB}
		mockQuery := new(MockQuery)
		model := struct{ ID string }{ID: "123"}

		mockDB.On("Model", model).Return(mockQuery)
		mockQuery.On("Update", mock.MatchedBy(func(fields []string) bool {
			return len(fields) == 0
		})).Return(nil)

		err := tx.Update(model)
		assert.NoError(t, err)
	})

	t.Run("Delete", func(t *testing.T) {
		mockDB := new(MockDB)
		tx := &Tx{db: mockDB}
		mockQuery := new(MockQuery)
		model := struct{ ID string }{ID: "123"}

		mockDB.On("Model", model).Return(mockQuery)
		mockQuery.On("Delete").Return(nil)

		err := tx.Delete(model)
		assert.NoError(t, err)
		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("Delete with error", func(t *testing.T) {
		mockDB := new(MockDB)
		tx := &Tx{db: mockDB}
		mockQuery := new(MockQuery)
		model := struct{ ID string }{ID: "123"}
		expectedErr := errors.New("delete failed")

		mockDB.On("Model", model).Return(mockQuery)
		mockQuery.On("Delete").Return(expectedErr)

		err := tx.Delete(model)
		assert.ErrorIs(t, err, expectedErr)
	})
}

// TestPaginatedResult tests the PaginatedResult struct
func TestPaginatedResult(t *testing.T) {
	t.Run("Basic fields", func(t *testing.T) {
		items := []string{"item1", "item2", "item3"}
		lastKey := map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: "123"},
		}

		result := &PaginatedResult{
			Items:            items,
			Count:            3,
			ScannedCount:     5,
			LastEvaluatedKey: lastKey,
			NextCursor:       "eyJpZCI6IjEyMyJ9",
			HasMore:          true,
		}

		assert.Equal(t, items, result.Items)
		assert.Equal(t, 3, result.Count)
		assert.Equal(t, 5, result.ScannedCount)
		assert.Equal(t, lastKey, result.LastEvaluatedKey)
		assert.Equal(t, "eyJpZCI6IjEyMyJ9", result.NextCursor)
		assert.True(t, result.HasMore)
	})

	t.Run("Empty result", func(t *testing.T) {
		result := &PaginatedResult{
			Items:            []string{},
			Count:            0,
			ScannedCount:     0,
			LastEvaluatedKey: nil,
			NextCursor:       "",
			HasMore:          false,
		}

		assert.Empty(t, result.Items)
		assert.Zero(t, result.Count)
		assert.Zero(t, result.ScannedCount)
		assert.Nil(t, result.LastEvaluatedKey)
		assert.Empty(t, result.NextCursor)
		assert.False(t, result.HasMore)
	})
}

// TestParam tests the Param struct
func TestParam(t *testing.T) {
	param := Param{
		Name:  "userId",
		Value: "12345",
	}

	assert.Equal(t, "userId", param.Name)
	assert.Equal(t, "12345", param.Value)

	// Test with different value types
	paramInt := Param{Name: "age", Value: 30}
	assert.Equal(t, "age", paramInt.Name)
	assert.Equal(t, 30, paramInt.Value)

	paramBool := Param{Name: "active", Value: true}
	assert.Equal(t, "active", paramBool.Name)
	assert.Equal(t, true, paramBool.Value)

	paramNil := Param{Name: "optional", Value: nil}
	assert.Equal(t, "optional", paramNil.Name)
	assert.Nil(t, paramNil.Value)
}

// TestCompiledQuery tests the CompiledQuery struct
func TestCompiledQuery(t *testing.T) {
	t.Run("Query operation", func(t *testing.T) {
		limit := int32(10)
		scanForward := true
		offset := 5

		cq := &CompiledQuery{
			Operation:              "Query",
			TableName:              "users",
			IndexName:              "email-index",
			KeyConditionExpression: "email = :email",
			FilterExpression:       "age > :age",
			ProjectionExpression:   "id, email, name",
			ExpressionAttributeNames: map[string]string{
				"#name": "name",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":email": &types.AttributeValueMemberS{Value: "test@example.com"},
				":age":   &types.AttributeValueMemberN{Value: "18"},
			},
			Limit:            &limit,
			ScanIndexForward: &scanForward,
			Select:           "SPECIFIC_ATTRIBUTES",
			Offset:           &offset,
		}

		assert.Equal(t, "Query", cq.Operation)
		assert.Equal(t, "users", cq.TableName)
		assert.Equal(t, "email-index", cq.IndexName)
		assert.Equal(t, "email = :email", cq.KeyConditionExpression)
		assert.Equal(t, "age > :age", cq.FilterExpression)
		assert.Equal(t, "id, email, name", cq.ProjectionExpression)
		assert.Equal(t, "name", cq.ExpressionAttributeNames["#name"])
		assert.Equal(t, &types.AttributeValueMemberS{Value: "test@example.com"}, cq.ExpressionAttributeValues[":email"])
		assert.Equal(t, &types.AttributeValueMemberN{Value: "18"}, cq.ExpressionAttributeValues[":age"])
		assert.Equal(t, int32(10), *cq.Limit)
		assert.True(t, *cq.ScanIndexForward)
		assert.Equal(t, "SPECIFIC_ATTRIBUTES", cq.Select)
		assert.Equal(t, 5, *cq.Offset)
	})

	t.Run("Scan operation with parallel scan", func(t *testing.T) {
		segment := int32(2)
		totalSegments := int32(4)

		cq := &CompiledQuery{
			Operation:        "Scan",
			TableName:        "products",
			FilterExpression: "price > :min_price",
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":min_price": &types.AttributeValueMemberN{Value: "100"},
			},
			Segment:       &segment,
			TotalSegments: &totalSegments,
		}

		assert.Equal(t, "Scan", cq.Operation)
		assert.Equal(t, "products", cq.TableName)
		assert.Equal(t, "price > :min_price", cq.FilterExpression)
		assert.Equal(t, &types.AttributeValueMemberN{Value: "100"}, cq.ExpressionAttributeValues[":min_price"])
		assert.Equal(t, int32(2), *cq.Segment)
		assert.Equal(t, int32(4), *cq.TotalSegments)
	})

	t.Run("Update operation", func(t *testing.T) {
		cq := &CompiledQuery{
			Operation:           "UpdateItem",
			TableName:           "orders",
			UpdateExpression:    "SET #status = :status, updated_at = :now",
			ConditionExpression: "attribute_exists(id)",
			ExpressionAttributeNames: map[string]string{
				"#status": "status",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":status": &types.AttributeValueMemberS{Value: "completed"},
				":now":    &types.AttributeValueMemberS{Value: "2023-06-15T10:30:00Z"},
			},
		}

		assert.Equal(t, "UpdateItem", cq.Operation)
		assert.Equal(t, "orders", cq.TableName)
		assert.Equal(t, "SET #status = :status, updated_at = :now", cq.UpdateExpression)
		assert.Equal(t, "attribute_exists(id)", cq.ConditionExpression)
		assert.Equal(t, "status", cq.ExpressionAttributeNames["#status"])
		assert.Equal(t, &types.AttributeValueMemberS{Value: "completed"}, cq.ExpressionAttributeValues[":status"])
		assert.Equal(t, &types.AttributeValueMemberS{Value: "2023-06-15T10:30:00Z"}, cq.ExpressionAttributeValues[":now"])
	})

	t.Run("Empty compiled query", func(t *testing.T) {
		cq := &CompiledQuery{}

		assert.Empty(t, cq.Operation)
		assert.Empty(t, cq.TableName)
		assert.Nil(t, cq.Limit)
		assert.Nil(t, cq.ScanIndexForward)
		assert.Nil(t, cq.Segment)
	})
}

// TestKeySchema tests the KeySchema struct
func TestKeySchema(t *testing.T) {
	t.Run("Simple primary key", func(t *testing.T) {
		ks := KeySchema{
			PartitionKey: "id",
		}

		assert.Equal(t, "id", ks.PartitionKey)
		assert.Empty(t, ks.SortKey)
	})

	t.Run("Composite primary key", func(t *testing.T) {
		ks := KeySchema{
			PartitionKey: "userId",
			SortKey:      "timestamp",
		}

		assert.Equal(t, "userId", ks.PartitionKey)
		assert.Equal(t, "timestamp", ks.SortKey)
	})
}

// TestIndexSchema tests the IndexSchema struct
func TestIndexSchema(t *testing.T) {
	t.Run("GSI with projection", func(t *testing.T) {
		idx := IndexSchema{
			Name:            "email-index",
			Type:            "GSI",
			PartitionKey:    "email",
			SortKey:         "created_at",
			ProjectionType:  "INCLUDE",
			ProjectedFields: []string{"name", "status"},
		}

		assert.Equal(t, "email-index", idx.Name)
		assert.Equal(t, "GSI", idx.Type)
		assert.Equal(t, "email", idx.PartitionKey)
		assert.Equal(t, "created_at", idx.SortKey)
		assert.Equal(t, "INCLUDE", idx.ProjectionType)
		assert.Len(t, idx.ProjectedFields, 2)
		assert.Contains(t, idx.ProjectedFields, "name")
		assert.Contains(t, idx.ProjectedFields, "status")
	})

	t.Run("LSI with ALL projection", func(t *testing.T) {
		idx := IndexSchema{
			Name:           "status-index",
			Type:           "LSI",
			PartitionKey:   "id",
			SortKey:        "status",
			ProjectionType: "ALL",
		}

		assert.Equal(t, "status-index", idx.Name)
		assert.Equal(t, "LSI", idx.Type)
		assert.Equal(t, "id", idx.PartitionKey)
		assert.Equal(t, "status", idx.SortKey)
		assert.Equal(t, "ALL", idx.ProjectionType)
		assert.Empty(t, idx.ProjectedFields)
	})
}

// TestAttributeMetadata tests the AttributeMetadata struct
func TestAttributeMetadata(t *testing.T) {
	t.Run("Basic attribute", func(t *testing.T) {
		attr := &AttributeMetadata{
			Name:         "UserEmail",
			Type:         "string",
			DynamoDBName: "email",
			Tags: map[string]string{
				"dynamodb": "email",
				"json":     "user_email",
			},
		}

		assert.Equal(t, "UserEmail", attr.Name)
		assert.Equal(t, "string", attr.Type)
		assert.Equal(t, "email", attr.DynamoDBName)
		assert.Len(t, attr.Tags, 2)
		assert.Equal(t, "email", attr.Tags["dynamodb"])
		assert.Equal(t, "user_email", attr.Tags["json"])
	})

	t.Run("Attribute without tags", func(t *testing.T) {
		attr := &AttributeMetadata{
			Name:         "ID",
			Type:         "string",
			DynamoDBName: "id",
		}

		assert.Equal(t, "ID", attr.Name)
		assert.Equal(t, "string", attr.Type)
		assert.Equal(t, "id", attr.DynamoDBName)
		assert.Nil(t, attr.Tags)
	})
}

// TestInterfaceCompliance verifies that our mocks implement the interfaces correctly
func TestInterfaceCompliance(t *testing.T) {
	t.Run("MockDB implements DB", func(t *testing.T) {
		var _ DB = (*MockDB)(nil)
	})

	t.Run("MockQuery implements Query", func(t *testing.T) {
		var _ Query = (*MockQuery)(nil)
	})

	t.Run("MockUpdateBuilder implements UpdateBuilder", func(t *testing.T) {
		var _ UpdateBuilder = (*MockUpdateBuilder)(nil)
	})
}

// MockModelMetadata is a mock implementation of ModelMetadata interface
type MockModelMetadata struct {
	mock.Mock
}

func (m *MockModelMetadata) TableName() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockModelMetadata) PrimaryKey() KeySchema {
	args := m.Called()
	return mustKeySchema(args.Get(0))
}

func (m *MockModelMetadata) Indexes() []IndexSchema {
	args := m.Called()
	return mustIndexSchemas(args.Get(0))
}

func (m *MockModelMetadata) AttributeMetadata(field string) *AttributeMetadata {
	args := m.Called(field)
	return mustAttributeMetadata(args.Get(0))
}

func (m *MockModelMetadata) VersionFieldName() string {
	args := m.Called()
	return args.String(0)
}

func TestModelMetadataInterface(t *testing.T) {
	t.Run("MockModelMetadata implements ModelMetadata", func(t *testing.T) {
		var _ ModelMetadata = (*MockModelMetadata)(nil)
	})

	t.Run("ModelMetadata methods", func(t *testing.T) {
		mockMeta := new(MockModelMetadata)

		// Test TableName
		mockMeta.On("TableName").Return("users")
		assert.Equal(t, "users", mockMeta.TableName())

		// Test PrimaryKey
		pk := KeySchema{PartitionKey: "id", SortKey: "version"}
		mockMeta.On("PrimaryKey").Return(pk)
		assert.Equal(t, pk, mockMeta.PrimaryKey())

		// Test Indexes
		indexes := []IndexSchema{
			{Name: "email-index", Type: "GSI", PartitionKey: "email"},
		}
		mockMeta.On("Indexes").Return(indexes)
		assert.Equal(t, indexes, mockMeta.Indexes())

		// Test AttributeMetadata
		attr := &AttributeMetadata{Name: "Email", Type: "string"}
		mockMeta.On("AttributeMetadata", "email").Return(attr)
		assert.Equal(t, attr, mockMeta.AttributeMetadata("email"))

		// Test AttributeMetadata not found
		mockMeta.On("AttributeMetadata", "unknown").Return(nil)
		assert.Nil(t, mockMeta.AttributeMetadata("unknown"))

		mockMeta.AssertExpectations(t)
	})
}

// TestDBTransaction tests transaction behavior
func TestDBTransaction(t *testing.T) {
	t.Run("Successful transaction", func(t *testing.T) {
		mockDB := new(MockDB)
		var txCalled bool
		fn := func(tx *Tx) error {
			txCalled = true
			assert.NotNil(t, tx)
			assert.Equal(t, mockDB, tx.db)
			return nil
		}

		mockDB.On("Transaction", mock.MatchedBy(func(f func(tx *Tx) error) bool {
			return f != nil
		})).Return(nil).Run(func(args mock.Arguments) {
			f, ok := args.Get(0).(func(tx *Tx) error)
			if !ok {
				t.Fatalf("expected function func(tx *Tx) error, got %T", args.Get(0))
			}
			if err := f(&Tx{db: mockDB}); err != nil {
				t.Errorf("unexpected transaction error: %v", err)
			}
		})

		err := mockDB.Transaction(fn)
		assert.NoError(t, err)
		assert.True(t, txCalled)
		mockDB.AssertExpectations(t)
	})

	t.Run("Failed transaction", func(t *testing.T) {
		mockDB := new(MockDB)
		expectedErr := errors.New("transaction failed")
		fn := func(tx *Tx) error {
			return expectedErr
		}

		mockDB.On("Transaction", mock.MatchedBy(func(f func(tx *Tx) error) bool {
			return f != nil
		})).Return(expectedErr).Run(func(args mock.Arguments) {
			f, ok := args.Get(0).(func(tx *Tx) error)
			if !ok {
				t.Fatalf("expected function func(tx *Tx) error, got %T", args.Get(0))
			}
			if err := f(&Tx{db: mockDB}); err != nil {
				assert.ErrorIs(t, err, expectedErr)
			}
		})

		err := mockDB.Transaction(fn)
		assert.ErrorIs(t, err, expectedErr)
	})
}

// TestQueryChaining tests the chainable nature of Query methods
func TestQueryChaining(t *testing.T) {
	mockQuery := new(MockQuery)

	// Set up all methods to return the same query instance for chaining
	mockQuery.On("Where", "status", "=", "active").Return(mockQuery)
	mockQuery.On("Index", "status-index").Return(mockQuery)
	mockQuery.On("Filter", "age", ">", 18).Return(mockQuery)
	mockQuery.On("OrderBy", "created_at", "DESC").Return(mockQuery)
	mockQuery.On("Limit", 10).Return(mockQuery)
	mockQuery.On("Select", []string{"id", "name", "email"}).Return(mockQuery)

	// Test method chaining
	result := mockQuery.
		Where("status", "=", "active").
		Index("status-index").
		Filter("age", ">", 18).
		OrderBy("created_at", "DESC").
		Limit(10).
		Select("id", "name", "email")

	assert.Equal(t, mockQuery, result)
	mockQuery.AssertExpectations(t)
}

// TestUpdateBuilderChaining tests the chainable nature of UpdateBuilder methods
func TestUpdateBuilderChaining(t *testing.T) {
	mockBuilder := new(MockUpdateBuilder)

	// Set up all methods to return the same builder instance for chaining
	mockBuilder.On("Set", "name", "John Doe").Return(mockBuilder).Once()
	mockBuilder.On("Increment", "view_count").Return(mockBuilder).Once()
	mockBuilder.On("Add", "tags", []string{"new", "featured"}).Return(mockBuilder).Once()
	mockBuilder.On("ConditionExists", "id").Return(mockBuilder).Once()
	mockBuilder.On("ReturnValues", "ALL_NEW").Return(mockBuilder).Once()

	// Test method chaining
	result := mockBuilder.
		Set("name", "John Doe").
		Increment("view_count").
		Add("tags", []string{"new", "featured"}).
		ConditionExists("id").
		ReturnValues("ALL_NEW")

	assert.Equal(t, mockBuilder, result)
	mockBuilder.AssertExpectations(t)
}
