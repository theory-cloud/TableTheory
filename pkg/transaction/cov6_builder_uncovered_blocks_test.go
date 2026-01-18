package transaction

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/internal/expr"
	"github.com/theory-cloud/tabletheory/pkg/core"
	customerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/model"
	queryPkg "github.com/theory-cloud/tabletheory/pkg/query"
	pkgTypes "github.com/theory-cloud/tabletheory/pkg/types"
)

type cov6BadMarshalItem struct {
	PK  string   `theorydb:"pk"`
	Bad chan int `theorydb:"attr:bad"`
}

func (cov6BadMarshalItem) TableName() string { return "tbl" }

func TestBuilder_addOperation_SkipsWhenErrAlreadySet_COV6(t *testing.T) {
	b := &Builder{err: errors.New("boom")}
	b.addOperation(opPut, &cov5Item{}, nil, nil, nil)
	require.Empty(t, b.operations)
}

func TestBuilder_buildWriteItem_ErrorPaths_COV6(t *testing.T) {
	converter := pkgTypes.NewConverter()
	registry := model.NewRegistry()
	b := &Builder{registry: registry, converter: converter}

	require.NoError(t, registry.Register(&cov6BadMarshalItem{}))
	badMeta, err := registry.GetMetadata(&cov6BadMarshalItem{})
	require.NoError(t, err)

	t.Run("put build errors bubble", func(t *testing.T) {
		_, err := b.buildWriteItem(0, transactOperation{
			typ:      opPut,
			model:    &cov6BadMarshalItem{PK: "p1", Bad: make(chan int)},
			metadata: badMeta,
		})
		require.ErrorContains(t, err, "failed to marshal item for put")
	})

	t.Run("update build errors bubble", func(t *testing.T) {
		meta := cov5ItemMetadata(t)
		_, err := b.buildWriteItem(0, transactOperation{
			typ:      opUpdate,
			model:    &cov5Item{Status: "ok", Version: 1},
			metadata: meta,
			fields:   []string{"Status"},
		})
		require.ErrorContains(t, err, "failed to extract primary key")
	})

	t.Run("update-with-builder build errors bubble", func(t *testing.T) {
		meta := cov5ItemMetadata(t)
		_, err := b.buildWriteItem(0, transactOperation{
			typ:      opUpdateWithBuilder,
			model:    &cov5Item{Status: "ok", Version: 1},
			metadata: meta,
			updateFn: func(core.UpdateBuilder) error { return nil },
		})
		require.ErrorContains(t, err, "partition key")
	})

	t.Run("condition check build errors bubble", func(t *testing.T) {
		meta := cov5ItemMetadata(t)
		_, err := b.buildWriteItem(0, transactOperation{
			typ:      opConditionCheck,
			model:    &cov5Item{PK: "p1", Status: "ok", Version: 1},
			metadata: meta,
		})
		require.ErrorContains(t, err, "condition check requires at least one condition")
	})
}

func TestBuilder_buildPut_ConditionErrorBranches_COV6(t *testing.T) {
	converter := pkgTypes.NewConverter()
	b := &Builder{converter: converter}

	meta := cov5ItemMetadata(t)
	item := &cov5Item{PK: "p1", Status: "ok", Version: 1}

	t.Run("condition field validation bubbles", func(t *testing.T) {
		_, err := b.buildPut(transactOperation{
			model:    item,
			metadata: meta,
			typ:      opPut,
			conditions: []core.TransactCondition{
				{Field: "", Operator: "=", Value: "x"},
			},
		})
		require.ErrorContains(t, err, "condition field cannot be empty")
	})

	t.Run("duplicate placeholders in raw conditions", func(t *testing.T) {
		_, err := b.buildPut(transactOperation{
			model:    item,
			metadata: meta,
			typ:      opPut,
			conditions: []core.TransactCondition{
				{Field: "Status", Operator: "=", Value: "ok"},
				{Kind: core.TransactConditionKindExpression, Expression: "status = :v1", Values: map[string]any{":v1": "dup"}},
			},
		})
		require.ErrorContains(t, err, "duplicate condition value placeholder")
	})
}

