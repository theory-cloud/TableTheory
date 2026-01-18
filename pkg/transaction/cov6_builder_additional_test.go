package transaction

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/model"
	queryPkg "github.com/theory-cloud/tabletheory/pkg/query"
	pkgTypes "github.com/theory-cloud/tabletheory/pkg/types"
)

func TestBuilder_ValidationErrorsAndExecuteGuards_COV6(t *testing.T) {
	t.Run("Update requires fields", func(t *testing.T) {
		b := &Builder{}
		b.Update(&cov5Item{}, nil)
		require.Error(t, b.err)
	})

	t.Run("UpdateWithBuilder requires function", func(t *testing.T) {
		b := &Builder{}
		b.UpdateWithBuilder(&cov5Item{}, nil)
		require.Error(t, b.err)
	})

	t.Run("ConditionCheck requires conditions", func(t *testing.T) {
		b := &Builder{}
		b.ConditionCheck(&cov5Item{})
		require.Error(t, b.err)
	})

	t.Run("ExecuteWithContext requires operations", func(t *testing.T) {
		b := &Builder{}
		require.ErrorContains(t, b.ExecuteWithContext(nil), "transaction has no operations")
	})

	t.Run("WithContext nil uses Background", func(t *testing.T) {
		b := &Builder{}
		require.Same(t, b, b.WithContext(nil))
	})
}

func TestBuilder_BuildWriteItemAndOperations_Errors_COV6(t *testing.T) {
	converter := pkgTypes.NewConverter()
	b := &Builder{converter: converter}

	t.Run("unsupported operation type", func(t *testing.T) {
		_, err := b.buildWriteItem(0, transactOperation{typ: operationType(999)})
		require.Error(t, err)
	})

	t.Run("build field update unknown field", func(t *testing.T) {
		meta := cov5ItemMetadata(t)
		item := &cov5Item{PK: "p1", Status: "ok", Version: 1}
		_, err := b.buildFieldUpdate(transactOperation{
			typ:      opUpdate,
			model:    item,
			metadata: meta,
			fields:   []string{"Missing"},
		})
		require.ErrorContains(t, err, "unknown field Missing")
	})

	t.Run("build field update empty expression", func(t *testing.T) {
		meta := cov5ItemMetadata(t)
		item := &cov5Item{PK: "p1", Status: "ok", Version: 1}
		_, err := b.buildFieldUpdate(transactOperation{
			typ:      opUpdate,
			model:    item,
			metadata: meta,
			fields:   nil,
		})
		require.ErrorContains(t, err, "update expression cannot be empty")
	})

	t.Run("build condition check requires expression", func(t *testing.T) {
		meta := cov5ItemMetadata(t)
		item := &cov5Item{PK: "p1", Status: "ok", Version: 1}
		_, err := b.buildConditionCheck(transactOperation{
			typ:        opConditionCheck,
			model:      item,
			metadata:   meta,
			conditions: nil,
		})
		require.ErrorContains(t, err, "condition check requires at least one condition")
	})
}

func TestBuilder_populateKeyConditions_ValidatesPrimaryKey_COV6(t *testing.T) {
	converter := pkgTypes.NewConverter()
	b := &Builder{converter: converter}

	t.Run("missing metadata", func(t *testing.T) {
		q := queryPkg.New(&cov5Item{}, nil, noopQueryExecutor{})
		require.Error(t, b.populateKeyConditions(q, &model.Metadata{}, &cov5Item{}))
	})

	t.Run("missing partition key value", func(t *testing.T) {
		meta := cov5ItemMetadata(t)
		q := queryPkg.New(&cov5Item{}, adaptMetadata(meta), noopQueryExecutor{})
		require.ErrorContains(t, b.populateKeyConditions(q, meta, &cov5Item{}), "partition key")
	})

	t.Run("missing sort key value", func(t *testing.T) {
		meta := cov5CompositeItemMetadata(t)
		item := &cov5CompositeItem{PK: "p1"}
		q := queryPkg.New(item, adaptMetadata(meta), noopQueryExecutor{})
		require.ErrorContains(t, b.populateKeyConditions(q, meta, item), "sort key")
	})

	t.Run("success composite key", func(t *testing.T) {
		meta := cov5CompositeItemMetadata(t)
		item := &cov5CompositeItem{PK: "p1", SK: "s1"}
		q := queryPkg.New(item, adaptMetadata(meta), noopQueryExecutor{}).WithContext(context.Background()).(*queryPkg.Query)
		require.NoError(t, b.populateKeyConditions(q, meta, item))
	})
}
