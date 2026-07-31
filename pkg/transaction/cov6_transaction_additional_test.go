package transaction

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v3/pkg/model"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
	pkgTypes "github.com/theory-cloud/tabletheory/v3/pkg/types"
)

func TestTransaction_Create_ReturnsMetadataError_COV6(t *testing.T) {
	registry := model.NewRegistry()
	tx := NewTransaction(&session.Session{}, registry, pkgTypes.NewConverter())

	err := tx.Create(&unitUser{ID: "u1"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to get model metadata")
}

func TestTransaction_Update_ReturnsPrimaryKeyError_COV6(t *testing.T) {
	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&unitUser{}))

	tx := NewTransaction(&session.Session{}, registry, pkgTypes.NewConverter())

	err := tx.Update(&unitUser{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to extract primary key")
}

func TestTransaction_Commit_NoOps_ReturnsNil_COV6(t *testing.T) {
	tx := NewTransaction(&session.Session{}, model.NewRegistry(), pkgTypes.NewConverter())
	require.NoError(t, tx.Commit())
}

func TestTransaction_Commit_FailsWhenClientMissing_COV6(t *testing.T) {
	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&unitUser{}))

	tx := NewTransaction(&session.Session{}, registry, pkgTypes.NewConverter())

	require.NoError(t, tx.Create(&unitUser{ID: "u1", Email: "e"}))
	require.Error(t, tx.Commit())

	tx = NewTransaction(&session.Session{}, registry, pkgTypes.NewConverter())
	require.NoError(t, tx.Get(&unitUser{ID: "u1"}, &unitUser{}))
	require.Error(t, tx.Commit())
}

func TestTransaction_Update_OverridesUpdatedAtWhenAlreadySet_COV6(t *testing.T) {
	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&unitUser{}))

	tx := NewTransaction(&session.Session{}, registry, pkgTypes.NewConverter())
	user := &unitUser{
		ID:        "u1",
		Email:     "e",
		Version:   1,
		UpdatedAt: time.Now(),
	}

	// Legacy whole-model updates are an implicit RMW surface: loaded lifecycle values
	// are skipped, while the library-owned updated_at assignment is appended.
	require.NoError(t, tx.Update(user))
	require.Len(t, tx.writes, 1)
	require.NotNil(t, tx.writes[0].Update)
	require.Equal(t, 1, updatePathOccurrences(
		*tx.writes[0].Update.UpdateExpression,
		tx.writes[0].Update.ExpressionAttributeNames,
		"updatedAt",
	))
}
