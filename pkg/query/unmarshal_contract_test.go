package query

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	customerrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	"github.com/theory-cloud/tabletheory/v2/pkg/naming"
)

func TestUnmarshalItem_TableTheoryTagSemantics_CON3(t *testing.T) {
	type model struct {
		_ struct{} `theorydb:"naming:snake_case"`

		ID        string `theorydb:"pk"`
		SK        string `theorydb:"sk"`
		UserID    string
		CreatedAt time.Time `theorydb:"created_at"`
		Custom    string    `theorydb:"attr:custom_name"`
	}

	item := map[string]types.AttributeValue{
		"id":          &types.AttributeValueMemberS{Value: "p1"},
		"sk":          &types.AttributeValueMemberS{Value: "s1"},
		"user_id":     &types.AttributeValueMemberS{Value: "u1"},
		"created_at":  &types.AttributeValueMemberS{Value: "2020-01-01T00:00:00Z"},
		"custom_name": &types.AttributeValueMemberS{Value: "c"},
	}

	var out model
	require.NoError(t, UnmarshalItem(item, &out))
	require.Equal(t, "p1", out.ID)
	require.Equal(t, "s1", out.SK)
	require.Equal(t, "u1", out.UserID)
	require.Equal(t, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), out.CreatedAt)
	require.Equal(t, "c", out.Custom)
}

func TestUnmarshalItem_DynamORMNamingConvention_CON3(t *testing.T) {
	type model struct {
		_ struct{} `theorydb:"naming:dynamorm"`

		UserID    string `theorydb:"pk"`
		Entity    string `theorydb:"sk"`
		FirstName string
	}

	item := map[string]types.AttributeValue{
		"PK":        &types.AttributeValueMemberS{Value: "USER#1"},
		"SK":        &types.AttributeValueMemberS{Value: "PROFILE"},
		"firstName": &types.AttributeValueMemberS{Value: "Ada"},
	}

	var out model
	require.NoError(t, UnmarshalItem(item, &out))
	require.Equal(t, "USER#1", out.UserID)
	require.Equal(t, "PROFILE", out.Entity)
	require.Equal(t, "Ada", out.FirstName)
}

func TestUnmarshalItem_DynamORMNamingConvention_NestedStruct_CON3(t *testing.T) {
	type address struct {
		PostalCode  string
		CountryCode string
	}

	type profile struct {
		DisplayName    string
		MailingAddress address
	}

	type model struct {
		_ struct{} `theorydb:"naming:dynamorm"`

		UserID  string `theorydb:"pk"`
		Entity  string `theorydb:"sk"`
		Profile profile
	}

	item := map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: "USER#1"},
		"SK": &types.AttributeValueMemberS{Value: "PROFILE"},
		"profile": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
			"displayName": &types.AttributeValueMemberS{Value: "Ada Lovelace"},
			"mailingAddress": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
				"postalCode":  &types.AttributeValueMemberS{Value: "10001"},
				"countryCode": &types.AttributeValueMemberS{Value: "US"},
			}},
		}},
	}

	var out model
	require.NoError(t, UnmarshalItem(item, &out))
	require.Equal(t, "USER#1", out.UserID)
	require.Equal(t, "PROFILE", out.Entity)
	require.Equal(t, "Ada Lovelace", out.Profile.DisplayName)
	require.Equal(t, "10001", out.Profile.MailingAddress.PostalCode)
	require.Equal(t, "US", out.Profile.MailingAddress.CountryCode)
}

func TestUnmarshalItem_DynamORMNamingConvention_NestedStruct_PascalFallback_CON3(t *testing.T) {
	type address struct {
		PostalCode  string
		CountryCode string
	}

	type profile struct {
		DisplayName    string
		MailingAddress address
	}

	type model struct {
		_ struct{} `theorydb:"naming:dynamorm"`

		UserID  string `theorydb:"pk"`
		Entity  string `theorydb:"sk"`
		Profile profile
	}

	item := map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: "USER#1"},
		"SK": &types.AttributeValueMemberS{Value: "PROFILE"},
		"profile": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
			"DisplayName": &types.AttributeValueMemberS{Value: "Ada Lovelace"},
			"MailingAddress": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
				"PostalCode":  &types.AttributeValueMemberS{Value: "10001"},
				"CountryCode": &types.AttributeValueMemberS{Value: "US"},
			}},
		}},
	}

	var out model
	require.NoError(t, UnmarshalItem(item, &out))
	require.Equal(t, "Ada Lovelace", out.Profile.DisplayName)
	require.Equal(t, "10001", out.Profile.MailingAddress.PostalCode)
	require.Equal(t, "US", out.Profile.MailingAddress.CountryCode)
}