func TestBuilder_buildFieldUpdate_ErrorBranches_COV6(t *testing.T) {
	converter := pkgTypes.NewConverter()
	b := &Builder{converter: converter}

	meta := cov5ItemMetadata(t)

	t.Run("extractPrimaryKey wraps errors", func(t *testing.T) {
		_, err := b.buildFieldUpdate(transactOperation{
			typ:      opUpdate,
			model:    &cov5Item{Status: "ok", Version: 1},
			metadata: meta,
			fields:   []string{"Status"},
		})
		require.ErrorContains(t, err, "failed to extract primary key")
	})

	t.Run("applyConditionsToBuilder errors bubble", func(t *testing.T) {
		_, err := b.buildFieldUpdate(transactOperation{
			typ:      opUpdate,
			model:    &cov5Item{PK: "p1", Status: "ok", Version: 1},
			metadata: meta,
			fields:   []string{"Status"},
			conditions: []core.TransactCondition{
				{Field: "Status", Operator: "", Value: "x"},
			},
		})
		require.ErrorContains(t, err, "operator required")
	})

	t.Run("mergeRawConditions catches duplicate placeholders", func(t *testing.T) {
		_, err := b.buildFieldUpdate(transactOperation{
			typ:      opUpdate,
			model:    &cov5Item{PK: "p1", Status: "ok", Version: 1},
			metadata: meta,
			fields:   []string{"Status"},
			conditions: []core.TransactCondition{
				{Kind: core.TransactConditionKindExpression, Expression: "status = :v1", Values: map[string]any{":v1": "dup"}},
			},
		})
		require.ErrorContains(t, err, "duplicate condition value placeholder")
	})
}

func TestBuilder_buildBuilderUpdate_ErrorBranches_COV6(t *testing.T) {
	converter := pkgTypes.NewConverter()
	b := &Builder{converter: converter}

	meta := cov5ItemMetadata(t)

	t.Run("populateKeyConditions validates keys", func(t *testing.T) {
		_, err := b.buildBuilderUpdate(transactOperation{
			model:    &cov5Item{Status: "ok", Version: 1},
			metadata: meta,
			typ:      opUpdateWithBuilder,
			updateFn: func(core.UpdateBuilder) error { return nil },
		}, 0)
		require.ErrorContains(t, err, "partition key")
	})

	t.Run("partitionBuilderConditions bubbles raw condition errors", func(t *testing.T) {
		_, err := b.buildBuilderUpdate(transactOperation{
			model:    &cov5Item{PK: "p1", Status: "ok", Version: 1},
			metadata: meta,
			typ:      opUpdateWithBuilder,
			updateFn: func(ub core.UpdateBuilder) error { ub.Set("Status", "new"); return nil },
			conditions: []core.TransactCondition{
				{Kind: core.TransactConditionKindExpression, Expression: "   "},
			},
		}, 0)
		require.ErrorContains(t, err, "condition expression cannot be empty")
	})

	t.Run("applyConditionsToUpdateBuilder rejects empty operators", func(t *testing.T) {
		_, err := b.buildBuilderUpdate(transactOperation{
			model:    &cov5Item{PK: "p1", Status: "ok", Version: 1},
			metadata: meta,
			typ:      opUpdateWithBuilder,
			updateFn: func(ub core.UpdateBuilder) error { ub.Set("Status", "new"); return nil },
			conditions: []core.TransactCondition{
				{Field: "Status", Operator: "", Value: "x"},
			},
		}, 0)
		require.ErrorContains(t, err, "operator required")
	})

	t.Run("update function errors bubble", func(t *testing.T) {
		_, err := b.buildBuilderUpdate(transactOperation{
			model:    &cov5Item{PK: "p1", Status: "ok", Version: 1},
			metadata: meta,
			typ:      opUpdateWithBuilder,
			updateFn: func(core.UpdateBuilder) error { return errors.New("boom") },
		}, 0)
		require.ErrorContains(t, err, "boom")
	})

	t.Run("unsupported condition operators cause Execute to fail", func(t *testing.T) {
		_, err := b.buildBuilderUpdate(transactOperation{
			model:    &cov5Item{PK: "p1", Status: "ok", Version: 1},
			metadata: meta,
			typ:      opUpdateWithBuilder,
			updateFn: func(ub core.UpdateBuilder) error { ub.Set("Status", "new"); return nil },
			conditions: []core.TransactCondition{
				{Field: "Status", Operator: "NOPE", Value: "x"},
			},
		}, 0)
		require.ErrorContains(t, err, "invalid operator")
	})

	t.Run("empty update expressions are rejected", func(t *testing.T) {
		_, err := b.buildBuilderUpdate(transactOperation{
			model:    &cov5Item{PK: "p1", Status: "ok", Version: 1},
			metadata: meta,
			typ:      opUpdateWithBuilder,
			updateFn: func(core.UpdateBuilder) error { return nil },
		}, 0)
		require.ErrorContains(t, err, "empty update expression")
	})

	t.Run("raw condition placeholder collisions are rejected", func(t *testing.T) {
		_, err := b.buildBuilderUpdate(transactOperation{
			model:    &cov5Item{PK: "p1", Status: "ok", Version: 1},
			metadata: meta,
			typ:      opUpdateWithBuilder,
			updateFn: func(ub core.UpdateBuilder) error { ub.Set("Status", "new"); return nil },
			conditions: []core.TransactCondition{
				{Kind: core.TransactConditionKindExpression, Expression: "status = :v1", Values: map[string]any{":v1": "dup"}},
			},
		}, 0)
		require.ErrorContains(t, err, "duplicate condition value placeholder")
	})
}

