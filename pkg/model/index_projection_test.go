package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type indexProjectionValueModel struct {
	PK        string `theorydb:"pk"`
	Title     string `theorydb:"index:gsi-title,pk"`
	CreatedAt string `theorydb:"index:gsi-title,sk"`
}

func (indexProjectionValueModel) TableTheoryIndexProjections() map[string]struct {
	Type   string
	Fields []string
} {
	return map[string]struct {
		Type   string
		Fields []string
	}{
		"gsi-title": {Type: "INCLUDE", Fields: []string{"payload", "count"}},
	}
}

type indexProjectionPointerModel struct {
	PK    string `theorydb:"pk"`
	Email string `theorydb:"index:gsi-email,pk"`
}

func (*indexProjectionPointerModel) TableTheoryIndexProjections() map[string]struct {
	Type   string
	Fields []string
} {
	return map[string]struct {
		Type   string
		Fields []string
	}{
		"gsi-email": {Type: "KEYS_ONLY"},
	}
}

func TestIndexProjectionOverridesFromValueModel(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	require.NoError(t, registry.Register(indexProjectionValueModel{}))
	metadata, err := registry.GetMetadata(indexProjectionValueModel{})
	require.NoError(t, err)
	require.Len(t, metadata.Indexes, 1)
	require.Equal(t, "gsi-title", metadata.Indexes[0].Name)
	require.Equal(t, "INCLUDE", metadata.Indexes[0].ProjectionType)
	require.Equal(t, []string{"count", "payload"}, metadata.Indexes[0].ProjectedFields)
}

func TestIndexProjectionOverridesFromPointerModel(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	require.NoError(t, registry.Register(indexProjectionPointerModel{}))
	metadata, err := registry.GetMetadata(indexProjectionPointerModel{})
	require.NoError(t, err)
	require.Len(t, metadata.Indexes, 1)
	require.Equal(t, "gsi-email", metadata.Indexes[0].Name)
	require.Equal(t, "KEYS_ONLY", metadata.Indexes[0].ProjectionType)
	require.Empty(t, metadata.Indexes[0].ProjectedFields)
}
