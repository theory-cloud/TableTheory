package query

import (
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/core"
	theorydbErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/model"
)

type writePolicyQueryMetadata struct {
	meta *model.Metadata
}

func (m writePolicyQueryMetadata) TableName() string { return m.meta.TableName }

func (m writePolicyQueryMetadata) PrimaryKey() core.KeySchema {
	schema := core.KeySchema{}
	if m.meta.PrimaryKey != nil && m.meta.PrimaryKey.PartitionKey != nil {
		schema.PartitionKey = m.meta.PrimaryKey.PartitionKey.Name
	}
	if m.meta.PrimaryKey != nil && m.meta.PrimaryKey.SortKey != nil {
		schema.SortKey = m.meta.PrimaryKey.SortKey.Name
	}
	return schema
}

func (m writePolicyQueryMetadata) Indexes() []core.IndexSchema { return nil }

func (m writePolicyQueryMetadata) AttributeMetadata(field string) *core.AttributeMetadata {
	if meta := m.meta.Fields[field]; meta != nil {
		return writePolicyAttributeMetadata(meta)
	}
	if meta := m.meta.FieldsByDBName[field]; meta != nil {
		return writePolicyAttributeMetadata(meta)
	}
	return nil
}

func (m writePolicyQueryMetadata) VersionFieldName() string { return "" }

func (m writePolicyQueryMetadata) RawMetadata() *model.Metadata { return m.meta }

func (m writePolicyQueryMetadata) WritePolicy() model.WritePolicy {
	return m.meta.WritePolicy
}

func writePolicyAttributeMetadata(field *model.FieldMetadata) *core.AttributeMetadata {
	typeName := ""
	if field.Type != nil {
		typeName = field.Type.String()
	}
	return &core.AttributeMetadata{
		Name:         field.Name,
		Type:         typeName,
		DynamoDBName: field.DBName,
		Tags:         field.Tags,
	}
}

type writePolicyRecordingExecutor struct {
	compiled           *core.CompiledQuery
	batchTableName     string
	batchWriteRequests []types.WriteRequest
}

func (e *writePolicyRecordingExecutor) ExecuteQuery(*core.CompiledQuery, any) error { return nil }
func (e *writePolicyRecordingExecutor) ExecuteScan(*core.CompiledQuery, any) error  { return nil }

func (e *writePolicyRecordingExecutor) ExecutePutItem(input *core.CompiledQuery, _ map[string]types.AttributeValue) error {
	e.compiled = input
	return nil
}

func (e *writePolicyRecordingExecutor) ExecuteUpdateItem(input *core.CompiledQuery, _ map[string]types.AttributeValue) error {
	e.compiled = input
	return nil
}

func (e *writePolicyRecordingExecutor) ExecuteDeleteItem(input *core.CompiledQuery, _ map[string]types.AttributeValue) error {
	e.compiled = input
	return nil
}

func (e *writePolicyRecordingExecutor) ExecuteBatchWriteItem(tableName string, writeRequests []types.WriteRequest) (*core.BatchWriteResult, error) {
	e.batchTableName = tableName
	e.batchWriteRequests = writeRequests
	return &core.BatchWriteResult{}, nil
}

type writePolicyActualItem struct {
	PK              string `theorydb:"pk"`
	SK              string `theorydb:"sk"`
	Status          string
	PinnedReleaseID string `theorydb:"attr:pinnedReleaseId,omitempty"`
	Version         int64  `theorydb:"version"`
}

func (writePolicyActualItem) TableName() string { return "write_policy_items" }

func (writePolicyActualItem) WritePolicy() model.WritePolicy {
	return model.WritePolicy{
		Mode:                model.WritePolicyModeMutable,
		ProtectedAttributes: []string{"pinnedReleaseId"},
	}
}

type writePolicyEventItem struct {
	PK        string `theorydb:"pk"`
	SK        string `theorydb:"sk"`
	EventType string `theorydb:"attr:eventType"`
}

func (writePolicyEventItem) TableName() string { return "write_policy_items" }

func (writePolicyEventItem) WritePolicy() model.WritePolicy {
	return model.WritePolicy{Mode: model.WritePolicyModeWriteOnce}
}

func newWritePolicyQuery(t *testing.T, item any) (*Query, *writePolicyRecordingExecutor) {
	t.Helper()

	registry := model.NewRegistry()
	require.NoError(t, registry.Register(item))
	meta, err := registry.GetMetadata(item)
	require.NoError(t, err)

	executor := &writePolicyRecordingExecutor{}
	q := New(item, writePolicyQueryMetadata{meta: meta}, executor)
	return q, executor
}

func TestWritePolicy_CreateOnWriteOnceAddsNotExistsCondition(t *testing.T) {
	item := &writePolicyEventItem{PK: "release#svc", SK: "event#1", EventType: "promoted"}
	q, executor := newWritePolicyQuery(t, item)

	require.NoError(t, q.Create())
	require.NotNil(t, executor.compiled)
	require.Contains(t, executor.compiled.ConditionExpression, "attribute_not_exists")
}

