package query

import (
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/core"
)

type cov4Metadata struct {
	attrs   map[string]string
	pk      core.KeySchema
	table   string
	version string
	indexes []core.IndexSchema
}

func (m *cov4Metadata) TableName() string           { return m.table }
func (m *cov4Metadata) PrimaryKey() core.KeySchema  { return m.pk }
func (m *cov4Metadata) Indexes() []core.IndexSchema { return m.indexes }
func (m *cov4Metadata) VersionFieldName() string    { return m.version }
func (m *cov4Metadata) AttributeMetadata(field string) *core.AttributeMetadata {
	if m == nil {
		return nil
	}
	if m.attrs != nil {
		if dbName, ok := m.attrs[field]; ok {
			return &core.AttributeMetadata{Name: field, DynamoDBName: dbName, Type: "S"}
		}
	}
	return &core.AttributeMetadata{Name: field, DynamoDBName: field, Type: "S"}
}

type cov4Executor struct {
	lastUpdate  *core.CompiledQuery
	lastPutItem map[string]types.AttributeValue
	scans       []*core.CompiledQuery
	queries     []*core.CompiledQuery
	mu          sync.Mutex
}

func (e *cov4Executor) ExecuteQuery(input *core.CompiledQuery, dest any) error {
	e.mu.Lock()
	e.queries = append(e.queries, input)
	e.mu.Unlock()

	return nil
}

func (e *cov4Executor) ExecuteScan(input *core.CompiledQuery, dest any) error {
	e.mu.Lock()
	e.scans = append(e.scans, input)
	e.mu.Unlock()

	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr || destValue.Elem().Kind() != reflect.Slice {
		return fmt.Errorf("dest must be pointer to slice")
	}

	segment := int32(-1)
	if input != nil && input.Segment != nil {
		segment = *input.Segment
	}

	slice := destValue.Elem()
	elemType := slice.Type().Elem()
	elem := reflect.New(elemType).Elem()
	if elem.Kind() == reflect.Struct {
		if id := elem.FieldByName("ID"); id.IsValid() && id.CanSet() && id.Kind() == reflect.String {
			id.SetString(fmt.Sprintf("seg-%d", segment))
		}
	}
	slice.Set(reflect.Append(slice, elem))
	return nil
}

func (e *cov4Executor) ExecutePutItem(input *core.CompiledQuery, item map[string]types.AttributeValue) error {
	e.mu.Lock()
	e.lastPutItem = item
	e.mu.Unlock()
	return nil
}

func (e *cov4Executor) ExecuteUpdateItem(input *core.CompiledQuery, key map[string]types.AttributeValue) error {
	_ = key
	e.mu.Lock()
	e.lastUpdate = input
	e.mu.Unlock()
	return nil
}

func TestQuery_Scan_ParallelScan_ScanAllSegments(t *testing.T) {
	type scanItem struct {
		ID     string
		Status string
	}

	metadata := &cov4Metadata{
		table: "tbl",
		pk:    core.KeySchema{PartitionKey: "ID"},
		attrs: map[string]string{
			"ID":     "id",
			"Status": "status",
		},
	}

	executor := &cov4Executor{}
	q := New(&scanItem{}, metadata, executor)

	var out []scanItem
	require.NoError(t, q.Index("by-status").Offset(2).ConsistentRead().WithRetry(1, 0).Where("Status", "=", "active").Scan(&out))
	require.NotEmpty(t, executor.scans)

	out = nil
	require.NoError(t, q.ParallelScan(0, 2).Scan(&out))

	var all []scanItem
	require.NoError(t, q.ScanAllSegments(&all, 2))
	require.Len(t, all, 2)
}

func TestQuery_Update_AllFieldsPath(t *testing.T) {
	type updateItem struct {
		CreatedAt time.Time `theorydb:"created_at"`
		ID        string    `theorydb:"pk"`
		Optional  string    `theorydb:"attr:optional,omitempty"`
		Status    string    `theorydb:"attr:status"`
	}

	metadata := &cov4Metadata{
		table: "tbl",
		pk:    core.KeySchema{PartitionKey: "ID"},
		attrs: map[string]string{
			"ID":     "id",
			"Status": "status",
		},
	}

	executor := &cov4Executor{}
	item := &updateItem{ID: "id-1", Status: "ok"}
	q := New(item, metadata, executor).Where("ID", "=", item.ID)

	require.NoError(t, q.Update())
	require.NotNil(t, executor.lastUpdate)
	require.Contains(t, executor.lastUpdate.UpdateExpression, "SET")
}

func TestQuery_CreateOrUpdate_OmitsZeroValues(t *testing.T) {
	type upsertItem struct {
		When     time.Time `theorydb:"attr:when,omitempty"`
		ID       string    `theorydb:"pk"`
		Optional string    `theorydb:"attr:optional,omitempty"`
	}

	metadata := &cov4Metadata{
		table: "tbl",
		pk:    core.KeySchema{PartitionKey: "ID"},
	}

	executor := &cov4Executor{}
	item := &upsertItem{ID: "id-1"}
	q := New(item, metadata, executor)

	require.NoError(t, q.CreateOrUpdate())
	require.NotNil(t, executor.lastPutItem)
	require.Contains(t, executor.lastPutItem, "ID")
	require.NotContains(t, executor.lastPutItem, "Optional")
	require.NotContains(t, executor.lastPutItem, "When")
}

func TestQuery_buildConditionExpression_DefaultIfEmpty(t *testing.T) {
	metadata := &cov4Metadata{table: "tbl", pk: core.KeySchema{PartitionKey: "ID"}}
	executor := &cov4Executor{}
	q := New(&struct{ ID string }{}, metadata, executor)

	exprStr, _, _, err := q.buildConditionExpression(nil, false, false, true)
	require.NoError(t, err)
	require.Contains(t, exprStr, "attribute_not_exists")
}

func TestQuery_SetCursor_And_RecordBuilderError(t *testing.T) {
	metadata := &cov4Metadata{table: "tbl", pk: core.KeySchema{PartitionKey: "ID"}}
	executor := &cov4Executor{}
	q := New(&struct{ ID string }{}, metadata, executor)

	cursor, err := EncodeCursor(map[string]types.AttributeValue{
		"id": &types.AttributeValueMemberS{Value: "1"},
	}, "idx", "ASC")
	require.NoError(t, err)

	require.NoError(t, q.SetCursor(cursor))
	require.NotNil(t, q.exclusive)

	q.Cursor("not-a-cursor")
	require.Error(t, q.checkBuilderError())
}
