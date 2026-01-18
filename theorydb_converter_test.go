package theorydb

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/marshal"
	"github.com/theory-cloud/tabletheory/pkg/model"
	pkgTypes "github.com/theory-cloud/tabletheory/pkg/types"
)

type customContextValue struct {
	Values []string
}

type customContextConverter struct{}

func (customContextConverter) ToAttributeValue(value any) (types.AttributeValue, error) {
	switch v := value.(type) {
	case customContextValue:
		return customContextConverter{}.ToAttributeValue(&v)
	case *customContextValue:
		if v == nil {
			return &types.AttributeValueMemberNULL{Value: true}, nil
		}
		if len(v.Values) == 0 {
			return &types.AttributeValueMemberNULL{Value: true}, nil
		}
		list := make([]types.AttributeValue, len(v.Values))
		for i, s := range v.Values {
			list[i] = &types.AttributeValueMemberS{Value: s}
		}
		return &types.AttributeValueMemberL{Value: list}, nil
	default:
		return nil, fmt.Errorf("unsupported type %T", value)
	}
}

func (customContextConverter) FromAttributeValue(av types.AttributeValue, target any) error {
	var dest *customContextValue

	switch t := target.(type) {
	case *customContextValue:
		dest = t
	case **customContextValue:
		if *t == nil {
			*t = &customContextValue{}
		}
		dest = *t
	default:
		return fmt.Errorf("unsupported target type %T", target)
	}

	switch v := av.(type) {
	case *types.AttributeValueMemberNULL:
		dest.Values = nil
	case *types.AttributeValueMemberS:
		dest.Values = []string{v.Value}
	case *types.AttributeValueMemberL:
		dest.Values = dest.Values[:0]
		for _, item := range v.Value {
			s, ok := item.(*types.AttributeValueMemberS)
			if !ok {
				return fmt.Errorf("expected string list entry, got %T", item)
			}
			dest.Values = append(dest.Values, s.Value)
		}
	default:
		return fmt.Errorf("unsupported AttributeValue type %T", av)
	}

	return nil
}

func TestRegisterTypeConverter_MarshalAndUnmarshal(t *testing.T) {
	conv := pkgTypes.NewConverter()
	db := &DB{
		registry:  model.NewRegistry(),
		converter: conv,
	}
	db.marshaler = marshal.New(conv)

	type note struct {
		ID      string             `theorydb:"pk"`
		Context customContextValue `theorydb:"attr:context"`
	}

	require.NoError(t, db.registry.Register(&note{}))
	metadata, err := db.registry.GetMetadata(&note{})
	require.NoError(t, err)

	// Prime cache without custom converter to ensure registration clears it later.
	priming := &note{ID: "n-1", Context: customContextValue{Values: []string{"init"}}}
	itemBefore, err := db.marshaler.MarshalItem(priming, metadata)
	require.NoError(t, err)
	if _, ok := itemBefore["context"].(*types.AttributeValueMemberL); ok {
		t.Fatalf("expected pre-registration attribute to not be a list")
	}

	require.NoError(t, db.RegisterTypeConverter(reflect.TypeOf(customContextValue{}), customContextConverter{}))

	current := &note{ID: "n-1", Context: customContextValue{Values: []string{"https://example.com/a", "https://example.com/b"}}}
	itemAfter, err := db.marshaler.MarshalItem(current, metadata)
	require.NoError(t, err)

	listAttr, ok := itemAfter["context"].(*types.AttributeValueMemberL)
	require.True(t, ok, "expected context attribute to be marshaled as list")
	require.Len(t, listAttr.Value, 2)
	firstVal, ok := listAttr.Value[0].(*types.AttributeValueMemberS)
	require.True(t, ok)
	require.Equal(t, "https://example.com/a", firstVal.Value)

	// Legacy string data should be normalized through the converter.
	legacyAttr := &types.AttributeValueMemberS{Value: "https://legacy.example"}
	var decoded customContextValue
	require.NoError(t, db.converter.FromAttributeValue(legacyAttr, &decoded))
	require.Equal(t, []string{"https://legacy.example"}, decoded.Values)

	// Ensure lists also round-trip.
	require.NoError(t, db.converter.FromAttributeValue(itemAfter["context"], &decoded))
	require.Equal(t, []string{"https://example.com/a", "https://example.com/b"}, decoded.Values)
}

func TestRegisterTypeConverter_ValidatesInput(t *testing.T) {
	conv := pkgTypes.NewConverter()
	db := &DB{
		converter: conv,
	}
	db.marshaler = marshal.New(conv)

	err := db.RegisterTypeConverter(nil, customContextConverter{})
	require.Error(t, err)

	err = db.RegisterTypeConverter(reflect.TypeOf(customContextValue{}), nil)
	require.Error(t, err)
}
