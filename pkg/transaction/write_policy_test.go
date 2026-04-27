package transaction

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/core"
	theorydbErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/model"
	"github.com/theory-cloud/tabletheory/pkg/session"
	pkgTypes "github.com/theory-cloud/tabletheory/pkg/types"
)

type writePolicyTransactionActual struct {
	PK              string `theorydb:"pk"`
	SK              string `theorydb:"sk"`
	Status          string
	PinnedReleaseID string `theorydb:"attr:pinnedReleaseId,omitempty"`
}

func (writePolicyTransactionActual) WritePolicy() model.WritePolicy {
	return model.WritePolicy{
		Mode:                model.WritePolicyModeMutable,
		ProtectedAttributes: []string{"pinnedReleaseId"},
	}
}

type writePolicyTransactionEvent struct {
	PK        string `theorydb:"pk"`
	SK        string `theorydb:"sk"`
	EventType string `theorydb:"attr:eventType"`
}

func (writePolicyTransactionEvent) WritePolicy() model.WritePolicy {
	return model.WritePolicy{Mode: model.WritePolicyModeWriteOnce}
}

func newWritePolicyTransactionBuilder(t *testing.T) (*Builder, *model.Registry) {
	t.Helper()

	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&writePolicyTransactionActual{}))
	require.NoError(t, registry.Register(&writePolicyTransactionEvent{}))

	return NewBuilder(&session.Session{}, registry, pkgTypes.NewConverter()), registry
}

func TestWritePolicyTransactionBuilder_WriteOnceRules(t *testing.T) {
	event := &writePolicyTransactionEvent{PK: "release#svc", SK: "event#1", EventType: "promoted"}

	t.Run("create allowed", func(t *testing.T) {
		builder, _ := newWritePolicyTransactionBuilder(t)
		builder.Create(event)
		items, err := builder.materializeOperations()
		require.NoError(t, err)
		require.Len(t, items, 1)
		require.NotNil(t, items[0].Put)
		require.NotNil(t, items[0].Put.ConditionExpression)
	})

	for _, tt := range []struct {
		run  func(*Builder)
		name string
	}{
		{run: func(b *Builder) { b.Put(event) }, name: "put"},
		{run: func(b *Builder) { b.Update(event, []string{"EventType"}) }, name: "update"},
		{run: func(b *Builder) {
			b.UpdateWithBuilder(event, func(ub core.UpdateBuilder) error {
				ub.Set("eventType", "mutated")
				return nil
			})
		}, name: "update_builder"},
		{run: func(b *Builder) { b.Delete(event) }, name: "delete"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			builder, _ := newWritePolicyTransactionBuilder(t)
			tt.run(builder)
			err := builder.Execute()
			require.Error(t, err)
			require.True(t, errors.Is(err, theorydbErrors.ErrImmutableModelMutation))
		})
	}
}

func TestWritePolicyTransactionBuilder_ProtectedFieldRules(t *testing.T) {
	actual := &writePolicyTransactionActual{
		PK:              "release#svc",
		SK:              "actual",
		Status:          "active",
		PinnedReleaseID: "rel-1",
	}

	t.Run("create allowed", func(t *testing.T) {
		builder, _ := newWritePolicyTransactionBuilder(t)
		builder.Create(actual)
		items, err := builder.materializeOperations()
		require.NoError(t, err)
		require.Len(t, items, 1)
		require.NotNil(t, items[0].Put)
		require.NotNil(t, items[0].Put.ConditionExpression)
	})

	t.Run("put rejects protected overwrite", func(t *testing.T) {
		builder, _ := newWritePolicyTransactionBuilder(t)
		builder.Put(actual)
		err := builder.Execute()
		require.Error(t, err)
		require.True(t, errors.Is(err, theorydbErrors.ErrProtectedFieldMutation))
	})

	t.Run("field update rejects protected attr", func(t *testing.T) {
		builder, _ := newWritePolicyTransactionBuilder(t)
		builder.Update(actual, []string{"pinnedReleaseId"})
		err := builder.Execute()
		require.Error(t, err)
		require.True(t, errors.Is(err, theorydbErrors.ErrProtectedFieldMutation))
	})

	t.Run("update builder rejects protected attr", func(t *testing.T) {
		builder, _ := newWritePolicyTransactionBuilder(t)
		builder.UpdateWithBuilder(actual, func(ub core.UpdateBuilder) error {
			ub.Set("pinnedReleaseId", "rel-2")
			return nil
		})
		_, err := builder.materializeOperations()
		require.Error(t, err)
		require.True(t, errors.Is(err, theorydbErrors.ErrProtectedFieldMutation))
	})
}

func TestWritePolicyTransaction_EnforcesPolicies(t *testing.T) {
	_, registry := newWritePolicyTransactionBuilder(t)
	tx := NewTransaction(&session.Session{}, registry, pkgTypes.NewConverter())

	event := &writePolicyTransactionEvent{PK: "release#svc", SK: "event#1", EventType: "promoted"}
	err := tx.Update(event)
	require.Error(t, err)
	require.True(t, errors.Is(err, theorydbErrors.ErrImmutableModelMutation))

	err = tx.Delete(event)
	require.Error(t, err)
	require.True(t, errors.Is(err, theorydbErrors.ErrImmutableModelMutation))

	actual := &writePolicyTransactionActual{
		PK:              "release#svc",
		SK:              "actual",
		Status:          "active",
		PinnedReleaseID: "rel-1",
	}
	err = tx.Update(actual)
	require.Error(t, err)
	require.True(t, errors.Is(err, theorydbErrors.ErrProtectedFieldMutation))
}
