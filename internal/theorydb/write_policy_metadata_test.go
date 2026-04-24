package theorydb

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/model"
)

type writePolicyAdapterModel struct {
	PK              string `theorydb:"pk"`
	PinnedReleaseID string `theorydb:"attr:pinnedReleaseId"`
}

func (writePolicyAdapterModel) WritePolicy() model.WritePolicy {
	return model.WritePolicy{
		Mode:                model.WritePolicyModeMutable,
		ProtectedAttributes: []string{"PinnedReleaseID"},
	}
}

func TestWritePolicyMetadataAdapter(t *testing.T) {
	db := newBareDB()
	require.NoError(t, db.registry.Register(&writePolicyAdapterModel{}))

	metadata, err := db.registry.GetMetadata(&writePolicyAdapterModel{})
	require.NoError(t, err)

	adapter := &metadataAdapter{metadata: metadata}
	require.Equal(t, model.WritePolicy{
		Mode:                model.WritePolicyModeMutable,
		ProtectedAttributes: []string{"pinnedReleaseId"},
	}, adapter.WritePolicy())
}

func TestWritePolicyMetadataAdapterNilDefaultsMutable(t *testing.T) {
	adapter := &metadataAdapter{}
	require.Equal(t, model.DefaultWritePolicy(), adapter.WritePolicy())
}
