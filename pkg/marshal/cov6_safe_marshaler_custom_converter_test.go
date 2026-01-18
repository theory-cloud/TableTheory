package marshal

import (
	"fmt"
	"reflect"
	"strconv"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/model"
	pkgTypes "github.com/theory-cloud/tabletheory/pkg/types"
)

type customNumber int

type customNumberConverter struct{}

func (customNumberConverter) ToAttributeValue(value any) (types.AttributeValue, error) {
	v, ok := value.(customNumber)
	if !ok {
		return nil, fmt.Errorf("expected customNumber, got %T", value)
	}
	if v < 0 {
		return nil, fmt.Errorf("negative values rejected")
	}
	return &types.AttributeValueMemberN{Value: strconv.Itoa(int(v))}, nil
}

func (customNumberConverter) FromAttributeValue(_ types.AttributeValue, _ any) error {
	return nil
}

func TestSafeMarshaler_MarshalItem_UsesCustomConverter_COV6(t *testing.T) {
	type item struct {
		ID     string
		Custom customNumber
	}

	converter := pkgTypes.NewConverter()
	converter.RegisterConverter(reflect.TypeOf(customNumber(0)), customNumberConverter{})

	typ := reflect.TypeOf(item{})
	meta := createMetadata(
		createFieldMetadata(typ, "ID", "id", reflect.TypeOf("")),
		createFieldMetadata(typ, "Custom", "custom", reflect.TypeOf(customNumber(0))),
	)
	meta.PrimaryKey = &model.KeySchema{PartitionKey: meta.Fields["ID"]}

	m := NewSafeMarshalerWithConverter(converter)
	out, err := m.MarshalItem(item{ID: "id-1", Custom: 12}, meta)
	require.NoError(t, err)
	require.Equal(t, "12", requireAVN(t, out["custom"]).Value)
}

func TestSafeMarshaler_MarshalItem_CustomConverterError_COV6(t *testing.T) {
	type item struct {
		ID     string
		Custom customNumber
	}

	converter := pkgTypes.NewConverter()
	converter.RegisterConverter(reflect.TypeOf(customNumber(0)), customNumberConverter{})

	typ := reflect.TypeOf(item{})
	meta := createMetadata(
		createFieldMetadata(typ, "ID", "id", reflect.TypeOf("")),
		createFieldMetadata(typ, "Custom", "custom", reflect.TypeOf(customNumber(0))),
	)
	meta.PrimaryKey = &model.KeySchema{PartitionKey: meta.Fields["ID"]}

	m := NewSafeMarshalerWithConverter(converter)
	_, err := m.MarshalItem(item{ID: "id-1", Custom: -1}, meta)
	require.Error(t, err)
}