func TestUnmarshalItem_PromotedAnonymousEmbeds_CON3(t *testing.T) {
	type BaseObject struct {
		ID   string
		Type string
		To   []string
	}

	//nolint:govet // Field order mirrors the anonymous-embed contract fixture under test.
	type activity struct {
		BaseObject
		Actor  string
		Object string
	}

	expected := activity{
		BaseObject: BaseObject{
			ID:   "https://example.com/activities/1",
			Type: "Create",
			To: []string{
				"https://www.w3.org/ns/activitystreams#Public",
				"https://example.com/users/alice/followers",
			},
		},
		Actor:  "https://example.com/users/alice",
		Object: "https://example.com/notes/1",
	}

	testCases := []struct {
		item map[string]types.AttributeValue
		name string
	}{
		{
			name: "flat promoted-field payload",
			item: map[string]types.AttributeValue{
				"id":     &types.AttributeValueMemberS{Value: expected.ID},
				"type":   &types.AttributeValueMemberS{Value: expected.Type},
				"to":     queryStringListAttributeValue(expected.To),
				"actor":  &types.AttributeValueMemberS{Value: expected.Actor},
				"object": &types.AttributeValueMemberS{Value: expected.Object},
			},
		},
		{
			name: "legacy nested helper payload",
			item: map[string]types.AttributeValue{
				"baseObject": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
					"id":   &types.AttributeValueMemberS{Value: expected.ID},
					"type": &types.AttributeValueMemberS{Value: expected.Type},
					"to":   queryStringListAttributeValue(expected.To),
				}},
				"actor":  &types.AttributeValueMemberS{Value: expected.Actor},
				"object": &types.AttributeValueMemberS{Value: expected.Object},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var out activity
			require.NoError(t, UnmarshalItem(tc.item, &out))
			require.Equal(t, expected, out)
		})
	}
}

func TestUnmarshalItem_NestedPromotedAnonymousEmbeds_CON3(t *testing.T) {
	type BaseObject struct {
		ID   string
		Type string
	}

	type activity struct {
		BaseObject
		Actor string
	}

	type envelope struct {
		Activity activity
	}

	expected := envelope{
		Activity: activity{
			BaseObject: BaseObject{
				ID:   "https://example.com/activities/1",
				Type: "Like",
			},
			Actor: "https://example.com/users/alice",
		},
	}

	testCases := []struct {
		item map[string]types.AttributeValue
		name string
	}{
		{
			name: "nested flat promoted-field payload",
			item: map[string]types.AttributeValue{
				"activity": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
					"id":    &types.AttributeValueMemberS{Value: expected.Activity.ID},
					"type":  &types.AttributeValueMemberS{Value: expected.Activity.Type},
					"actor": &types.AttributeValueMemberS{Value: expected.Activity.Actor},
				}},
			},
		},
		{
			name: "nested legacy anonymous-container payload",
			item: map[string]types.AttributeValue{
				"activity": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
					"baseObject": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
						"id":   &types.AttributeValueMemberS{Value: expected.Activity.ID},
						"type": &types.AttributeValueMemberS{Value: expected.Activity.Type},
					}},
					"actor": &types.AttributeValueMemberS{Value: expected.Activity.Actor},
				}},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var out envelope
			require.NoError(t, UnmarshalItem(tc.item, &out))
			require.Equal(t, expected, out)
		})
	}
}

func TestResolveUnmarshalTarget_RejectsInvalidDestinations_CON3(t *testing.T) {
	type model struct {
		Name string
	}

	_, _, _, err := resolveUnmarshalTarget(model{})
	require.EqualError(t, err, "destination must be a pointer")

	var nilModel *model
	_, _, _, err = resolveUnmarshalTarget(nilModel)
	require.EqualError(t, err, "destination must be a pointer")

	value := 1
	_, _, _, err = resolveUnmarshalTarget(&value)
	require.EqualError(t, err, "destination must be a pointer to a struct")
}

