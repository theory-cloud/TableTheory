package types

import (
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"
)

// HookedEmbed is an anonymous embedded type with a registered custom converter.
type HookedEmbed struct {
	Value string
}

//nolint:govet // Field order and the anonymous embed mirror the production fixture under test.
type hookedEmbedItem struct {
	HookedEmbed
	Name string
}

type hookConverter struct {
	calls *int
}

func (c hookConverter) ToAttributeValue(_ any) (types.AttributeValue, error) {
	*c.calls++
	return &types.AttributeValueMemberS{Value: "hooked"}, nil
}

func (c hookConverter) FromAttributeValue(_ types.AttributeValue, _ any) error {
	return nil
}

func TestToAttributeValueInvokesCustomConverterForAnonymousEmbed(t *testing.T) {
	converter := NewConverter()
	calls := 0
	converter.RegisterConverter(reflect.TypeOf(HookedEmbed{}), hookConverter{calls: &calls})

	av, err := converter.ToAttributeValue(hookedEmbedItem{HookedEmbed: HookedEmbed{Value: "raw"}, Name: "n"})
	require.NoError(t, err)
	require.Equal(t, 1, calls, "the registered converter must run for the anonymous embed")

	got := messageMap(t, av)
	require.Contains(t, got, "HookedEmbed")
	require.Equal(t, "hooked", requireString(t, got["HookedEmbed"]))
	// The embed is terminal, so its exported members must not be decomposed.
	require.NotContains(t, got, "Value")
}

func requireString(t *testing.T, av types.AttributeValue) string {
	t.Helper()
	s, ok := av.(*types.AttributeValueMemberS)
	require.Truef(t, ok, "expected string attribute value, got %T", av)
	return s.Value
}
