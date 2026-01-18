package theorydb

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/model"
	pkgTypes "github.com/theory-cloud/tabletheory/pkg/types"
)

func TestQueryExecutor_WriteItemsToDest_COV7(t *testing.T) {
	qe := &queryExecutor{
		db: &DB{
			converter: pkgTypes.NewConverter(),
		},
	}

	items := []map[string]types.AttributeValue{
		{
			"a":    &types.AttributeValueMemberS{Value: "x"},
			"null": &types.AttributeValueMemberNULL{Value: true},
		},
	}

	t.Run("raw_attribute_value_maps", func(t *testing.T) {
		var dest []map[string]types.AttributeValue
		require.NoError(t, qe.writeItemsToDest(items, &dest))
		require.Len(t, dest, 1)
		require.Equal(t, items[0], dest[0])
	})

	t.Run("unmarshal_to_slice_of_maps", func(t *testing.T) {
		var dest []map[string]any
		require.NoError(t, qe.writeItemsToDest(items, &dest))
		require.Len(t, dest, 1)
		require.Equal(t, "x", dest[0]["a"])
		_, ok := dest[0]["null"]
		require.True(t, ok)
	})
}

func TestQueryExecutor_UnmarshalItemToStruct_COV7(t *testing.T) {
	type testModel struct {
		ID  string
		Age int
	}

	metadata := &model.Metadata{
		FieldsByDBName: map[string]*model.FieldMetadata{
			"id":  {Name: "ID", DBName: "id", IndexPath: []int{0}},
			"age": {Name: "Age", DBName: "age", IndexPath: []int{1}},
		},
	}

	qe := &queryExecutor{
		db: &DB{
			converter: pkgTypes.NewConverter(),
		},
		metadata: metadata,
	}

	t.Run("success", func(t *testing.T) {
		item := map[string]types.AttributeValue{
			"id":  &types.AttributeValueMemberS{Value: "u1"},
			"age": &types.AttributeValueMemberN{Value: "42"},
		}

		var dest testModel
		require.NoError(t, qe.unmarshalItem(item, &dest))
		require.Equal(t, "u1", dest.ID)
		require.Equal(t, 42, dest.Age)
	})

	t.Run("invalid_number_errors", func(t *testing.T) {
		item := map[string]types.AttributeValue{
			"age": &types.AttributeValueMemberN{Value: "not-a-number"},
		}

		var dest testModel
		require.Error(t, qe.unmarshalItem(item, &dest))
	})

	t.Run("metadata_required", func(t *testing.T) {
		item := map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: "u1"},
		}

		qeNoMeta := &queryExecutor{
			db: &DB{
				converter: pkgTypes.NewConverter(),
			},
		}

		var dest testModel
		require.Error(t, qeNoMeta.unmarshalItem(item, &dest))
	})
}