func TestBuilder_buildDelete_ConditionBranches_COV6(t *testing.T) {
	converter := pkgTypes.NewConverter()
	b := &Builder{converter: converter}

	meta := cov5ItemMetadata(t)
	item := &cov5Item{PK: "p1", Status: "ok", Version: 1}

	t.Run("sets expressions when present", func(t *testing.T) {
		del, err := b.buildDelete(transactOperation{
			model:    item,
			metadata: meta,
			conditions: []core.TransactCondition{
				{Field: "Status", Operator: "=", Value: "ok"},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, del.ConditionExpression)
		require.NotEmpty(t, aws.ToString(del.ConditionExpression))
		require.NotEmpty(t, del.ExpressionAttributeNames)
		require.NotEmpty(t, del.ExpressionAttributeValues)
	})

	t.Run("bubbles condition build errors", func(t *testing.T) {
		_, err := b.buildDelete(transactOperation{
			model:    item,
			metadata: meta,
			conditions: []core.TransactCondition{
				{Field: "Status", Operator: "", Value: "ok"},
			},
		})
		require.ErrorContains(t, err, "operator required")
	})

	t.Run("detects duplicate placeholders across raw conditions", func(t *testing.T) {
		_, err := b.buildDelete(transactOperation{
			model:    item,
			metadata: meta,
			conditions: []core.TransactCondition{
				{Field: "Status", Operator: "=", Value: "ok"},
				{Kind: core.TransactConditionKindExpression, Expression: "status = :v1", Values: map[string]any{":v1": "dup"}},
			},
		})
		require.ErrorContains(t, err, "duplicate condition value placeholder")
	})
}

func TestBuilder_buildConditionCheck_ErrorBranches_COV6(t *testing.T) {
	converter := pkgTypes.NewConverter()
	b := &Builder{converter: converter}

	meta := cov5ItemMetadata(t)

	t.Run("extractPrimaryKey wraps errors", func(t *testing.T) {
		_, err := b.buildConditionCheck(transactOperation{
			model:    &cov5Item{Status: "ok", Version: 1},
			metadata: meta,
		})
		require.ErrorContains(t, err, "failed to extract primary key")
	})

	t.Run("applyConditionsToBuilder errors bubble", func(t *testing.T) {
		_, err := b.buildConditionCheck(transactOperation{
			model:    &cov5Item{PK: "p1", Status: "ok", Version: 1},
			metadata: meta,
			conditions: []core.TransactCondition{
				{Field: "Status", Operator: "", Value: "x"},
			},
		})
		require.ErrorContains(t, err, "operator required")
	})

	t.Run("mergeRawConditions catches duplicate placeholders", func(t *testing.T) {
		_, err := b.buildConditionCheck(transactOperation{
			model:    &cov5Item{PK: "p1", Status: "ok", Version: 1},
			metadata: meta,
			conditions: []core.TransactCondition{
				{Field: "Status", Operator: "=", Value: "ok"},
				{Kind: core.TransactConditionKindExpression, Expression: "status = :v1", Values: map[string]any{":v1": "dup"}},
			},
		})
		require.ErrorContains(t, err, "duplicate condition value placeholder")
	})
}

func TestBuilder_applyConditionsToBuilder_AndHelpers_ErrorBranches_COV6(t *testing.T) {
	converter := pkgTypes.NewConverter()
	b := &Builder{converter: converter}

	t.Run("field conditions validate operator and supported operators", func(t *testing.T) {
		meta := cov5ItemMetadata(t)
		builder := expr.NewBuilderWithConverter(converter)

		_, err := b.applyConditionsToBuilder(meta, builder, []core.TransactCondition{
			{Field: "Status", Operator: "", Value: "x"},
		})
		require.ErrorContains(t, err, "operator required")

		_, err = b.applyConditionsToBuilder(meta, expr.NewBuilderWithConverter(converter), []core.TransactCondition{
			{Field: "Status", Operator: "NOPE", Value: "x"},
		})
		require.ErrorContains(t, err, "invalid operator")
	})

	t.Run("version conditions require a version field", func(t *testing.T) {
		meta := &model.Metadata{Fields: make(map[string]*model.FieldMetadata)}
		builder := expr.NewBuilderWithConverter(converter)
		_, err := b.applyConditionsToBuilder(meta, builder, []core.TransactCondition{
			{Kind: core.TransactConditionKindVersionEquals, Value: int64(1)},
		})
		require.ErrorContains(t, err, "does not define a version field")
	})

	t.Run("expression conditions validate and bubble errors", func(t *testing.T) {
		meta := cov5ItemMetadata(t)
		builder := expr.NewBuilderWithConverter(converter)
		_, err := b.applyConditionsToBuilder(meta, builder, []core.TransactCondition{
			{Kind: core.TransactConditionKindExpression, Expression: "   "},
		})
		require.ErrorContains(t, err, "condition expression cannot be empty")
	})

	t.Run("unsupported condition kinds return errors", func(t *testing.T) {
		meta := cov5ItemMetadata(t)
		builder := expr.NewBuilderWithConverter(converter)
		_, err := b.applyConditionsToBuilder(meta, builder, []core.TransactCondition{
			{Kind: "nope"},
		})
		require.ErrorContains(t, err, "unsupported transaction condition type")
	})
}

func TestBuilder_resolveAttributeName_UsesMetadataMaps_COV6(t *testing.T) {
	b := &Builder{converter: pkgTypes.NewConverter()}
	meta := cov5ItemMetadata(t)

	got, err := b.resolveAttributeName(meta, "Status")
	require.NoError(t, err)
	require.Equal(t, "status", got)

	got, err = b.resolveAttributeName(meta, "status")
	require.NoError(t, err)
	require.Equal(t, "status", got)
}

func TestBuilder_addPrimaryKeyCondition_CoversSortKeyBranch_COV6(t *testing.T) {
	converter := pkgTypes.NewConverter()
	b := &Builder{converter: converter}

	builder := expr.NewBuilderWithConverter(converter)
	require.NoError(t, b.addPrimaryKeyCondition(cov5CompositeItemMetadata(t), builder, "attribute_exists"))
	components := builder.Build()
	require.GreaterOrEqual(t, len(components.ExpressionAttributeNames), 2)
	require.GreaterOrEqual(t, len(components.ExpressionAttributeValues), 0)
	require.Contains(t, components.ConditionExpression, "attribute_exists")
	require.GreaterOrEqual(t, len(components.ConditionExpression), len("attribute_exists"))
}

func TestBuilder_mergeRawConditions_MergesWithoutDuplicates_COV6(t *testing.T) {
	b := &Builder{converter: pkgTypes.NewConverter()}

	exprStr, names, values, err := b.mergeRawConditions("", nil, nil, []rawCondition{
		{expression: "attribute_exists(pk)", values: map[string]types.AttributeValue{
			":v": &types.AttributeValueMemberS{Value: "x"},
		}},
	})
	require.NoError(t, err)
	require.Equal(t, "attribute_exists(pk)", exprStr)
	require.NotNil(t, names)
	require.Contains(t, values, ":v")
}

func TestBuilder_partitionBuilderConditions_EmptyAndErrorBranches_COV6(t *testing.T) {
	b := &Builder{converter: pkgTypes.NewConverter()}

	builderConds, rawConds, err := b.partitionBuilderConditions(nil, nil)
	require.NoError(t, err)
	require.Nil(t, builderConds)
	require.Nil(t, rawConds)

	_, _, err = b.partitionBuilderConditions(nil, []core.TransactCondition{
		{Kind: core.TransactConditionKindExpression, Expression: "   "},
	})
	require.ErrorContains(t, err, "condition expression cannot be empty")
}

func TestBuilder_applyConditionsToUpdateBuilder_ErrorBranches_COV6(t *testing.T) {
	b := &Builder{converter: pkgTypes.NewConverter()}
	meta := cov5CompositeItemMetadata(t)

	q := queryPkg.New(&cov5CompositeItem{PK: "p1", SK: "s1"}, adaptMetadata(meta), noopQueryExecutor{})
	ubAny := queryPkg.NewUpdateBuilder(q)
	ub, ok := ubAny.(*queryPkg.UpdateBuilder)
	require.True(t, ok)

	require.ErrorContains(t, b.applyConditionsToUpdateBuilder(ub, meta, []core.TransactCondition{
		{Field: "", Operator: "=", Value: "x"},
	}), "condition field cannot be empty")

	require.ErrorContains(t, b.applyConditionsToUpdateBuilder(ub, meta, []core.TransactCondition{
		{Field: "Status", Operator: "", Value: "x"},
	}), "operator required")

	require.ErrorContains(t, b.applyConditionsToUpdateBuilder(ub, nil, []core.TransactCondition{
		{Kind: core.TransactConditionKindVersionEquals, Value: int64(1)},
	}), "model metadata is required")

	require.ErrorContains(t, b.applyConditionsToUpdateBuilder(ub, meta, []core.TransactCondition{
		{Kind: "nope"},
	}), "unsupported condition type")
}

func TestBuilder_executeWithRetry_AndErrorTranslation_COV6(t *testing.T) {
	t.Run("requires session when no client", func(t *testing.T) {
		b := &Builder{}
		err := b.executeWithRetry(context.Background(), &dynamodb.TransactWriteItemsInput{})
		require.ErrorContains(t, err, "session is not configured")
	})

	t.Run("context cancellation aborts retries", func(t *testing.T) {
		cancel := &types.TransactionCanceledException{
			CancellationReasons: []types.CancellationReason{
				{Code: aws.String("TransactionConflict")},
			},
		}
		builder := &Builder{client: newMockTransactClient(t, cancel)}

		ctx, cancelFn := context.WithCancel(context.Background())
		cancelFn()

		err := builder.executeWithRetry(ctx, &dynamodb.TransactWriteItemsInput{})
		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("non-canceled errors are not retryable", func(t *testing.T) {
		builder := &Builder{}
		retryable, translated := builder.translateError(errors.New("boom"))
		require.False(t, retryable)
		require.ErrorContains(t, translated, "boom")
	})

	t.Run("buildTransactionError formats empty/no-code reasons", func(t *testing.T) {
		builder := &Builder{}
		base := errors.New("boom")

		err := builder.buildTransactionError(&types.TransactionCanceledException{}, base)
		require.ErrorContains(t, err, "transaction canceled")

		err = builder.buildTransactionError(&types.TransactionCanceledException{
			CancellationReasons: []types.CancellationReason{{Message: aws.String("ignored")}},
		}, base)
		require.ErrorContains(t, err, "transaction canceled")
	})

	t.Run("translateError marks non-retryable reasons", func(t *testing.T) {
		cancel := &types.TransactionCanceledException{
			CancellationReasons: []types.CancellationReason{
				{Code: aws.String("TransactionConflict")},
				{Code: aws.String("ConditionalCheckFailed")},
			},
		}
		builder := &Builder{}
		retryable, err := builder.translateError(cancel)
		require.False(t, retryable)
		require.ErrorIs(t, err, customerrors.ErrTransactionFailed)
	})
}
