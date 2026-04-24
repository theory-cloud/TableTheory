package model

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	theorydbErrors "github.com/theory-cloud/tabletheory/pkg/errors"
)

type writePolicyDefaultModel struct {
	PK    string `theorydb:"pk"`
	SK    string `theorydb:"sk"`
	Value string
}

type writePolicyValueReceiverModel struct {
	PK              string `theorydb:"pk"`
	SK              string `theorydb:"sk"`
	PinnedReleaseID string `theorydb:"attr:pinnedReleaseId"`
	Status          string
}

func (writePolicyValueReceiverModel) WritePolicy() WritePolicy {
	return WritePolicy{
		Mode: WritePolicyModeMutable,
		ProtectedAttributes: []string{
			"Status",
			"pinnedReleaseId",
			"PinnedReleaseID",
		},
	}
}

type writePolicyPointerReceiverModel struct {
	PK    string `theorydb:"pk"`
	SK    string `theorydb:"sk"`
	Event string
}

func (*writePolicyPointerReceiverModel) WritePolicy() WritePolicy {
	return WritePolicy{Mode: WritePolicyModeWriteOnce}
}

type writePolicyInvalidModeModel struct {
	PK string `theorydb:"pk"`
}

func (writePolicyInvalidModeModel) WritePolicy() WritePolicy {
	return WritePolicy{Mode: WritePolicyMode("frozen")}
}

type writePolicyMissingProtectedModel struct {
	PK string `theorydb:"pk"`
}

func (writePolicyMissingProtectedModel) WritePolicy() WritePolicy {
	return WritePolicy{ProtectedAttributes: []string{"missing"}}
}

type writePolicyEmptyProtectedModel struct {
	PK string `theorydb:"pk"`
}

func (writePolicyEmptyProtectedModel) WritePolicy() WritePolicy {
	return WritePolicy{ProtectedAttributes: []string{" "}}
}

func TestWritePolicyMetadata_DefaultsMutable(t *testing.T) {
	registry := NewRegistry()
	require.NoError(t, registry.Register(&writePolicyDefaultModel{}))

	metadata, err := registry.GetMetadata(&writePolicyDefaultModel{})
	require.NoError(t, err)
	require.Equal(t, WritePolicy{
		Mode:                WritePolicyModeMutable,
		ProtectedAttributes: []string{},
	}, metadata.WritePolicy)
}

func TestWritePolicyMetadata_ResolvesProtectedAttributesToCanonicalDBNames(t *testing.T) {
	registry := NewRegistry()
	require.NoError(t, registry.Register(&writePolicyValueReceiverModel{}))

	metadata, err := registry.GetMetadata(&writePolicyValueReceiverModel{})
	require.NoError(t, err)
	require.Equal(t, WritePolicyModeMutable, metadata.WritePolicy.Mode)
	require.Equal(t, []string{"pinnedReleaseId", "status"}, metadata.WritePolicy.ProtectedAttributes)
}

func TestWritePolicyMetadata_UsesPointerReceiver(t *testing.T) {
	registry := NewRegistry()
	require.NoError(t, registry.Register(&writePolicyPointerReceiverModel{}))

	metadata, err := registry.GetMetadata(&writePolicyPointerReceiverModel{})
	require.NoError(t, err)
	require.Equal(t, WritePolicyModeWriteOnce, metadata.WritePolicy.Mode)
	require.Equal(t, []string{}, metadata.WritePolicy.ProtectedAttributes)
}

func TestWritePolicyMetadata_RejectsInvalidMode(t *testing.T) {
	registry := NewRegistry()
	err := registry.Register(&writePolicyInvalidModeModel{})
	require.Error(t, err)
	require.True(t, errors.Is(err, theorydbErrors.ErrInvalidModel))
	require.Contains(t, err.Error(), "unsupported write_policy.mode")
}

func TestWritePolicyMetadata_RejectsMissingProtectedAttribute(t *testing.T) {
	registry := NewRegistry()
	err := registry.Register(&writePolicyMissingProtectedModel{})
	require.Error(t, err)
	require.True(t, errors.Is(err, theorydbErrors.ErrInvalidModel))
	require.Contains(t, err.Error(), "write_policy protected attribute not found")
}

func TestWritePolicyMetadata_RejectsEmptyProtectedAttribute(t *testing.T) {
	registry := NewRegistry()
	err := registry.Register(&writePolicyEmptyProtectedModel{})
	require.Error(t, err)
	require.True(t, errors.Is(err, theorydbErrors.ErrInvalidModel))
	require.Contains(t, err.Error(), "write_policy protected attributes must be non-empty")
}
