package marshal

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v4/pkg/model"
	pkgTypes "github.com/theory-cloud/tabletheory/v4/pkg/types"
)

//nolint:govet // Field order mirrors the production fixture under test.
type MarshalHiddenPrivileges struct {
	IsAdmin bool
	Secret  string
}

// marshalExcludedItem excludes its promoted subtree with theorydb:"-".
//
//nolint:govet // Field order and the anonymous embed mirror the production fixture under test.
type marshalExcludedItem struct {
	ID                      string `theorydb:"pk" json:"id"`
	MarshalHiddenPrivileges `theorydb:"-"`
	Name                    string `json:"name"`
}

// marshalVisibleItem is the authorized-success control.
//
//nolint:govet
type marshalVisibleItem struct {
	ID string `theorydb:"pk" json:"id"`
	MarshalHiddenPrivileges
	Name string `json:"name"`
}

func TestSafeMarshalerExcludesTaggedAnonymousEmbeds(t *testing.T) {
	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&marshalExcludedItem{}))
	meta, err := registry.GetMetadata(&marshalExcludedItem{})
	require.NoError(t, err)

	m := NewSafeMarshalerWithConverter(pkgTypes.NewConverter().WithFlatAnonymousEmbedEncoding())
	out, err := m.MarshalItem(marshalExcludedItem{
		ID:                      "1",
		MarshalHiddenPrivileges: MarshalHiddenPrivileges{IsAdmin: true, Secret: "s3cr3t"},
		Name:                    "visible",
	}, meta)
	require.NoError(t, err)

	require.Contains(t, out, "id")
	require.Contains(t, out, "name")
	for _, forbidden := range []string{"is_admin", "isAdmin", "IsAdmin", "secret", "Secret", "marshalHiddenPrivileges"} {
		require.NotContains(t, out, forbidden, "excluded embed field %q must not be persisted", forbidden)
	}
}

func TestSafeMarshalerKeepsVisibleAnonymousEmbed(t *testing.T) {
	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&marshalVisibleItem{}))
	meta, err := registry.GetMetadata(&marshalVisibleItem{})
	require.NoError(t, err)

	m := NewSafeMarshalerWithConverter(pkgTypes.NewConverter().WithFlatAnonymousEmbedEncoding())
	out, err := m.MarshalItem(marshalVisibleItem{
		ID:                      "1",
		MarshalHiddenPrivileges: MarshalHiddenPrivileges{IsAdmin: true, Secret: "s3cr3t"},
		Name:                    "visible",
	}, meta)
	require.NoError(t, err)

	require.Contains(t, out, "isAdmin")
	require.True(t, requireAVBOOL(t, out["isAdmin"]).Value)
}
