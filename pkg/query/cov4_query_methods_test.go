package query

import (
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/internal/expr"
	"github.com/theory-cloud/tabletheory/pkg/core"
	pkgtypes "github.com/theory-cloud/tabletheory/pkg/types"
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

func TestQuery_Update_PromotedAnonymousEmbeddedKeyAndFieldFallback(t *testing.T) {
	type BaseFields struct {
		PK     string `theorydb:"pk"`
		Status string `theorydb:"attr:status"`
	}

	type updateItem struct {
		BaseFields
	}

	metadata := &cov4Metadata{
		table: "tbl",
		pk:    core.KeySchema{PartitionKey: "pk"},
	}

	executor := &cov4Executor{}
	item := &updateItem{
		BaseFields: BaseFields{
			PK:     "id-1",
			Status: "ok",
		},
	}
	q := New(item, metadata, executor)

	require.NoError(t, q.Update("status"))
	require.NotNil(t, executor.lastUpdate)
	require.NotEmpty(t, executor.lastUpdate.UpdateExpression)

	foundStatus := false
	for _, name := range executor.lastUpdate.ExpressionAttributeNames {
		if name == "status" {
			foundStatus = true
			break
		}
	}
	require.True(t, foundStatus, "expected status placeholder to be emitted for promoted embedded field")
}

func TestQuery_buildUpdateExpressionFromTags_PromotedAnonymousEmbeds_PreservesLegacyContainer(t *testing.T) {
	type BaseObject struct {
		ID   string
		Type string
		To   []string
	}

	//nolint:govet // Field order mirrors the anonymous-embed contract fixture under test.
	type activity struct {
		BaseObject
		Actor string
	}

	item := &activity{
		BaseObject: BaseObject{
			ID:   "activity-1",
			Type: "Create",
			To:   []string{"acct:one", "acct:two"},
		},
		Actor: "acct:actor",
	}

	q := New(item, &cov4Metadata{
		table: "tbl",
		pk:    core.KeySchema{PartitionKey: "pk"},
	}, &cov4Executor{})

	modelValue, err := q.updateModelValue()
	require.NoError(t, err)

	builder := expr.NewBuilder()
	require.NoError(t, q.buildUpdateExpressionFromTags(builder, modelValue, nil))

	components := builder.Build()
	require.NotEmpty(t, components.UpdateExpression)

	foundBaseObject := false
	for _, av := range components.ExpressionAttributeValues {
		baseObjectAV, ok := av.(*types.AttributeValueMemberM)
		if !ok {
			continue
		}

		idAV, ok := baseObjectAV.Value["id"].(*types.AttributeValueMemberS)
		if !ok || idAV.Value != "activity-1" {
			continue
		}

		typeAV, ok := baseObjectAV.Value["type"].(*types.AttributeValueMemberS)
		require.True(t, ok)
		require.Equal(t, "Create", typeAV.Value)
		foundBaseObject = true
		break
	}

	require.True(t, foundBaseObject, "expected legacy BaseObject container update to remain present")
}

func TestQuery_buildUpdateExpressionFromTags_PromotedAnonymousEmbeds_FlatOptIn(t *testing.T) {
	type BaseObject struct {
		ID   string
		Type string
		To   []string
	}

	//nolint:govet // Field order mirrors the anonymous-embed contract fixture under test.
	type activity struct {
		BaseObject
		Actor string
	}

	item := &activity{
		BaseObject: BaseObject{
			ID:   "activity-1",
			Type: "Create",
			To:   []string{"acct:one", "acct:two"},
		},
		Actor: "acct:actor",
	}

	q := New(item, &cov4Metadata{
		table: "tbl",
		pk:    core.KeySchema{PartitionKey: "pk"},
	}, &cov4Executor{}).WithConverter(pkgtypes.NewConverter().WithFlatAnonymousEmbedEncoding())

	modelValue, err := q.updateModelValue()
	require.NoError(t, err)

	builder := expr.NewBuilderWithConverter(q.converter)
	require.NoError(t, q.buildUpdateExpressionFromTags(builder, modelValue, nil))

	components := builder.Build()
	require.NotEmpty(t, components.UpdateExpression)

	foundFlat := false
	for _, av := range components.ExpressionAttributeValues {
		flatAV, ok := av.(*types.AttributeValueMemberM)
		if ok {
			require.NotContains(t, flatAV.Value, "baseObject")
		}
		idAV, ok := av.(*types.AttributeValueMemberS)
		if ok && idAV.Value == "activity-1" {
			foundFlat = true
		}
	}

	require.True(t, foundFlat, "expected promoted fields to update without a BaseObject container when flat mode is enabled")
}

func TestQuery_marshalItemTaggedFlat_SkipsIgnoredAndOmitEmptyFields(t *testing.T) {
	type BaseFields struct {
		ID      string `theorydb:"pk"`
		Ignored string `theorydb:"-"`
	}

	type item struct {
		BaseFields
		Optional string `theorydb:"attr:optional,omitempty"`
		Status   string `theorydb:"attr:status"`
	}

	q := New(&item{}, &cov4Metadata{
		table: "tbl",
		pk:    core.KeySchema{PartitionKey: "ID"},
	}, &cov4Executor{}).WithConverter(pkgtypes.NewConverter().WithFlatAnonymousEmbedEncoding())

	out, err := q.marshalItemTaggedFlat(reflect.ValueOf(item{
		BaseFields: BaseFields{
			ID:      "id-1",
			Ignored: "drop-me",
		},
		Status: "ready",
	}))
	require.NoError(t, err)
	require.Contains(t, out, "ID")
	require.Contains(t, out, "Status")
	require.NotContains(t, out, "Optional")
	require.NotContains(t, out, "Ignored")
}

func TestQuery_buildUpdateExpressionFromTaggedVisibleFields_SkipsKeysAndOmitEmpty(t *testing.T) {
	type BaseFields struct {
		ID string `theorydb:"pk"`
	}

	type item struct {
		BaseFields
		Optional string `theorydb:"attr:optional,omitempty"`
		Status   string `theorydb:"attr:status"`
	}

	q := New(&item{}, &cov4Metadata{
		table: "tbl",
		pk:    core.KeySchema{PartitionKey: "ID"},
		attrs: map[string]string{
			"Status": "status",
		},
	}, &cov4Executor{}).WithConverter(pkgtypes.NewConverter().WithFlatAnonymousEmbedEncoding())

	builder := expr.NewBuilderWithConverter(q.converter)
	err := q.buildUpdateExpressionFromTaggedVisibleFields(builder, reflect.ValueOf(item{
		BaseFields: BaseFields{ID: "id-1"},
		Status:     "ready",
	}), q.metadata.PrimaryKey())
	require.NoError(t, err)

	components := builder.Build()
	require.NotEmpty(t, components.UpdateExpression)
	require.Len(t, components.ExpressionAttributeValues, 1)

	for _, av := range components.ExpressionAttributeValues {
		statusAV, ok := av.(*types.AttributeValueMemberS)
		require.True(t, ok)
		require.Equal(t, "ready", statusAV.Value)
	}
}

func TestQuery_FlatTaggedHelpers_NormalizeJSONValues(t *testing.T) {
	type BaseFields struct {
		ID string `theorydb:"pk"`
	}

	type item struct {
		Payload map[string]any `theorydb:"json"`
		BaseFields
	}

	q := New(&item{}, &cov4Metadata{
		table: "tbl",
		pk:    core.KeySchema{PartitionKey: "ID"},
		attrs: map[string]string{
			"Payload": "payload",
		},
	}, &cov4Executor{}).WithConverter(pkgtypes.NewConverter().WithFlatAnonymousEmbedEncoding())

	modelValue := reflect.ValueOf(item{
		BaseFields: BaseFields{ID: "id-1"},
		Payload: map[string]any{
			"count": 2,
			"mode":  "sync",
		},
	})

	out, err := q.marshalItemTaggedFlat(modelValue)
	require.NoError(t, err)

	payloadAV, ok := out["Payload"].(*types.AttributeValueMemberM)
	require.True(t, ok)
	countAV, ok := payloadAV.Value["count"].(*types.AttributeValueMemberN)
	require.True(t, ok)
	require.Equal(t, "2", countAV.Value)
	modeAV, ok := payloadAV.Value["mode"].(*types.AttributeValueMemberS)
	require.True(t, ok)
	require.Equal(t, "sync", modeAV.Value)

	builder := expr.NewBuilderWithConverter(q.converter)
	require.NoError(t, q.buildUpdateExpressionFromTaggedVisibleFields(builder, modelValue, q.metadata.PrimaryKey()))

	components := builder.Build()
	require.Len(t, components.ExpressionAttributeValues, 1)
	for _, av := range components.ExpressionAttributeValues {
		normalizedPayload, ok := av.(*types.AttributeValueMemberM)
		require.True(t, ok)
		require.Contains(t, normalizedPayload.Value, "count")
		require.Contains(t, normalizedPayload.Value, "mode")
	}
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
