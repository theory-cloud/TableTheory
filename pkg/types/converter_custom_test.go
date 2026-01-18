package types

import (
	"reflect"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeCustomConverter struct {
	to   func(value any) (types.AttributeValue, error)
	from func(av types.AttributeValue, target any) error
}

func (f fakeCustomConverter) ToAttributeValue(value any) (types.AttributeValue, error) {
	if f.to == nil {
		panic("unexpected call to ToAttributeValue with no stub")
	}
	return f.to(value)
}

func (f fakeCustomConverter) FromAttributeValue(av types.AttributeValue, target any) error {
	if f.from == nil {
		panic("unexpected call to FromAttributeValue with no stub")
	}
	return f.from(av, target)
}

type customPayload struct {
	Raw string
}

func TestConverterHasCustomConverterAndPointerLookup(t *testing.T) {
	converter := NewConverter()

	convType := reflect.TypeOf(customPayload{})
	fake := fakeCustomConverter{
		to: func(value any) (types.AttributeValue, error) {
			payload, ok := value.(customPayload)
			if !ok {
				panic("unexpected type: expected customPayload")
			}
			return &types.AttributeValueMemberS{Value: strings.ToUpper(payload.Raw)}, nil
		},
		from: func(av types.AttributeValue, target any) error {
			out, ok := target.(*customPayload)
			if !ok {
				panic("unexpected type: expected *customPayload")
			}
			strAV, ok := av.(*types.AttributeValueMemberS)
			if !ok {
				panic("unexpected type: expected *types.AttributeValueMemberS")
			}
			strVal := strAV.Value
			out.Raw = strings.ToLower(strVal)
			return nil
		},
	}

	converter.RegisterConverter(convType, fake)

	t.Run("nil type returns no converter", func(t *testing.T) {
		conv, ok := converter.lookupConverter(nil)
		assert.False(t, ok)
		assert.Nil(t, conv)
	})

	t.Run("pointer types reuse registered converter", func(t *testing.T) {
		ptrType := reflect.TypeOf((*customPayload)(nil))
		assert.True(t, converter.HasCustomConverter(ptrType))

		conv, ok := converter.lookupConverter(ptrType)
		require.True(t, ok)
		require.NotNil(t, conv)

		av, err := conv.ToAttributeValue(customPayload{Raw: "pointer"})
		require.NoError(t, err)
		strVal, ok := av.(*types.AttributeValueMemberS)
		require.True(t, ok)
		assert.Equal(t, "POINTER", strVal.Value)
	})

	t.Run("custom converter used in ToAttributeValue and FromAttributeValue", func(t *testing.T) {
		input := &customPayload{Raw: "mixedCase"}

		av, err := converter.ToAttributeValue(input)
		require.NoError(t, err)

		strVal, ok := av.(*types.AttributeValueMemberS)
		require.True(t, ok)
		assert.Equal(t, "MIXEDCASE", strVal.Value)

		var output customPayload
		err = converter.FromAttributeValue(av, &output)
		require.NoError(t, err)
		assert.Equal(t, "mixedcase", output.Raw)
	})

	t.Run("nil pointer input still returns NULL attribute", func(t *testing.T) {
		var payload *customPayload
		av, err := converter.ToAttributeValue(payload)
		require.NoError(t, err)

		_, ok := av.(*types.AttributeValueMemberNULL)
		assert.True(t, ok)
	})
}
