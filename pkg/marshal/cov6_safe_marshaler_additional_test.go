package marshal

import (
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/model"
)

func TestSafeMarshaler_getOrBuildSafeStructMarshaler_RebuildsOnBadCache_COV6(t *testing.T) {
	type item struct {
		ID string
	}

	typ := reflect.TypeOf(item{})
	idField, ok := typ.FieldByName("ID")
	require.True(t, ok)

	meta := &model.Metadata{
		Fields:         make(map[string]*model.FieldMetadata),
		FieldsByDBName: make(map[string]*model.FieldMetadata),
		PrimaryKey:     &model.KeySchema{},
	}
	meta.Fields["ID"] = &model.FieldMetadata{
		Name:      "ID",
		DBName:    "id",
		Type:      reflect.TypeOf(""),
		Index:     idField.Index[0],
		IndexPath: idField.Index,
	}
	meta.FieldsByDBName["id"] = meta.Fields["ID"]
	meta.PrimaryKey.PartitionKey = meta.Fields["ID"]

	m := NewSafeMarshaler()
	m.cache.Store(typ, (*safeStructMarshaler)(nil))

	sm := m.getOrBuildSafeStructMarshaler(typ, meta)
	require.NotNil(t, sm)
	require.NotEmpty(t, sm.fields)
}

func TestSafeMarshaler_MarshalItem_NilMapAndInterface_COV6(t *testing.T) {
	type item struct {
		Any  any
		Data map[string]string
		ID   string
	}

	typ := reflect.TypeOf(item{})
	idField, ok := typ.FieldByName("ID")
	require.True(t, ok)
	anyField, ok := typ.FieldByName("Any")
	require.True(t, ok)
	dataField, ok := typ.FieldByName("Data")
	require.True(t, ok)

	meta := &model.Metadata{
		Fields:         make(map[string]*model.FieldMetadata),
		FieldsByDBName: make(map[string]*model.FieldMetadata),
		PrimaryKey:     &model.KeySchema{},
	}

	meta.Fields["ID"] = &model.FieldMetadata{Name: "ID", DBName: "id", Type: reflect.TypeOf(""), Index: idField.Index[0], IndexPath: idField.Index}
	meta.FieldsByDBName["id"] = meta.Fields["ID"]
	meta.PrimaryKey.PartitionKey = meta.Fields["ID"]

	meta.Fields["Any"] = &model.FieldMetadata{Name: "Any", DBName: "any", Type: anyField.Type, Index: anyField.Index[0], IndexPath: anyField.Index}
	meta.FieldsByDBName["any"] = meta.Fields["Any"]

	meta.Fields["Data"] = &model.FieldMetadata{Name: "Data", DBName: "data", Type: dataField.Type, Index: dataField.Index[0], IndexPath: dataField.Index}
	meta.FieldsByDBName["data"] = meta.Fields["Data"]

	m := NewSafeMarshaler()

	out, err := m.MarshalItem(item{ID: "id-1"}, meta)
	require.NoError(t, err)

	require.Equal(t, "id-1", requireAVS(t, out["id"]).Value)

	anyValue, ok := out["any"].(*types.AttributeValueMemberNULL)
	require.True(t, ok)
	require.True(t, anyValue.Value)

	dataValue, ok := out["data"].(*types.AttributeValueMemberNULL)
	require.True(t, ok)
	require.True(t, dataValue.Value)
}
