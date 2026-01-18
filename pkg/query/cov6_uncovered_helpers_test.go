package query

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/model"
)

type anyValueConverter struct{}

func (anyValueConverter) HasCustomConverter(reflect.Type) bool { return false }

func (anyValueConverter) ToAttributeValue(value any) (types.AttributeValue, error) {
	return nil, errors.New("not implemented")
}

func (anyValueConverter) FromAttributeValue(av types.AttributeValue, target any) error {
	out, ok := target.(*any)
	if !ok {
		return errors.New("target must be *any")
	}

	switch v := av.(type) {
	case *types.AttributeValueMemberS:
		*out = v.Value
		return nil
	case *types.AttributeValueMemberN:
		i, err := strconv.Atoi(v.Value)
		if err != nil {
			return err
		}
		*out = i
		return nil
	case *types.AttributeValueMemberNULL:
		*out = nil
		return nil
	default:
		return errors.New("unsupported attribute value type")
	}
}

func (anyValueConverter) ConvertToSet(slice any, isSet bool) (types.AttributeValue, error) {
	_ = slice
	_ = isSet
	return nil, errors.New("not implemented")
}

type stubModelMetadata struct {
	pk core.KeySchema
}

func (s stubModelMetadata) TableName() string { return "TestTable" }
func (s stubModelMetadata) PrimaryKey() core.KeySchema {
	return s.pk
}
func (s stubModelMetadata) Indexes() []core.IndexSchema { return nil }
func (s stubModelMetadata) AttributeMetadata(string) *core.AttributeMetadata {
	return nil
}
func (s stubModelMetadata) VersionFieldName() string { return "" }

type stubModelMetadataWithRaw struct {
	raw *model.Metadata
	stubModelMetadata
}

func (s stubModelMetadataWithRaw) RawMetadata() *model.Metadata { return s.raw }

func TestQuery_unmarshalItemWithMetadata_MapDestination_COV6(t *testing.T) {
	q := &Query{
		rawMetadata: &model.Metadata{},
		converter:   anyValueConverter{},
	}

	item := map[string]types.AttributeValue{
		"Name":  &types.AttributeValueMemberS{Value: "Alice"},
		"Score": &types.AttributeValueMemberN{Value: "5"},
		"Nil":   &types.AttributeValueMemberNULL{Value: true},
	}

	t.Run("MapStringAny", func(t *testing.T) {
		var dest map[string]any
		err := q.unmarshalItemWithMetadata(item, &dest)
		require.NoError(t, err)
		require.Equal(t, "Alice", dest["Name"])
		require.Equal(t, 5, dest["Score"])
		require.Nil(t, dest["Nil"])
	})

	t.Run("MapStringFloat64_ConversionAndSkip", func(t *testing.T) {
		var dest map[string]float64
		err := q.unmarshalItemWithMetadata(item, &dest)
		require.NoError(t, err)
		require.Equal(t, 5.0, dest["Score"])
		require.Equal(t, 0.0, dest["Nil"])
		_, ok := dest["Name"]
		require.False(t, ok)
	})

	t.Run("MapKeyTypeMustBeString", func(t *testing.T) {
		var dest map[int]any
		err := q.unmarshalItemWithMetadata(item, &dest)
		require.Error(t, err)
		require.Contains(t, err.Error(), "destination map must have string keys")
	})

	t.Run("MapValueForType_NilValue", func(t *testing.T) {
		val, ok := mapValueForType(nil, reflect.TypeOf(int64(0)))
		require.True(t, ok)
		require.Equal(t, int64(0), val.Int())
	})
}

func TestQuery_extractKey_KeyPair_COV6(t *testing.T) {
	q := &Query{
		metadata: stubModelMetadata{
			pk: core.KeySchema{PartitionKey: "PK", SortKey: "SK"},
		},
	}

	key, err := q.extractKey(core.NewKeyPair("p", "s"))
	require.NoError(t, err)
	require.Equal(t, "p", key["PK"])
	require.Equal(t, "s", key["SK"])

	_, err = q.extractKey(core.NewKeyPair("p"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "sort key value is required")

	_, err = q.extractKey(core.KeyPair{PartitionKey: nil, SortKey: "s"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "partition key value is required")
}

func TestNewWithConditions_DefaultsContextAndRawMetadata_COV6(t *testing.T) {
	raw := &model.Metadata{TableName: "RawTable"}

	q := NewWithConditions(
		&struct{}{},
		stubModelMetadataWithRaw{
			stubModelMetadata: stubModelMetadata{
				pk: core.KeySchema{PartitionKey: "PK"},
			},
			raw: raw,
		},
		nil,
		[]Condition{{Field: "PK", Operator: "=", Value: "p"}},
		nil,
	)

	require.NotNil(t, q)
	require.Equal(t, context.Background(), q.ctx)
	require.Same(t, raw, q.rawMetadata)
	require.Len(t, q.filters, 0)
	require.Len(t, q.writeConditions, 0)
	require.Len(t, q.rawConditionExpressions, 0)
}
