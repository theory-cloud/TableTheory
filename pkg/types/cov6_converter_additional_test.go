package types

import (
	"reflect"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/naming"
)

func TestRegisterConverter_IgnoresNilInputs_COV6(t *testing.T) {
	converter := NewConverter()

	converter.RegisterConverter(nil, nil)
	converter.RegisterConverter(reflect.TypeOf(""), nil)

	require.False(t, converter.HasCustomConverter(reflect.TypeOf("")))
}

func TestFromAttributeValueTime_ValidatesTypesAndFormats_COV6(t *testing.T) {
	converter := NewConverter()

	t.Run("rejects non-string attribute", func(t *testing.T) {
		var out time.Time
		err := converter.FromAttributeValue(&types.AttributeValueMemberN{Value: "1"}, &out)
		require.Error(t, err)
		require.Contains(t, err.Error(), "expected string")
	})

	t.Run("rejects invalid time format", func(t *testing.T) {
		var out time.Time
		err := converter.FromAttributeValue(&types.AttributeValueMemberS{Value: "not-a-time"}, &out)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid time format")
	})
}

func TestFromAttributeValueMap_RejectsNonMapTargets_COV6(t *testing.T) {
	converter := NewConverter()

	av := &types.AttributeValueMemberM{
		Value: map[string]types.AttributeValue{"a": &types.AttributeValueMemberS{Value: "b"}},
	}

	var out int
	require.Error(t, converter.FromAttributeValue(av, &out))
}

func TestListToSlice_RejectsNonSliceTargets_COV6(t *testing.T) {
	converter := NewConverter()

	av := &types.AttributeValueMemberL{
		Value: []types.AttributeValue{&types.AttributeValueMemberS{Value: "x"}},
	}

	var out int
	require.Error(t, converter.FromAttributeValue(av, &out))
}

func TestAttributeValueMapToMap_RejectsNonStringKeys_COV6(t *testing.T) {
	converter := NewConverter()

	av := &types.AttributeValueMemberM{
		Value: map[string]types.AttributeValue{"a": &types.AttributeValueMemberS{Value: "b"}},
	}

	var out map[int]string
	require.Error(t, converter.FromAttributeValue(av, &out))
}

func TestNumberSetToSlice_RejectsNonSliceTargets_COV6(t *testing.T) {
	converter := NewConverter()

	av := &types.AttributeValueMemberNS{
		Value: []string{"1", "2"},
	}

	var out string
	require.Error(t, converter.FromAttributeValue(av, &out))
}

func TestDetectNamingConvention_AndSplitTag_COV6(t *testing.T) {
	type withSnake struct {
		Name string `theorydb:"naming:snake_case"`
	}

	type withCamel struct {
		Name string `theorydb:"naming:camelCase"`
	}

	type withoutTag struct {
		Name string
	}

	require.Equal(t, naming.SnakeCase, detectNamingConvention(reflect.TypeOf(withSnake{})))
	require.Equal(t, naming.CamelCase, detectNamingConvention(reflect.TypeOf(withCamel{})))
	require.Equal(t, naming.CamelCase, detectNamingConvention(reflect.TypeOf(withoutTag{})))

	require.Nil(t, splitTag(""))
	require.Equal(t, []string{"attr:field", "omitempty", "naming:snake_case"}, splitTag("attr:field, omitempty, naming:snake_case"))
}

func TestMapToStruct_ValidatesAttributeNames_COV6(t *testing.T) {
	converter := NewConverter()

	type badAttr struct {
		Name string `theorydb:"attr:BadName"`
	}

	av := &types.AttributeValueMemberM{
		Value: map[string]types.AttributeValue{"BadName": &types.AttributeValueMemberS{Value: "x"}},
	}

	var out badAttr
	err := converter.FromAttributeValue(av, &out)
	require.Error(t, err)
	require.Contains(t, err.Error(), "attribute name must be camelCase")
}

type cov6BadNumberSetConverter struct{}

func (cov6BadNumberSetConverter) ToAttributeValue(any) (types.AttributeValue, error) {
	return &types.AttributeValueMemberS{Value: "not-a-number"}, nil
}

func (cov6BadNumberSetConverter) FromAttributeValue(types.AttributeValue, any) error {
	return nil
}

func TestConvertToSet_RejectsNonNumericCustomConverter_COV6(t *testing.T) {
	converter := NewConverter()
	converter.RegisterConverter(reflect.TypeOf(int(0)), cov6BadNumberSetConverter{})

	_, err := converter.ConvertToSet([]int{1}, true)
	require.Error(t, err)
	require.Contains(t, err.Error(), "expected number type for set")
}
