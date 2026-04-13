package query

import (
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/model"
)

type jsonQueryRecord struct {
	ID       string         `theorydb:"pk" json:"id"`
	Payload  map[string]any `theorydb:"json" json:"payload"`
	Response string         `theorydb:"json" json:"response"`
}

type jsonQueryMetadata struct {
	raw *model.Metadata
}

func mustJSONQueryMetadata(t *testing.T) jsonQueryMetadata {
	t.Helper()

	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&jsonQueryRecord{}))

	raw, err := registry.GetMetadata(&jsonQueryRecord{})
	require.NoError(t, err)

	return jsonQueryMetadata{raw: raw}
}

func (m jsonQueryMetadata) TableName() string {
	if m.raw == nil {
		return ""
	}
	return m.raw.TableName
}

func (m jsonQueryMetadata) PrimaryKey() core.KeySchema {
	if m.raw == nil || m.raw.PrimaryKey == nil || m.raw.PrimaryKey.PartitionKey == nil {
		return core.KeySchema{}
	}

	key := core.KeySchema{PartitionKey: m.raw.PrimaryKey.PartitionKey.Name}
	if m.raw.PrimaryKey.SortKey != nil {
		key.SortKey = m.raw.PrimaryKey.SortKey.Name
	}
	return key
}

func (m jsonQueryMetadata) Indexes() []core.IndexSchema { return nil }

func (m jsonQueryMetadata) AttributeMetadata(field string) *core.AttributeMetadata {
	if m.raw == nil {
		return nil
	}

	fieldMeta := m.raw.Fields[field]
	if fieldMeta == nil {
		fieldMeta = m.raw.FieldsByDBName[field]
	}
	if fieldMeta == nil {
		return nil
	}

	return &core.AttributeMetadata{
		Name:         fieldMeta.Name,
		Type:         fieldMeta.Type.String(),
		DynamoDBName: fieldMeta.DBName,
		Tags:         fieldMeta.Tags,
	}
}

func (m jsonQueryMetadata) VersionFieldName() string {
	if m.raw != nil && m.raw.VersionField != nil {
		return m.raw.VersionField.DBName
	}
	return ""
}

func (m jsonQueryMetadata) RawMetadata() *model.Metadata { return m.raw }

type jsonUpdateExecutor struct {
	compiled *core.CompiledQuery
	key      map[string]types.AttributeValue
	result   *core.UpdateResult
	err      error
}

func (*jsonUpdateExecutor) ExecuteQuery(*core.CompiledQuery, any) error { return fmt.Errorf("unused") }
func (*jsonUpdateExecutor) ExecuteScan(*core.CompiledQuery, any) error  { return fmt.Errorf("unused") }

func (e *jsonUpdateExecutor) ExecuteUpdateItem(input *core.CompiledQuery, key map[string]types.AttributeValue) error {
	e.compiled = input
	e.key = key
	return e.err
}

func (e *jsonUpdateExecutor) ExecuteUpdateItemWithResult(input *core.CompiledQuery, key map[string]types.AttributeValue) (*core.UpdateResult, error) {
	e.compiled = input
	e.key = key
	return e.result, e.err
}

func TestUnmarshalItem_JSONTaggedFieldsSupportLegacyAndNativeShapes(t *testing.T) {
	item := map[string]types.AttributeValue{
		"id":      &types.AttributeValueMemberS{Value: "rec-1"},
		"payload": &types.AttributeValueMemberS{Value: `{"count":3,"mode":"legacy"}`},
		"response": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
			"accepted": &types.AttributeValueMemberBOOL{Value: true},
			"count":    &types.AttributeValueMemberN{Value: "2"},
		}},
	}

	var got jsonQueryRecord
	require.NoError(t, UnmarshalItem(item, &got))

	require.Equal(t, "rec-1", got.ID)
	require.Equal(t, int64(3), got.Payload["count"])
	require.Equal(t, "legacy", got.Payload["mode"])
	require.Equal(t, `{"accepted":true,"count":2}`, got.Response)
}

func TestQueryFilter_JSONTaggedValueNormalizesJSONString(t *testing.T) {
	metadata := mustJSONQueryMetadata(t)
	q := &Query{
		metadata:    metadata,
		rawMetadata: metadata.raw,
	}

	q.Filter("Payload", "=", `{"count":4,"mode":"sync"}`)
	require.NoError(t, q.checkBuilderError())

	components := q.builder.Build()
	require.NotEmpty(t, components.ExpressionAttributeValues)

	foundStructured := false
	for _, av := range components.ExpressionAttributeValues {
		mapAV, ok := av.(*types.AttributeValueMemberM)
		if !ok {
			continue
		}

		modeAV, ok := mapAV.Value["mode"].(*types.AttributeValueMemberS)
		if !ok || modeAV.Value != "sync" {
			continue
		}

		countAV, ok := mapAV.Value["count"].(*types.AttributeValueMemberN)
		require.True(t, ok)
		require.Equal(t, "4", countAV.Value)
		foundStructured = true
	}

	require.True(t, foundStructured, "expected json filter value to normalize into a DynamoDB map")
}

func TestUpdateBuilder_JSONTaggedFieldsNormalizeValuesAndDecodeResults(t *testing.T) {
	metadata := mustJSONQueryMetadata(t)
	executor := &jsonUpdateExecutor{
		result: &core.UpdateResult{
			Attributes: map[string]types.AttributeValue{
				"id":      &types.AttributeValueMemberS{Value: "rec-1"},
				"payload": &types.AttributeValueMemberS{Value: `{"count":4,"mode":"sync"}`},
				"response": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
					"accepted": &types.AttributeValueMemberBOOL{Value: true},
				}},
			},
		},
	}

	q := &Query{
		metadata:    metadata,
		rawMetadata: metadata.raw,
		executor:    executor,
	}
	q.Where("ID", "=", "rec-1")
	q.WithCondition("Payload", "=", `{"count":4,"mode":"sync"}`)

	var got jsonQueryRecord
	err := q.UpdateBuilder().
		Set("Payload", `{"count":4,"mode":"sync"}`).
		Set("Response", map[string]any{"accepted": true}).
		ExecuteWithResult(&got)
	require.NoError(t, err)

	require.NotNil(t, executor.compiled)
	require.NotEmpty(t, executor.compiled.ExpressionAttributeValues)

	foundPayloadMap := false
	foundResponseString := false
	for _, av := range executor.compiled.ExpressionAttributeValues {
		switch v := av.(type) {
		case *types.AttributeValueMemberM:
			modeAV, ok := v.Value["mode"].(*types.AttributeValueMemberS)
			if !ok || modeAV.Value != "sync" {
				continue
			}
			countAV, ok := v.Value["count"].(*types.AttributeValueMemberN)
			require.True(t, ok)
			require.Equal(t, "4", countAV.Value)
			foundPayloadMap = true
		case *types.AttributeValueMemberS:
			if v.Value == `{"accepted":true}` {
				foundResponseString = true
			}
		}
	}

	require.True(t, foundPayloadMap, "expected json struct field to be normalized into a DynamoDB map")
	require.True(t, foundResponseString, "expected json string field to stay text-backed")

	require.Equal(t, "rec-1", got.ID)
	require.Equal(t, int64(4), got.Payload["count"])
	require.Equal(t, "sync", got.Payload["mode"])
	require.Equal(t, `{"accepted":true}`, got.Response)
}
