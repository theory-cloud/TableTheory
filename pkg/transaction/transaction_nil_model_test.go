package transaction

import (
	"testing"

	"github.com/stretchr/testify/require"

	theorydberrors "github.com/theory-cloud/tabletheory/v4/pkg/errors"
	"github.com/theory-cloud/tabletheory/v4/pkg/model"
	"github.com/theory-cloud/tabletheory/v4/pkg/session"
	pkgTypes "github.com/theory-cloud/tabletheory/v4/pkg/types"
)

type nilModelRecord struct {
	ID string `theorydb:"pk,attr:id" json:"id"`
}

// The explicit query delete path guards a nil model and returns a typed error.
// The legacy transaction surface must do the same instead of panicking inside
// reflect.
func TestTransaction_NilModelReturnsTypedError(t *testing.T) {
	operations := []struct {
		operation func(tx *Transaction) error
		name      string
	}{
		{name: "create", operation: func(tx *Transaction) error { return tx.Create(nil) }},
		{name: "update", operation: func(tx *Transaction) error { return tx.Update(nil) }},
		{name: "delete", operation: func(tx *Transaction) error { return tx.Delete(nil) }},
		{name: "get", operation: func(tx *Transaction) error { return tx.Get(nil, nil) }},
	}

	for _, test := range operations {
		t.Run(test.name, func(t *testing.T) {
			registry := model.NewRegistry()
			require.NoError(t, registry.Register(&nilModelRecord{}))
			tx := NewTransaction(&session.Session{}, registry, pkgTypes.NewConverter())

			var err error
			require.NotPanics(t, func() {
				err = test.operation(tx)
			}, "a nil model must not panic the legacy transaction surface")
			require.Error(t, err)
			require.ErrorIs(t, err, theorydberrors.ErrInvalidModel)
			require.ErrorContains(t, err, "model cannot be nil")
		})
	}
}

func TestTransaction_TypedNilModelReturnsTypedError(t *testing.T) {
	operations := []struct {
		operation func(tx *Transaction) error
		name      string
	}{
		{name: "create", operation: func(tx *Transaction) error { return tx.Create((*nilModelRecord)(nil)) }},
		{name: "update", operation: func(tx *Transaction) error { return tx.Update((*nilModelRecord)(nil)) }},
		{name: "delete", operation: func(tx *Transaction) error { return tx.Delete((*nilModelRecord)(nil)) }},
	}

	for _, test := range operations {
		t.Run(test.name, func(t *testing.T) {
			registry := model.NewRegistry()
			require.NoError(t, registry.Register(&nilModelRecord{}))
			tx := NewTransaction(&session.Session{}, registry, pkgTypes.NewConverter())

			var err error
			require.NotPanics(t, func() {
				err = test.operation(tx)
			}, "a typed nil model must not panic the legacy transaction surface")
			require.Error(t, err)
			require.ErrorContains(t, err, "model cannot be nil")
		})
	}
}
