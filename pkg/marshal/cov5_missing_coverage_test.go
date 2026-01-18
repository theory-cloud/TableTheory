package marshal

import (
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/model"
)

func TestMarshaler_jsonTagName_ParsesBeforeComma(t *testing.T) {
	require.Equal(t, "field", jsonTagName("field,omitempty"))
	require.Equal(t, "field", jsonTagName("field"))
}

func TestMarshaler_marshalValue_CoversPointerAndInterfaceKinds(t *testing.T) {
	m := New(nil)

	value := "x"
	av, err := m.marshalValue(reflect.ValueOf(&value))
	require.NoError(t, err)
	member, ok := av.(*types.AttributeValueMemberS)
	require.True(t, ok)
	require.Equal(t, "x", member.Value)

	var iface any = "y"
	av, err = m.marshalValue(reflect.ValueOf(&iface).Elem())
	require.NoError(t, err)
	member, ok = av.(*types.AttributeValueMemberS)
	require.True(t, ok)
	require.Equal(t, "y", member.Value)

	var nilIface any
	av, err = m.marshalValue(reflect.ValueOf(&nilIface).Elem())
	require.NoError(t, err)
	require.IsType(t, &types.AttributeValueMemberNULL{}, av)
}

func TestSafeMarshaler_marshalValue_CoversInterfaceKind(t *testing.T) {
	type item struct {
		Any any
		ID  string
	}

	typ := reflect.TypeOf(item{})
	idField, ok := typ.FieldByName("ID")
	require.True(t, ok)
	anyField, ok := typ.FieldByName("Any")
	require.True(t, ok)

	metadata := &model.Metadata{
		Fields:         make(map[string]*model.FieldMetadata),
		FieldsByDBName: make(map[string]*model.FieldMetadata),
	}
	metadata.Fields["ID"] = &model.FieldMetadata{Name: "ID", DBName: "id", Type: reflect.TypeOf(""), Index: idField.Index[0], IndexPath: idField.Index}
	metadata.FieldsByDBName["id"] = metadata.Fields["ID"]
	metadata.Fields["Any"] = &model.FieldMetadata{Name: "Any", DBName: "any", Type: reflect.TypeOf((*any)(nil)).Elem(), Index: anyField.Index[0], IndexPath: anyField.Index}
	metadata.FieldsByDBName["any"] = metadata.Fields["Any"]

	marshaler := NewSafeMarshaler()

	out, err := marshaler.MarshalItem(item{ID: "id-1", Any: "value"}, metadata)
	require.NoError(t, err)
	anyValue, ok := out["any"].(*types.AttributeValueMemberS)
	require.True(t, ok)
	require.Equal(t, "value", anyValue.Value)
}
