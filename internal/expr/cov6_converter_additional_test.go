package expr

import (
	"reflect"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v2/pkg/naming"
)

func TestFieldNameFromJSONTag_DefaultsWhenEmpty_COV6(t *testing.T) {
	require.Equal(t, "default", fieldNameFromJSONTag("default", ",omitempty"))
	require.Equal(t, "explicit", fieldNameFromJSONTag("default", "explicit,omitempty"))
}

func TestIsZeroValue_CoversPointersInterfacesAndStructs_COV6(t *testing.T) {
	var ptr *int
	require.True(t, isZeroValue(reflect.ValueOf(ptr)))

	val := 1
	ptr = &val
	require.False(t, isZeroValue(reflect.ValueOf(ptr)))

	var iface any
	require.True(t, isZeroValue(reflect.ValueOf(&iface).Elem()))

	iface = "x"
	require.False(t, isZeroValue(reflect.ValueOf(&iface).Elem()))

	require.False(t, isZeroValue(reflect.ValueOf(time.Time{})))
}

func TestAttributeValueToInterface_UnknownTypesAndErrorPropagation_COV6(t *testing.T) {
	type unsupportedAV struct{ types.AttributeValue }

	_, err := attributeValueToInterface(&unsupportedAV{})
	require.Error(t, err)

	_, err = attributeValueListToInterface([]types.AttributeValue{&unsupportedAV{}})
	require.Error(t, err)

	_, err = attributeValueMapToInterface(map[string]types.AttributeValue{"a": &unsupportedAV{}})
	require.Error(t, err)
}

func TestUnmarshalHelpers_ErrorBranches_COV6(t *testing.T) {
	var outString string
	require.Error(t, unmarshalBinary([]byte("x"), reflect.ValueOf(&outString).Elem()))
	require.Error(t, unmarshalBool(true, reflect.ValueOf(&outString).Elem()))
	require.Error(t, unmarshalList([]types.AttributeValue{}, reflect.ValueOf(&outString).Elem()))
	require.Error(t, unmarshalMap(map[string]types.AttributeValue{}, reflect.ValueOf(&outString).Elem()))

	var intKeyed map[int]string
	require.Error(t, unmarshalMapIntoMap(map[string]types.AttributeValue{}, reflect.ValueOf(&intKeyed).Elem()))

	require.Error(t, unmarshalStringSet([]string{"a"}, reflect.ValueOf(&outString).Elem()))
	require.Error(t, unmarshalNumberSet([]string{"1"}, reflect.ValueOf(&outString).Elem()))
	require.Error(t, unmarshalBinarySet([][]byte{[]byte("x")}, reflect.ValueOf(&outString).Elem()))
}

func TestUnmarshalNumberSet_WrapsParseErrors_COV6(t *testing.T) {
	var out []int
	err := unmarshalNumberSet([]string{"not-a-number"}, reflect.ValueOf(&out).Elem())
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to unmarshal number set item")
}

func TestNamingHelpers_COV6(t *testing.T) {
	type withSnake struct {
		_ struct{} `theorydb:"naming:snake_case"`
	}

	type withPascal struct {
		_ struct{} `theorydb:"naming:pascalCase"`
	}

	type withDynamORM struct {
		_ struct{} `theorydb:"naming:dynamorm"`
	}

	type withoutTag struct {
		Name string
	}

	require.Equal(t, naming.SnakeCase, detectNamingConvention(reflect.TypeOf(withSnake{})))
	require.Equal(t, naming.PascalCase, detectNamingConvention(reflect.TypeOf(withPascal{})))
	require.Equal(t, naming.DynamORM, detectNamingConvention(reflect.TypeOf(withDynamORM{})))
	require.Equal(t, naming.CamelCase, detectNamingConvention(reflect.TypeOf(withoutTag{})))

	convention, explicit := explicitNamingConvention(reflect.TypeOf(withDynamORM{}))
	require.True(t, explicit)
	require.Equal(t, naming.DynamORM, convention)

	convention, inherited := resolveStructNaming(reflect.TypeOf(withoutTag{}), naming.SnakeCase, true)
	require.True(t, inherited)
	require.Equal(t, naming.SnakeCase, convention)

	require.True(t, hasStandaloneTagPart("pk,attr:ignored", "pk"))
	require.False(t, hasStandaloneTagPart("attr:pkValue", "pk"))
	require.Equal(t, "created_at", fieldNameFromTheorydbTag("createdAt", "created_at,omitempty", naming.CamelCase))
	require.Equal(t, "PK", fieldNameFromTheorydbTag("userID", "pk", naming.DynamORM))
	require.Equal(t, "SK", fieldNameFromTheorydbTag("entity", "sk", naming.DynamORM))
	require.Equal(t, "json_name", jsonTagName("json_name,omitempty"))
	require.Empty(t, jsonTagName(",omitempty"))
}

func TestLookupHelpers_COV6(t *testing.T) {
	names := appendUniqueLookupName(nil, "first")
	names = appendUniqueLookupName(names, "first")
	names = appendUniqueLookupName(names, "")
	names = appendUniqueLookupName(names, "second")
	require.Equal(t, []string{"first", "second"}, names)

	value, ok := lookupMapValue(map[string]types.AttributeValue{
		"second": &types.AttributeValueMemberS{Value: "value"},
	}, "missing", "second")
	require.True(t, ok)
	valueAV, ok := value.(*types.AttributeValueMemberS)
	require.True(t, ok)
	require.Equal(t, "value", valueAV.Value)

	_, ok = lookupMapValue(map[string]types.AttributeValue{}, "missing")
	require.False(t, ok)
}

func TestConvertToAttributeValueWithConvention_InterfaceBranch_COV6(t *testing.T) {
	type nested struct {
		CreatedAt string
	}

	var value any = nested{CreatedAt: "now"}
	av, err := convertToAttributeValueWithConvention(reflect.ValueOf(&value).Elem(), naming.SnakeCase, true, ConvertOptions{})
	require.NoError(t, err)

	m, ok := av.(*types.AttributeValueMemberM)
	require.True(t, ok)
	require.Contains(t, m.Value, "created_at")
}