func TestResolveUnmarshalFieldLookupNames_CON3(t *testing.T) {
	type model struct {
		Ignored     string `dynamodb:"-"`
		Tagged      string `dynamodb:"custom_name,omitempty"`
		FieldNamed  string `dynamodb:",omitempty"`
		CamelName   string
		JSONNamed   string `json:"json_name,omitempty"`
		TheorySkip  string `theorydb:"-"`
		JSONSkipped string `json:"-"`
	}

	modelType := reflect.TypeOf(model{})

	attrNames, skip := resolveUnmarshalFieldLookupNames(modelType.Field(0), naming.CamelCase)
	require.True(t, skip)
	require.Empty(t, attrNames)

	attrNames, skip = resolveUnmarshalFieldLookupNames(modelType.Field(1), naming.CamelCase)
	require.False(t, skip)
	require.Equal(t, []string{"custom_name", "tagged"}, attrNames[:2])

	attrNames, skip = resolveUnmarshalFieldLookupNames(modelType.Field(2), naming.CamelCase)
	require.False(t, skip)
	require.Equal(t, []string{"FieldNamed"}, attrNames[:1])

	attrNames, skip = resolveUnmarshalFieldLookupNames(modelType.Field(3), naming.CamelCase)
	require.False(t, skip)
	require.Equal(t, []string{"camelName", "CamelName"}, attrNames[:2])

	attrNames, skip = resolveUnmarshalFieldLookupNames(modelType.Field(4), naming.CamelCase)
	require.False(t, skip)
	require.Equal(t, []string{"json_name", "jsonNamed", "JSONNamed"}, attrNames)

	attrNames, skip = resolveUnmarshalFieldLookupNames(modelType.Field(5), naming.CamelCase)
	require.True(t, skip)
	require.Empty(t, attrNames)

	attrNames, skip = resolveUnmarshalFieldLookupNames(modelType.Field(6), naming.CamelCase)
	require.True(t, skip)
	require.Empty(t, attrNames)
}

func TestResolveStructNaming_CON3(t *testing.T) {
	type explicit struct {
		_    struct{} `theorydb:"naming:legacy_dynamorm"`
		Name string
	}

	type inherited struct {
		Name string
	}

	type defaulted struct {
		Name string `theorydb:"attr:custom_name"`
	}

	convention, isExplicit := explicitNamingConvention(reflect.TypeOf(explicit{}))
	require.True(t, isExplicit)
	require.Equal(t, naming.DynamORM, convention)

	convention = detectNamingConvention(reflect.TypeOf(explicit{}))
	require.Equal(t, naming.DynamORM, convention)

	convention, useTheorydbNaming := resolveStructNaming(reflect.TypeOf(inherited{}), naming.SnakeCase, true)
	require.Equal(t, naming.SnakeCase, convention)
	require.True(t, useTheorydbNaming)

	convention, useTheorydbNaming = resolveStructNaming(reflect.TypeOf(defaulted{}), naming.SnakeCase, false)
	require.Equal(t, naming.CamelCase, convention)
	require.False(t, useTheorydbNaming)
}

func TestUnmarshalItemField_CoversMissingValue_CON3(t *testing.T) {
	type model struct {
		Name string
	}

	var out model
	field := reflect.TypeOf(model{}).Field(0)
	require.NoError(t, unmarshalItemField(map[string]types.AttributeValue{}, field, reflect.ValueOf(&out).Elem().Field(0), naming.CamelCase))
	require.Empty(t, out.Name)
}

func TestUnmarshalMapIntoStructWithConvention_CoversTagAndFallbackBranches_CON3(t *testing.T) {
	type model struct {
		Explicit  string `dynamodb:"explicit_name"`
		FirstName string
		Ignored   string `dynamodb:"-"`
	}

	values := map[string]types.AttributeValue{
		"explicit_name": &types.AttributeValueMemberS{Value: "tagged"},
		"FirstName":     &types.AttributeValueMemberS{Value: "Ada"},
		"Ignored":       &types.AttributeValueMemberS{Value: "skip"},
	}

	var out model
	require.NoError(t, unmarshalMapIntoStructWithConvention(values, reflect.ValueOf(&out).Elem(), naming.CamelCase, true))
	require.Equal(t, "tagged", out.Explicit)
	require.Equal(t, "Ada", out.FirstName)
	require.Empty(t, out.Ignored)
}

