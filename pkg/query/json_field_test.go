package query

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/model"
	pkgtypes "github.com/theory-cloud/tabletheory/pkg/types"
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

type helperMetadata struct {
	attrs   map[string]*core.AttributeMetadata
	indexes []core.IndexSchema
}

func (*jsonUpdateExecutor) ExecuteQuery(*core.CompiledQuery, any) error { return fmt.Errorf("unused") }
func (*jsonUpdateExecutor) ExecuteScan(*core.CompiledQuery, any) error  { return fmt.Errorf("unused") }

func (m helperMetadata) TableName() string            { return "" }
func (m helperMetadata) PrimaryKey() core.KeySchema   { return core.KeySchema{} }
func (m helperMetadata) Indexes() []core.IndexSchema  { return m.indexes }
func (m helperMetadata) VersionFieldName() string     { return "" }
func (m helperMetadata) RawMetadata() *model.Metadata { return nil }
func (m helperMetadata) AttributeMetadata(field string) *core.AttributeMetadata {
	return m.attrs[field]
}

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

type taggedMarshalRecord struct {
	Hidden  string         `theorydb:"-"`
	Payload map[string]any `theorydb:"json,omitempty"`
	Title   taggedString   `theorydb:"attr:title"`
	ID      string         `theorydb:"pk"`
}

type taggedString string

func TestMarshalItemTagged_CoversJSONConverterAndErrorPaths(t *testing.T) {
	t.Run("marshalItemTagged normalizes json fields and skips ignored ones", func(t *testing.T) {
		q := &Query{}
		item := taggedMarshalRecord{
			Hidden: "skip-me",
			Payload: map[string]any{
				"count": 2,
			},
			Title: "sync",
			ID:    "rec-1",
		}

		out, err := q.marshalItemTagged(item)
		require.NoError(t, err)
		require.NotContains(t, out, "Hidden")

		payloadAV, ok := out["Payload"].(*types.AttributeValueMemberM)
		require.True(t, ok)
		countAV, ok := payloadAV.Value["count"].(*types.AttributeValueMemberN)
		require.True(t, ok)
		require.Equal(t, "2", countAV.Value)

		titleAV, ok := out["Title"].(*types.AttributeValueMemberS)
		require.True(t, ok)
		require.Equal(t, "sync", titleAV.Value)
	})

	t.Run("marshalTaggedFieldAttributeValue uses the configured converter", func(t *testing.T) {
		q := &Query{converter: pkgtypes.NewConverter()}
		recordType := reflect.TypeOf(taggedMarshalRecord{})
		field, ok := recordType.FieldByName("Title")
		require.True(t, ok)

		av, err := q.marshalTaggedFieldAttributeValue(field, reflect.ValueOf(taggedString("converted")))
		require.NoError(t, err)
		titleAV, ok := av.(*types.AttributeValueMemberS)
		require.True(t, ok)
		require.Equal(t, "converted", titleAV.Value)
	})

	t.Run("marshalTaggedFieldAttributeValue returns normalization errors", func(t *testing.T) {
		recordType := reflect.TypeOf(taggedMarshalRecord{})
		field, ok := recordType.FieldByName("Payload")
		require.True(t, ok)

		_, err := (&Query{}).marshalTaggedFieldAttributeValue(field, reflect.ValueOf(map[string]any{
			"bad": make(chan int),
		}))
		require.Error(t, err)
	})

	t.Run("marshalItemTagged rejects invalid inputs", func(t *testing.T) {
		_, err := (&Query{}).marshalItemTagged(nil)
		require.Error(t, err)

		_, err = (&Query{}).marshalItemTagged("not-a-struct")
		require.Error(t, err)
	})

	t.Run("marshalItemTagged preserves legacy anonymous embedded container shape", func(t *testing.T) {
		type BaseObject struct {
			ID   string
			Type string
			To   []string
		}

		//nolint:govet // Field order mirrors the anonymous-embed contract fixture under test.
		type Activity struct {
			BaseObject
			Actor string
		}

		out, err := (&Query{}).marshalItemTagged(Activity{
			BaseObject: BaseObject{
				ID:   "activity-1",
				Type: "Create",
				To:   []string{"acct:one", "acct:two"},
			},
			Actor: "acct:actor",
		})
		require.NoError(t, err)

		require.Contains(t, out, "BaseObject")
		require.Contains(t, out, "Actor")
		require.NotContains(t, out, "ID")
		require.NotContains(t, out, "Type")
		require.NotContains(t, out, "To")

		baseObjectAV, ok := out["BaseObject"].(*types.AttributeValueMemberM)
		require.True(t, ok)

		idAV, ok := baseObjectAV.Value["id"].(*types.AttributeValueMemberS)
		require.True(t, ok)
		require.Equal(t, "activity-1", idAV.Value)

		typeAV, ok := baseObjectAV.Value["type"].(*types.AttributeValueMemberS)
		require.True(t, ok)
		require.Equal(t, "Create", typeAV.Value)
	})
}

func TestQueryMetadataHelpers_CoverLookupBranches(t *testing.T) {
	rawMeta := &model.Metadata{
		Fields:         make(map[string]*model.FieldMetadata),
		FieldsByDBName: make(map[string]*model.FieldMetadata),
	}
	goFieldMeta := &model.FieldMetadata{Name: "GoName", DBName: "go_name"}
	dbFieldMeta := &model.FieldMetadata{Name: "DBName", DBName: "db_name"}
	rawMeta.Fields["GoName"] = goFieldMeta
	rawMeta.FieldsByDBName["go_name"] = goFieldMeta
	rawMeta.Fields["DBName"] = dbFieldMeta
	rawMeta.FieldsByDBName["db_name"] = dbFieldMeta

	q := &Query{
		rawMetadata: rawMeta,
		metadata: helperMetadata{
			attrs: map[string]*core.AttributeMetadata{
				"alias_by_name":   {Name: "GoName"},
				"alias_by_dbname": {DynamoDBName: "db_name"},
			},
			indexes: []core.IndexSchema{
				{Name: "gsi1", Type: "GSI"},
			},
		},
	}

	require.Same(t, goFieldMeta, q.rawFieldMetadata("GoName"))
	require.Same(t, goFieldMeta, q.rawFieldMetadata("go_name"))
	require.Same(t, goFieldMeta, q.rawFieldMetadata("alias_by_name"))
	require.Same(t, dbFieldMeta, q.rawFieldMetadata("alias_by_dbname"))
	require.Nil(t, q.rawFieldMetadata("missing"))

	idx := q.indexSchemaByName("gsi1")
	require.NotNil(t, idx)
	require.Equal(t, "gsi1", idx.Name)
	require.Nil(t, q.indexSchemaByName("missing"))
}
