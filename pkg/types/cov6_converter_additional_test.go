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

	type withPascal struct {
		Name string `theorydb:"naming:pascalCase"`
	}

	type withDynamORM struct {
		Name string `theorydb:"naming:dynamorm"`
	}

	type withoutTag struct {
		Name string
	}

	require.Equal(t, naming.SnakeCase, detectNamingConvention(reflect.TypeOf(withSnake{})))
	require.Equal(t, naming.CamelCase, detectNamingConvention(reflect.TypeOf(withCamel{})))
	require.Equal(t, naming.PascalCase, detectNamingConvention(reflect.TypeOf(withPascal{})))
	require.Equal(t, naming.DynamORM, detectNamingConvention(reflect.TypeOf(withDynamORM{})))
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

func TestStructAndMapToStruct_DynamORMNaming_COV6(t *testing.T) {
	converter := NewConverter()

	type legacy struct {
		_ struct{} `theorydb:"naming:dynamorm"`

		UserID    string `theorydb:"pk"`
		Entity    string `theorydb:"sk"`
		FirstName string
	}

	av, err := converter.ToAttributeValue(legacy{
		UserID:    "USER#1",
		Entity:    "PROFILE",
		FirstName: "Ada",
	})
	require.NoError(t, err)

	m, ok := av.(*types.AttributeValueMemberM)
	require.True(t, ok)
	require.Contains(t, m.Value, "PK")
	require.Contains(t, m.Value, "SK")
	require.Contains(t, m.Value, "firstName")

	var out legacy
	require.NoError(t, converter.FromAttributeValue(av, &out))
	require.Equal(t, "USER#1", out.UserID)
	require.Equal(t, "PROFILE", out.Entity)
	require.Equal(t, "Ada", out.FirstName)
}

func TestStructAndMapToStruct_DynamORMNaming_NestedStruct_COV6(t *testing.T) {
	converter := NewConverter()

	type address struct {
		PostalCode  string
		CountryCode string
	}

	type profile struct {
		DisplayName    string
		MailingAddress address
	}

	type legacy struct {
		_ struct{} `theorydb:"naming:dynamorm"`

		UserID  string `theorydb:"pk"`
		Entity  string `theorydb:"sk"`
		Profile profile
	}

	av, err := converter.ToAttributeValue(legacy{
		UserID: "USER#1",
		Entity: "PROFILE",
		Profile: profile{
			DisplayName: "Ada Lovelace",
			MailingAddress: address{
				PostalCode:  "10001",
				CountryCode: "US",
			},
		},
	})
	require.NoError(t, err)

	m, ok := av.(*types.AttributeValueMemberM)
	require.True(t, ok)
	profileAV, ok := m.Value["profile"].(*types.AttributeValueMemberM)
	require.True(t, ok)
	require.Contains(t, profileAV.Value, "displayName")
	require.NotContains(t, profileAV.Value, "DisplayName")
	addressAV, ok := profileAV.Value["mailingAddress"].(*types.AttributeValueMemberM)
	require.True(t, ok)
	require.Contains(t, addressAV.Value, "postalCode")
	require.Contains(t, addressAV.Value, "countryCode")
	require.NotContains(t, addressAV.Value, "PostalCode")
	require.NotContains(t, addressAV.Value, "CountryCode")

	var out legacy
	require.NoError(t, converter.FromAttributeValue(av, &out))
	require.Equal(t, "USER#1", out.UserID)
	require.Equal(t, "PROFILE", out.Entity)
	require.Equal(t, "Ada Lovelace", out.Profile.DisplayName)
	require.Equal(t, "10001", out.Profile.MailingAddress.PostalCode)
	require.Equal(t, "US", out.Profile.MailingAddress.CountryCode)
}