func TestUnmarshalItem_DynamORMNestedStruct_JSONTags_CON3(t *testing.T) {
	type address struct {
		PostalCode  string `json:"postal_code"`
		CountryCode string `json:"country_code"`
	}

	type profile struct {
		DisplayName    string  `json:"display_name"`
		MailingAddress address `json:"mailing_address"`
	}

	type model struct {
		_       struct{} `theorydb:"naming:dynamorm"`
		UserID  string   `theorydb:"pk"`
		Entity  string   `theorydb:"sk"`
		Profile profile
	}

	item := map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: "USER#1"},
		"SK": &types.AttributeValueMemberS{Value: "PROFILE"},
		"profile": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
			"display_name": &types.AttributeValueMemberS{Value: "Ada Lovelace"},
			"mailing_address": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
				"postal_code":  &types.AttributeValueMemberS{Value: "10001"},
				"country_code": &types.AttributeValueMemberS{Value: "US"},
			}},
		}},
	}

	var out model
	require.NoError(t, UnmarshalItem(item, &out))
	require.Equal(t, "Ada Lovelace", out.Profile.DisplayName)
	require.Equal(t, "10001", out.Profile.MailingAddress.PostalCode)
	require.Equal(t, "US", out.Profile.MailingAddress.CountryCode)
}

func TestLookupAndJSONHelpers_CON3(t *testing.T) {
	require.Equal(t, "json_name", parseJSONAttributeName("json_name,omitempty"))
	require.Empty(t, parseJSONAttributeName(",omitempty"))

	names := appendLookupName(nil, "first")
	names = appendLookupName(names, "first")
	names = appendLookupName(names, "")
	names = appendLookupName(names, "second")
	require.Equal(t, []string{"first", "second"}, names)

	av, ok := lookupAttributeValue(map[string]types.AttributeValue{
		"second": &types.AttributeValueMemberS{Value: "value"},
	}, "missing", "second")
	require.True(t, ok)
	valueAV, ok := av.(*types.AttributeValueMemberS)
	require.True(t, ok)
	require.Equal(t, "value", valueAV.Value)
}

func queryStringListAttributeValue(values []string) *types.AttributeValueMemberL {
	items := make([]types.AttributeValue, 0, len(values))
	for _, value := range values {
		items = append(items, &types.AttributeValueMemberS{Value: value})
	}
	return &types.AttributeValueMemberL{Value: items}
}

func TestUnmarshalStringToStruct_JSON_CON3(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}

	var out payload
	require.NoError(t, unmarshalStringToStruct(`{"name":"Ada"}`, reflect.ValueOf(&out).Elem()))
	require.Equal(t, "Ada", out.Name)
}

func TestUnmarshalItem_EncryptedEnvelope_FailsClosed_CON3(t *testing.T) {
	type model struct {
		_ struct{} `theorydb:"naming:snake_case"`

		Secret string `theorydb:"encrypted,attr:secret"`
	}

	item := map[string]types.AttributeValue{
		"secret": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
			"v":     &types.AttributeValueMemberN{Value: "1"},
			"edk":   &types.AttributeValueMemberB{Value: []byte("edk")},
			"nonce": &types.AttributeValueMemberB{Value: []byte("nonce")},
			"ct":    &types.AttributeValueMemberB{Value: []byte("ct")},
		}},
	}

	var out model
	err := UnmarshalItem(item, &out)
	require.Error(t, err)
	require.True(t, errors.Is(err, customerrors.ErrEncryptionNotConfigured))
}

func TestUnmarshalItem_EncryptedTag_AllowsNonEnvelopeValue_CON3(t *testing.T) {
	type model struct {
		_ struct{} `theorydb:"naming:snake_case"`

		Secret string `theorydb:"encrypted,attr:secret"`
	}

	item := map[string]types.AttributeValue{
		"secret": &types.AttributeValueMemberS{Value: "plaintext"},
	}

	var out model
	require.NoError(t, UnmarshalItem(item, &out))
	require.Equal(t, "plaintext", out.Secret)
}
