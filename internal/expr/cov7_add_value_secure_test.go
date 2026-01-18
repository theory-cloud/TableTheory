package expr

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"
)

type cov7Converter struct {
	wantType reflect.Type
	av       types.AttributeValue
	err      error

	hasCalled bool
	toCalled  bool
}

func (c *cov7Converter) HasCustomConverter(typ reflect.Type) bool {
	c.hasCalled = true
	return typ == c.wantType
}

func (c *cov7Converter) ToAttributeValue(value any) (types.AttributeValue, error) {
	c.toCalled = true
	return c.av, c.err
}

type cov7CustomType string

func TestBuilder_addValueSecure_CustomConverterAndValidationPaths_COV7(t *testing.T) {
	t.Run("CustomConverterSuccess", func(t *testing.T) {
		converter := &cov7Converter{
			wantType: reflect.TypeOf(cov7CustomType("x")),
			av:       &types.AttributeValueMemberS{Value: "converted"},
		}
		b := NewBuilderWithConverter(converter)

		ref, err := b.addValueSecure(cov7CustomType("x"))
		require.NoError(t, err)
		require.Equal(t, ":v1", ref)
		require.True(t, converter.hasCalled)
		require.True(t, converter.toCalled)
		require.Equal(t, &types.AttributeValueMemberS{Value: "converted"}, b.values[ref])
	})

	t.Run("CustomConverterError", func(t *testing.T) {
		converter := &cov7Converter{
			wantType: reflect.TypeOf(cov7CustomType("x")),
			err:      fmt.Errorf("boom"),
		}
		b := NewBuilderWithConverter(converter)

		ref, err := b.addValueSecure(cov7CustomType("x"))
		require.Error(t, err)
		require.Empty(t, ref)
		require.True(t, converter.hasCalled)
		require.True(t, converter.toCalled)
		require.Equal(t, 0, b.valueCounter)
		require.Empty(t, b.values)
	})

	t.Run("ValidationErrorWhenNoConverter", func(t *testing.T) {
		converter := &cov7Converter{
			wantType: reflect.TypeOf(cov7CustomType("x")),
		}
		b := NewBuilderWithConverter(converter)

		ref, err := b.addValueSecure(make(chan int))
		require.Error(t, err)
		require.Empty(t, ref)
	})
}
