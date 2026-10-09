package model

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v4/pkg/errors"
)

type nilGuardRecord struct {
	ID string `theorydb:"pk,attr:id" json:"id"`
}

// A nil model has no reflect.Type, and reflect.TypeOf(nil).Kind() panics. The
// registry must report a typed error instead so the legacy transaction surface
// cannot panic on a nil model.
func TestRegistry_GetMetadataNilModelReturnsTypedError(t *testing.T) {
	registry := NewRegistry()
	require.NoError(t, registry.Register(&nilGuardRecord{}))

	var metadata *Metadata
	var err error
	require.NotPanics(t, func() {
		metadata, err = registry.GetMetadata(nil)
	})
	require.Error(t, err)
	require.Nil(t, metadata)
	require.ErrorIs(t, err, errors.ErrInvalidModel)
	require.ErrorContains(t, err, "model cannot be nil")
}

func TestRegistry_RegisterNilModelReturnsTypedError(t *testing.T) {
	registry := NewRegistry()

	var err error
	require.NotPanics(t, func() {
		err = registry.Register(nil)
	})
	require.Error(t, err)
	require.ErrorIs(t, err, errors.ErrInvalidModel)
	require.ErrorContains(t, err, "model cannot be nil")
}
