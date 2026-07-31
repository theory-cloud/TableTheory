package query

import (
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v3/internal/expr"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	pkgtypes "github.com/theory-cloud/tabletheory/v3/pkg/types"
)

type Cov7TaggedSecret struct {
	Value string
}

type cov7TaggedSecretConverter struct{}

func (cov7TaggedSecretConverter) ToAttributeValue(value any) (types.AttributeValue, error) {
	secret, ok := value.(Cov7TaggedSecret)
	if !ok {
		return nil, nil
	}
	return &types.AttributeValueMemberS{Value: "HOOK:" + secret.Value}, nil
}

func (cov7TaggedSecretConverter) FromAttributeValue(types.AttributeValue, any) error { return nil }

type Cov7TaggedMarshalerSecret struct {
	Value string
}

func (s Cov7TaggedMarshalerSecret) MarshalDynamoDBAttributeValue() (types.AttributeValue, error) {
	return &types.AttributeValueMemberS{Value: "HOOK:" + s.Value}, nil
}

func TestQuery_FlatTaggedHelpers_AnonymousEmbeddedCustomConverterWins_COV7(t *testing.T) {
	//nolint:govet // Field order mirrors the anonymous-embed hook contract under test.
	type item struct {
		Cov7TaggedSecret
		ID     string `theorydb:"pk"`
		Status string `theorydb:"attr:status"`
	}

	converter := pkgtypes.NewConverter()
	converter.RegisterConverter(reflect.TypeOf(Cov7TaggedSecret{}), cov7TaggedSecretConverter{})
	converter.WithFlatAnonymousEmbedEncoding()

	q := New(&item{}, &cov4Metadata{
		table: "tbl",
		pk:    core.KeySchema{PartitionKey: "ID"},
		attrs: map[string]string{
			"Status": "status",
		},
	}, &cov4Executor{}).WithConverter(converter)

	modelValue := reflect.ValueOf(item{
		Cov7TaggedSecret: Cov7TaggedSecret{Value: "plaintext"},
		ID:               "id-1",
		Status:           "ready",
	})

	out, err := q.marshalItemTaggedFlat(modelValue)
	require.NoError(t, err)

	secretAV, ok := out["Cov7TaggedSecret"]
	require.True(t, ok, "expected anonymous embedded custom converter field to be preserved as a terminal field")
	secretString, ok := secretAV.(*types.AttributeValueMemberS)
	require.True(t, ok)
	require.Equal(t, "HOOK:plaintext", secretString.Value)
	require.Contains(t, out, "Status")
	require.NotContains(t, out, "Value")

	builder := expr.NewBuilderWithConverter(q.converter)
	require.NoError(t, q.buildUpdateExpressionFromTaggedVisibleFields(builder, modelValue, q.metadata.PrimaryKey()))

	components := builder.Build()
	require.NotEmpty(t, components.UpdateExpression)

	foundHook := false
	for _, av := range components.ExpressionAttributeValues {
		if strAV, ok := av.(*types.AttributeValueMemberS); ok && strAV.Value == "HOOK:plaintext" {
			foundHook = true
		}
	}
	require.True(t, foundHook, "expected update expression values to use the anonymous embedded custom converter output")

	for _, name := range components.ExpressionAttributeNames {
		require.NotEqual(t, "Value", name)
	}
}

func TestQuery_FlatTaggedHelpers_AnonymousEmbeddedMarshalerWins_COV7(t *testing.T) {
	//nolint:govet // Field order mirrors the anonymous-embed hook contract under test.
	type item struct {
		Cov7TaggedMarshalerSecret
		ID     string `theorydb:"pk"`
		Status string `theorydb:"attr:status"`
	}

	converter := pkgtypes.NewConverter().WithFlatAnonymousEmbedEncoding()
	q := New(&item{}, &cov4Metadata{
		table: "tbl",
		pk:    core.KeySchema{PartitionKey: "ID"},
	}, &cov4Executor{}).WithConverter(converter)

	out, err := q.marshalItemTaggedFlat(reflect.ValueOf(item{
		Cov7TaggedMarshalerSecret: Cov7TaggedMarshalerSecret{Value: "plaintext"},
		ID:                        "id-1",
		Status:                    "ready",
	}))
	require.NoError(t, err)

	secretAV, ok := out["Cov7TaggedMarshalerSecret"].(*types.AttributeValueMemberS)
	require.True(t, ok)
	require.Equal(t, "HOOK:plaintext", secretAV.Value)
	require.NotContains(t, out, "Value")
}
