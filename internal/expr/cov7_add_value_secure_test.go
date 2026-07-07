package expr

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v2/internal/anonymous"
)

type cov7Converter struct {
	wantType reflect.Type
	av       types.AttributeValue
	err      error
	flat     bool

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

func (c *cov7Converter) FlatAnonymousEmbedEncodingEnabled() bool {
	return c.flat
}

type cov7CustomType string

type cov7MarshalerValue struct {
	Value string
}

func (v cov7MarshalerValue) MarshalDynamoDBAttributeValue() (types.AttributeValue, error) {
	return &types.AttributeValueMemberS{Value: "HOOK:" + v.Value}, nil
}

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

	t.Run("FlatAnonymousEmbedEncodingUsesConverterForGenericStructs", func(t *testing.T) {
		type BaseObject struct {
			ID string
		}
		//nolint:govet // Field order mirrors the anonymous-embed contract fixture under test.
		type Activity struct {
			BaseObject
			Actor string
		}

		converter := &cov7Converter{
			flat: true,
			av: &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
				"id":    &types.AttributeValueMemberS{Value: "activity-1"},
				"actor": &types.AttributeValueMemberS{Value: "acct:actor"},
			}},
		}
		b := NewBuilderWithConverter(converter)

		ref, err := b.addValueSecure(Activity{
			BaseObject: BaseObject{ID: "activity-1"},
			Actor:      "acct:actor",
		})
		require.NoError(t, err)
		require.Equal(t, ":v1", ref)
		require.True(t, converter.hasCalled)
		require.True(t, converter.toCalled)
		_, ok := b.values[ref].(*types.AttributeValueMemberM)
		require.True(t, ok)
	})

	t.Run("FlatAnonymousEmbedEncodingStillHonorsMarshalerHooks", func(t *testing.T) {
		converter := &cov7Converter{flat: true}
		b := NewBuilderWithConverter(converter)

		ref, err := b.addValueSecure(cov7MarshalerValue{Value: "plaintext"})
		require.NoError(t, err)
		require.Equal(t, ":v1", ref)
		require.True(t, converter.hasCalled)
		require.False(t, converter.toCalled)

		av, ok := b.values[ref].(*types.AttributeValueMemberS)
		require.True(t, ok)
		require.Equal(t, "HOOK:plaintext", av.Value)
	})
}

func TestFlatAnonymousEmbedEncodingHelpers_COV7(t *testing.T) {
	require.False(t, convertOptionsRequestFlatAnonymousEmbeds(ConvertOptions{}))
	require.True(t, convertOptionsRequestFlatAnonymousEmbeds(ConvertOptions{
		FlatAnonymousEmbedEncoding: true,
	}))

	require.False(t, anonymous.RequestsFlatEncoding(nil))
	require.False(t, anonymous.RequestsFlatEncoding(struct{}{}))
	require.False(t, anonymous.RequestsFlatEncoding(&cov7Converter{}))
	require.True(t, anonymous.RequestsFlatEncoding(&cov7Converter{flat: true}))
}
