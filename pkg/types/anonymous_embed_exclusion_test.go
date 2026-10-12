package types

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"
)

//nolint:govet // Field order mirrors the production fixture under test.
type HiddenPrivileges struct {
	IsAdmin bool   `json:"is_admin"`
	Secret  string `json:"secret"`
}

// ItemExcludedByTheorydb excludes the whole promoted subtree with theorydb:"-".
//
//nolint:govet // Field order and the anonymous embed mirror the production fixture under test.
type ItemExcludedByTheorydb struct {
	HiddenPrivileges `theorydb:"-"`
	Name             string `json:"name"`
}

// ItemExcludedByJSON excludes the whole promoted subtree with json:"-".
//
//nolint:govet
type ItemExcludedByJSON struct {
	HiddenPrivileges `json:"-"`
	Name             string `json:"name"`
}

// ItemWithVisibleEmbed is the authorized-success control: an unexcluded embed.
//
//nolint:govet
type ItemWithVisibleEmbed struct {
	HiddenPrivileges
	Name string `json:"name"`
}

func messageMap(t *testing.T, av types.AttributeValue) map[string]types.AttributeValue {
	t.Helper()
	m, ok := av.(*types.AttributeValueMemberM)
	require.Truef(t, ok, "expected map attribute value, got %T", av)
	return m.Value
}

func boolValue(t *testing.T, av types.AttributeValue) bool {
	t.Helper()
	boolean, ok := av.(*types.AttributeValueMemberBOOL)
	require.Truef(t, ok, "expected bool attribute value, got %T", av)
	return boolean.Value
}

func numberText(t *testing.T, av types.AttributeValue) string {
	t.Helper()
	number, ok := av.(*types.AttributeValueMemberN)
	require.Truef(t, ok, "expected number attribute value, got %T", av)
	return number.Value
}

func TestToAttributeValueExcludesTaggedAnonymousEmbeds(t *testing.T) {
	cases := []struct { //nolint:govet // Field alignment is irrelevant for two-field test rows.
		model any
		name  string
	}{
		{name: "theorydb", model: ItemExcludedByTheorydb{HiddenPrivileges: HiddenPrivileges{IsAdmin: true, Secret: "s3cr3t"}, Name: "visible"}},
		{name: "json", model: ItemExcludedByJSON{HiddenPrivileges: HiddenPrivileges{IsAdmin: true, Secret: "s3cr3t"}, Name: "visible"}},
	}

	for _, flat := range []bool{false, true} {
		for _, tc := range cases {
			converter := NewConverter()
			if flat {
				converter = converter.WithFlatAnonymousEmbedEncoding()
			}

			av, err := converter.ToAttributeValue(tc.model)
			require.NoError(t, err, tc.name, flat)
			got := messageMap(t, av)

			require.Len(t, got, 1, "%s/flat=%v: only the unexcluded field may be encoded", tc.name, flat)
			require.Contains(t, got, "Name")
			require.NotContains(t, got, "is_admin")
			require.NotContains(t, got, "isAdmin")
			require.NotContains(t, got, "IsAdmin")
			require.NotContains(t, got, "HiddenPrivileges")
		}
	}
}

func TestToAttributeValueKeepsVisibleAnonymousEmbed(t *testing.T) {
	converter := NewConverter()
	av, err := converter.ToAttributeValue(ItemWithVisibleEmbed{
		HiddenPrivileges: HiddenPrivileges{IsAdmin: true, Secret: "s3cr3t"},
		Name:             "visible",
	})
	require.NoError(t, err)

	got := messageMap(t, av)
	require.Contains(t, got, "Name")
	require.True(t, boolValue(t, messageMap(t, got["hiddenPrivileges"])["IsAdmin"]),
		"unexcluded embed must nest under its container name with promoted fields intact")

	flatConverter := NewConverter().WithFlatAnonymousEmbedEncoding()
	flatAV, err := flatConverter.ToAttributeValue(ItemWithVisibleEmbed{
		HiddenPrivileges: HiddenPrivileges{IsAdmin: true, Secret: "s3cr3t"},
		Name:             "visible",
	})
	require.NoError(t, err)
	flat := messageMap(t, flatAV)
	require.Contains(t, flat, "IsAdmin")
	require.Contains(t, flat, "Secret")
	require.Contains(t, flat, "Name")
	require.NotContains(t, flat, "hiddenPrivileges")
}

func TestFromAttributeValueRejectsPromotedFieldsIntoExcludedEmbed(t *testing.T) {
	item := &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
		"is_admin": &types.AttributeValueMemberBOOL{Value: true},
		"secret":   &types.AttributeValueMemberS{Value: "s3cr3t"},
		"name":     &types.AttributeValueMemberS{Value: "visible"},
	}}

	var theorydbExcluded ItemExcludedByTheorydb
	require.NoError(t, NewConverter().FromAttributeValue(item, &theorydbExcluded))
	require.False(t, theorydbExcluded.IsAdmin, "promoted field must not be mass-assigned through an excluded embed")
	require.Empty(t, theorydbExcluded.Secret)
	require.Equal(t, "visible", theorydbExcluded.Name)

	var jsonExcluded ItemExcludedByJSON
	require.NoError(t, NewConverter().FromAttributeValue(item, &jsonExcluded))
	require.False(t, jsonExcluded.IsAdmin)
	require.Empty(t, jsonExcluded.Secret)
	require.Equal(t, "visible", jsonExcluded.Name)

	// Authorized-success control: an unexcluded embed still decodes.
	var visible ItemWithVisibleEmbed
	require.NoError(t, NewConverter().FromAttributeValue(item, &visible))
	require.True(t, visible.IsAdmin)
	require.Equal(t, "s3cr3t", visible.Secret)
	require.Equal(t, "visible", visible.Name)
}
