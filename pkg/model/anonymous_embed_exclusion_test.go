package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type ModelHiddenPrivileges struct {
	IsAdmin bool `json:"is_admin"`
}

//nolint:govet // Field order and the anonymous embed mirror the production fixture under test.
type modelExcludedEmbed struct {
	PK                    string `theorydb:"pk" json:"pk"`
	ModelHiddenPrivileges `theorydb:"-"`
	Name                  string `json:"name"`
}

//nolint:govet
type modelExcludedEmbedJSON struct {
	PK                    string `theorydb:"pk" json:"pk"`
	ModelHiddenPrivileges `json:"-"`
	Name                  string `json:"name"`
}

//nolint:govet
type modelVisibleEmbed struct {
	PK string `theorydb:"pk" json:"pk"`
	ModelHiddenPrivileges
	Name string `json:"name"`
}

func TestRegistryExcludesTaggedAnonymousEmbeds(t *testing.T) {
	for _, tc := range []struct { //nolint:govet // Field alignment is irrelevant for two-field test rows.
		model any
		name  string
	}{
		{&modelExcludedEmbed{}, "theorydb"},
		{&modelExcludedEmbedJSON{}, "json"},
	} {
		registry := NewRegistry()
		require.NoError(t, registry.Register(tc.model))
		meta, err := registry.GetMetadata(tc.model)
		require.NoError(t, err)

		require.Nil(t, meta.Fields["IsAdmin"], "%s: excluded promoted field must not be registered", tc.name)
		require.Nil(t, meta.FieldsByDBName["isAdmin"], tc.name)
		require.NotNil(t, meta.Fields["Name"], tc.name)
	}

	registry := NewRegistry()
	require.NoError(t, registry.Register(&modelVisibleEmbed{}))
	meta, err := registry.GetMetadata(&modelVisibleEmbed{})
	require.NoError(t, err)
	require.NotNil(t, meta.Fields["IsAdmin"], "an unexcluded embed must still register its promoted fields")
}
