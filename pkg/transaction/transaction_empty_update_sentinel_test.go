package transaction

import (
	"testing"

	"github.com/stretchr/testify/require"

	theorydberrors "github.com/theory-cloud/tabletheory/v4/pkg/errors"
	"github.com/theory-cloud/tabletheory/v4/pkg/model"
	"github.com/theory-cloud/tabletheory/v4/pkg/session"
	pkgTypes "github.com/theory-cloud/tabletheory/v4/pkg/types"
)

// The empty legacy update must be matchable with errors.Is while its message
// stays exactly as documented for existing callers.
func TestTransaction_EmptyUpdateMatchesSentinel(t *testing.T) {
	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&unitCreatedAtOnly{}))

	tx := NewTransaction(&session.Session{}, registry, pkgTypes.NewConverter())

	err := tx.Update(&unitCreatedAtOnly{ID: "empty-update"})
	require.ErrorIs(t, err, theorydberrors.ErrNoUpdatableFields)
	require.EqualError(t, err, "no non-key fields to update")

	// A failed update poisons the transaction, and Commit must report the same
	// matchable error.
	require.ErrorIs(t, tx.Commit(), theorydberrors.ErrNoUpdatableFields)
}