func TestQueryExecutor_unmarshalItem_MapAny_DecodesCommonTypes_COV7(t *testing.T) {
	executor := &queryExecutor{
		db: &DB{
			converter: pkgTypes.NewConverter(),
		},
	}

	t.Run("success", func(t *testing.T) {
		item := map[string]types.AttributeValue{
			"s":       &types.AttributeValueMemberS{Value: "hello"},
			"n_int":   &types.AttributeValueMemberN{Value: "3"},
			"n_float": &types.AttributeValueMemberN{Value: "3.14"},
			"bool":    &types.AttributeValueMemberBOOL{Value: true},
			"null":    &types.AttributeValueMemberNULL{Value: true},
			"b":       &types.AttributeValueMemberB{Value: []byte{0x01}},
			"ss":      &types.AttributeValueMemberSS{Value: []string{"a", "b"}},
			"ns":      &types.AttributeValueMemberNS{Value: []string{"1.5", "2"}},
			"bs":      &types.AttributeValueMemberBS{Value: [][]byte{[]byte("a")}},
			"l": &types.AttributeValueMemberL{Value: []types.AttributeValue{
				&types.AttributeValueMemberS{Value: "x"},
				&types.AttributeValueMemberN{Value: "2"},
				&types.AttributeValueMemberNULL{Value: true},
			}},
			"m": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
				"k": &types.AttributeValueMemberS{Value: "v"},
				"n": &types.AttributeValueMemberN{Value: "1"},
			}},
		}

		var dest map[string]any
		require.NoError(t, executor.unmarshalItem(item, &dest))

		require.Equal(t, "hello", dest["s"])
		require.Equal(t, int64(3), dest["n_int"])
		require.Equal(t, float64(3.14), dest["n_float"])
		require.Equal(t, true, dest["bool"])
		require.Nil(t, dest["null"])
		require.Equal(t, []byte{0x01}, dest["b"])
		require.Equal(t, []string{"a", "b"}, dest["ss"])
		require.Equal(t, []float64{1.5, 2}, dest["ns"])

		list, ok := dest["l"].([]interface{})
		require.True(t, ok)
		require.Equal(t, []interface{}{"x", int64(2), nil}, list)

		m, ok := dest["m"].(map[string]interface{})
		require.True(t, ok)
		require.Equal(t, map[string]interface{}{"k": "v", "n": int64(1)}, m)
	})

	t.Run("invalid_number_errors", func(t *testing.T) {
		item := map[string]types.AttributeValue{
			"n": &types.AttributeValueMemberN{Value: "nope"},
		}
		var dest map[string]any
		require.Error(t, executor.unmarshalItem(item, &dest))
	})

	t.Run("invalid_number_set_errors", func(t *testing.T) {
		item := map[string]types.AttributeValue{
			"ns": &types.AttributeValueMemberNS{Value: []string{"nope"}},
		}
		var dest map[string]any
		require.Error(t, executor.unmarshalItem(item, &dest))
	})

	t.Run("list_conversion_errors", func(t *testing.T) {
		item := map[string]types.AttributeValue{
			"l": &types.AttributeValueMemberL{Value: []types.AttributeValue{
				&types.AttributeValueMemberN{Value: "nope"},
			}},
		}
		var dest map[string]any
		require.Error(t, executor.unmarshalItem(item, &dest))
	})

	t.Run("map_conversion_errors", func(t *testing.T) {
		item := map[string]types.AttributeValue{
			"m": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
				"n": &types.AttributeValueMemberN{Value: "nope"},
			}},
		}
		var dest map[string]any
		require.Error(t, executor.unmarshalItem(item, &dest))
	})

	t.Run("unsupported_attribute_value_type_errors", func(t *testing.T) {
		item := map[string]types.AttributeValue{
			"bad": nil,
		}
		var dest map[string]any
		require.Error(t, executor.unmarshalItem(item, &dest))
	})
}

func TestQueryExecutor_unmarshalItem_MapDestinationTypes_COV7(t *testing.T) {
	executor := &queryExecutor{
		db: &DB{
			converter: pkgTypes.NewConverter(),
		},
	}

	t.Run("map_string_to_attribute_values", func(t *testing.T) {
		item := map[string]types.AttributeValue{
			"s":   &types.AttributeValueMemberS{Value: "hello"},
			"nil": nil,
		}

		var dest map[string]types.AttributeValue
		require.NoError(t, executor.unmarshalItem(item, &dest))
		require.Len(t, dest, 2)
		require.IsType(t, &types.AttributeValueMemberS{}, dest["s"])
		_, ok := dest["nil"]
		require.True(t, ok)
		require.Nil(t, dest["nil"])
	})

	t.Run("map_string_to_string", func(t *testing.T) {
		item := map[string]types.AttributeValue{
			"s": &types.AttributeValueMemberS{Value: "hello"},
		}

		var dest map[string]string
		require.NoError(t, executor.unmarshalItem(item, &dest))
		require.Equal(t, map[string]string{"s": "hello"}, dest)
	})

	t.Run("map_string_to_string_number_errors", func(t *testing.T) {
		item := map[string]types.AttributeValue{
			"n": &types.AttributeValueMemberN{Value: "3"},
		}

		var dest map[string]string
		require.Error(t, executor.unmarshalItem(item, &dest))
	})
}