func TestMapToStructWithConvention_DynamORMNestedJSONTags_COV6(t *testing.T) {
	converter := NewConverter()

	type address struct {
		PostalCode  string `json:"postal_code"`
		CountryCode string `json:"country_code"`
	}

	type profile struct {
		DisplayName    string  `json:"display_name"`
		MailingAddress address `json:"mailing_address"`
	}

	type legacy struct {
		_       struct{} `theorydb:"naming:dynamorm"`
		UserID  string   `theorydb:"pk"`
		Entity  string   `theorydb:"sk"`
		Profile profile
	}

	av := &types.AttributeValueMemberM{
		Value: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "USER#1"},
			"SK": &types.AttributeValueMemberS{Value: "PROFILE"},
			"profile": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
				"display_name": &types.AttributeValueMemberS{Value: "Ada Lovelace"},
				"mailing_address": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
					"postal_code":  &types.AttributeValueMemberS{Value: "10001"},
					"country_code": &types.AttributeValueMemberS{Value: "US"},
				}},
			}},
		},
	}

	var out legacy
	require.NoError(t, converter.FromAttributeValue(av, &out))
	require.Equal(t, "USER#1", out.UserID)
	require.Equal(t, "PROFILE", out.Entity)
	require.Equal(t, "Ada Lovelace", out.Profile.DisplayName)
	require.Equal(t, "10001", out.Profile.MailingAddress.PostalCode)
	require.Equal(t, "US", out.Profile.MailingAddress.CountryCode)
}

func TestStructAndMapToStruct_DynamORMAcronymNestedFields_COV6(t *testing.T) {
	converter := NewConverter()

	type profile struct {
		MerchantTaxIDSecID string
		IdentityID         string
		DID                string
		TPPID              string
	}

	type legacy struct {
		_       struct{} `theorydb:"naming:dynamorm"`
		UserID  string   `theorydb:"pk"`
		Entity  string   `theorydb:"sk"`
		Profile profile
	}

	av, err := converter.ToAttributeValue(legacy{
		UserID: "USER#1",
		Entity: "PROFILE",
		Profile: profile{
			MerchantTaxIDSecID: "tax-1",
			IdentityID:         "identity-1",
			DID:                "did-1",
			TPPID:              "tpp-1",
		},
	})
	require.NoError(t, err)

	m, ok := av.(*types.AttributeValueMemberM)
	require.True(t, ok)
	profileAV, ok := m.Value["profile"].(*types.AttributeValueMemberM)
	require.True(t, ok)
	require.Contains(t, profileAV.Value, "merchantTaxIDSecID")
	require.Contains(t, profileAV.Value, "identityID")
	require.Contains(t, profileAV.Value, "did")
	require.Contains(t, profileAV.Value, "tppid")

	var out legacy
	require.NoError(t, converter.FromAttributeValue(av, &out))
	require.Equal(t, "tax-1", out.Profile.MerchantTaxIDSecID)
	require.Equal(t, "identity-1", out.Profile.IdentityID)
	require.Equal(t, "did-1", out.Profile.DID)
	require.Equal(t, "tpp-1", out.Profile.TPPID)
}

func TestMapLookupHelpers_COV6(t *testing.T) {
	type model struct {
		JSONNamed string `json:"json_name,omitempty"`
	}

	require.Equal(t, "json_name", jsonTagName("json_name,omitempty"))
	require.Empty(t, jsonTagName(",omitempty"))

	names := appendMapLookupName(nil, "first")
	names = appendMapLookupName(names, "first")
	names = appendMapLookupName(names, "")
	names = appendMapLookupName(names, "second")
	require.Equal(t, []string{"first", "second"}, names)

	av, ok := lookupMapFieldValue(map[string]types.AttributeValue{
		"second": &types.AttributeValueMemberS{Value: "value"},
	}, "missing", "second")
	require.True(t, ok)
	valueAV, ok := av.(*types.AttributeValueMemberS)
	require.True(t, ok)
	require.Equal(t, "value", valueAV.Value)

	field := reflect.TypeOf(model{}).Field(0)
	attrNames, skip, err := resolveMapFieldLookupNames(field, naming.CamelCase)
	require.NoError(t, err)
	require.False(t, skip)
	require.Equal(t, []string{"json_name", "jsonNamed", "JSONNamed"}, attrNames)
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