func TestWritePolicy_CreateOnProtectedModelAddsNotExistsCondition(t *testing.T) {
	item := &writePolicyActualItem{PK: "release#svc", SK: "actual", Status: "warming", PinnedReleaseID: "rel-1"}
	q, executor := newWritePolicyQuery(t, item)

	require.NoError(t, q.Create())
	require.NotNil(t, executor.compiled)
	require.Contains(t, executor.compiled.ConditionExpression, "attribute_not_exists")
}

func TestWritePolicy_WriteOnceRejectsGenericMutations(t *testing.T) {
	tests := []struct {
		run  func(*Query) error
		name string
	}{
		{run: func(q *Query) error { return q.Update("eventType") }, name: "update"},
		{run: func(q *Query) error { return q.CreateOrUpdate() }, name: "upsert"},
		{run: func(q *Query) error { return q.Delete() }, name: "delete"},
		{run: func(q *Query) error {
			return q.UpdateBuilder().Set("eventType", "mutated").Execute()
		}, name: "update_builder"},
		{run: func(q *Query) error {
			return q.BatchCreate([]writePolicyEventItem{{PK: "release#svc", SK: "event#2"}})
		}, name: "batch_create"},
		{run: func(q *Query) error {
			return q.BatchDelete([]any{core.KeyPair{PartitionKey: "release#svc", SortKey: "event#1"}})
		}, name: "batch_delete"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := &writePolicyEventItem{PK: "release#svc", SK: "event#1", EventType: "promoted"}
			q, _ := newWritePolicyQuery(t, item)
			err := tt.run(q)
			require.Error(t, err)
			require.True(t, errors.Is(err, theorydbErrors.ErrImmutableModelMutation))
		})
	}
}

func TestWritePolicy_ProtectedAttributesRejectPutStyleOverwrites(t *testing.T) {
	item := &writePolicyActualItem{
		PK:              "release#svc",
		SK:              "actual",
		Status:          "warming",
		PinnedReleaseID: "rel-2",
		Version:         1,
	}

	tests := []struct {
		run  func(*Query) error
		name string
	}{
		{run: func(q *Query) error { return q.CreateOrUpdate() }, name: "upsert"},
		{run: func(q *Query) error {
			return q.BatchCreate([]writePolicyActualItem{*item})
		}, name: "batch_create"},
		{run: func(q *Query) error {
			return q.BatchWrite([]any{*item}, nil)
		}, name: "batch_put"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, _ := newWritePolicyQuery(t, item)
			err := tt.run(q)
			require.Error(t, err)
			require.True(t, errors.Is(err, theorydbErrors.ErrProtectedFieldMutation))
		})
	}
}

func TestWritePolicy_ProtectedAttributesRejectRoutineUpdates(t *testing.T) {
	item := &writePolicyActualItem{
		PK:              "release#svc",
		SK:              "actual",
		Status:          "warming",
		PinnedReleaseID: "rel-2",
		Version:         1,
	}

	tests := []struct {
		run  func(*Query) error
		name string
	}{
		{run: func(q *Query) error { return q.Update("PinnedReleaseID") }, name: "update_named_go_field"},
		{run: func(q *Query) error { return q.Update("pinnedReleaseId") }, name: "update_named_db_attr"},
		{run: func(q *Query) error { return q.Update() }, name: "update_all_non_empty_fields"},
		{run: func(q *Query) error {
			return q.UpdateBuilder().Set("pinnedReleaseId", "rel-3").Execute()
		}, name: "update_builder_set"},
		{run: func(q *Query) error {
			return q.UpdateBuilder().Set("pinnedReleaseId.value", "rel-3").Execute()
		}, name: "update_builder_nested"},
		{run: func(q *Query) error {
			return q.BatchUpdate([]writePolicyActualItem{*item}, "pinnedReleaseId")
		}, name: "batch_update"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, _ := newWritePolicyQuery(t, item)
			err := tt.run(q)
			require.Error(t, err)
			require.True(t, errors.Is(err, theorydbErrors.ErrProtectedFieldMutation))
		})
	}
}

func TestWritePolicy_ProtectedAttributesAllowOtherUpdatesAndDelete(t *testing.T) {
	item := &writePolicyActualItem{
		PK:      "release#svc",
		SK:      "actual",
		Status:  "active",
		Version: 1,
	}
	q, executor := newWritePolicyQuery(t, item)

	require.NoError(t, q.Update("Status"))
	require.NotNil(t, executor.compiled)
	require.Equal(t, "UpdateItem", executor.compiled.Operation)

	executor.compiled = nil
	require.NoError(t, q.Delete())
	require.NotNil(t, executor.compiled)
	require.Equal(t, "DeleteItem", executor.compiled.Operation)
}

func TestWritePolicy_FieldRootHelpers(t *testing.T) {
	require.Equal(t, "pinnedReleaseId", rootAttributeName("pinnedReleaseId.value"))
	require.Equal(t, "pinnedReleaseId", rootAttributeName("pinnedReleaseId[0]"))
	require.Equal(t, "status.tail", replaceRootAttribute("Status.tail", "status"))
	require.Equal(t, "status[0]", replaceRootAttribute("Status[0]", "status"))
}
