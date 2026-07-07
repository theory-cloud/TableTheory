package marshal

import (
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	pkgTypes "github.com/theory-cloud/tabletheory/v2/pkg/types"
)

type Cov7AnonymousEmbedSecret struct {
	Value string
}

type cov7AnonymousEmbedSecretConverter struct{}

func (cov7AnonymousEmbedSecretConverter) ToAttributeValue(value any) (types.AttributeValue, error) {
	secret, ok := value.(Cov7AnonymousEmbedSecret)
	if !ok {
		return nil, nil
	}
	return &types.AttributeValueMemberS{Value: "HOOK:" + secret.Value}, nil
}

func (cov7AnonymousEmbedSecretConverter) FromAttributeValue(types.AttributeValue, any) error {
	return nil
}

func TestMarshaler_AnonymousEmbeddedCustomConverterUsesHook_COV7(t *testing.T) {
	//nolint:govet // Field order mirrors the anonymous-embed hook contract under test.
	type Activity struct {
		Cov7AnonymousEmbedSecret
		Actor string
	}

	converter := pkgTypes.NewConverter()
	converter.RegisterConverter(reflect.TypeOf(Cov7AnonymousEmbedSecret{}), cov7AnonymousEmbedSecretConverter{})

	av, err := New(converter).marshalStructAsMap(reflect.ValueOf(Activity{
		Cov7AnonymousEmbedSecret: Cov7AnonymousEmbedSecret{Value: "plaintext"},
		Actor:                    "acct:actor",
	}))
	require.NoError(t, err)

	activityAV := requireAVM(t, av).Value
	require.Equal(t, "HOOK:plaintext", requireAVS(t, activityAV["cov7AnonymousEmbedSecret"]).Value)
	require.Equal(t, "acct:actor", requireAVS(t, activityAV["actor"]).Value)
	require.NotContains(t, activityAV, "value")
}

func TestSafeMarshaler_AnonymousEmbeddedCustomConverterUsesHook_COV7(t *testing.T) {
	//nolint:govet // Field order mirrors the anonymous-embed hook contract under test.
	type Activity struct {
		Cov7AnonymousEmbedSecret
		Actor string
	}

	converter := pkgTypes.NewConverter()
	converter.RegisterConverter(reflect.TypeOf(Cov7AnonymousEmbedSecret{}), cov7AnonymousEmbedSecretConverter{})

	av, err := NewSafeMarshalerWithConverter(converter).marshalStruct(reflect.ValueOf(Activity{
		Cov7AnonymousEmbedSecret: Cov7AnonymousEmbedSecret{Value: "plaintext"},
		Actor:                    "acct:actor",
	}), &safeFieldMarshaler{})
	require.NoError(t, err)

	activityAV := requireAVM(t, av).Value
	require.Equal(t, "HOOK:plaintext", requireAVS(t, activityAV["cov7AnonymousEmbedSecret"]).Value)
	require.Equal(t, "acct:actor", requireAVS(t, activityAV["actor"]).Value)
	require.NotContains(t, activityAV, "value")
}
