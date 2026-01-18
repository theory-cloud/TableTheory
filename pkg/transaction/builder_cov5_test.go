package transaction

import (
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/internal/expr"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/model"
	pkgTypes "github.com/theory-cloud/tabletheory/pkg/types"
)

type cov5Item struct {
	PK      string
	Status  string
	Version int64
}

type cov5CompositeItem struct {
	PK string
	SK string
}

func cov5ItemMetadata(t *testing.T) *model.Metadata {
	t.Helper()

	typ := reflect.TypeOf(cov5Item{})
	pkField, ok := typ.FieldByName("PK")
	require.True(t, ok)
	statusField, ok := typ.FieldByName("Status")
	require.True(t, ok)
	versionField, ok := typ.FieldByName("Version")
	require.True(t, ok)

	pkMeta := &model.FieldMetadata{
		Name:      "PK",
		DBName:    "pk",
		Type:      reflect.TypeOf(""),
		Index:     pkField.Index[0],
		IndexPath: pkField.Index,
	}
	statusMeta := &model.FieldMetadata{
		Name:      "Status",
		DBName:    "status",
		Type:      reflect.TypeOf(""),
		Index:     statusField.Index[0],
		IndexPath: statusField.Index,
	}
	versionMeta := &model.FieldMetadata{
		Name:      "Version",
		DBName:    "ver",
		Type:      reflect.TypeOf(int64(0)),
		Index:     versionField.Index[0],
		IndexPath: versionField.Index,
	}

	meta := &model.Metadata{
		TableName:      "tbl",
		Fields:         make(map[string]*model.FieldMetadata),
		FieldsByDBName: make(map[string]*model.FieldMetadata),
		PrimaryKey: &model.KeySchema{
			PartitionKey: pkMeta,
		},
		VersionField: versionMeta,
	}

	meta.Fields["PK"] = pkMeta
	meta.FieldsByDBName["pk"] = pkMeta
	meta.Fields["Status"] = statusMeta
	meta.FieldsByDBName["status"] = statusMeta
	meta.Fields["Version"] = versionMeta
	meta.FieldsByDBName["ver"] = versionMeta

	return meta
}

func cov5CompositeItemMetadata(t *testing.T) *model.Metadata {
	t.Helper()

	typ := reflect.TypeOf(cov5CompositeItem{})
	pkField, ok := typ.FieldByName("PK")
	require.True(t, ok)
	skField, ok := typ.FieldByName("SK")
	require.True(t, ok)

	pkMeta := &model.FieldMetadata{
		Name:      "PK",
		DBName:    "pk",
		Type:      reflect.TypeOf(""),
		Index:     pkField.Index[0],
		IndexPath: pkField.Index,
	}
	skMeta := &model.FieldMetadata{
		Name:      "SK",
		DBName:    "sk",
		Type:      reflect.TypeOf(""),
		Index:     skField.Index[0],
		IndexPath: skField.Index,
	}

	meta := &model.Metadata{
		TableName:      "tbl",
		Fields:         make(map[string]*model.FieldMetadata),
		FieldsByDBName: make(map[string]*model.FieldMetadata),
		PrimaryKey: &model.KeySchema{
			PartitionKey: pkMeta,
			SortKey:      skMeta,
		},
	}

	meta.Fields["PK"] = pkMeta
	meta.FieldsByDBName["pk"] = pkMeta
	meta.Fields["SK"] = skMeta
	meta.FieldsByDBName["sk"] = skMeta

	return meta
}

func TestBuilder_applyConditionsToBuilder_CoversConditionKinds(t *testing.T) {
	converter := pkgTypes.NewConverter()
	b := &Builder{converter: converter}
	meta := cov5ItemMetadata(t)

	builder := expr.NewBuilderWithConverter(converter)
	raw, err := b.applyConditionsToBuilder(meta, builder, []core.TransactCondition{
		{Field: "Status", Operator: "=", Value: "ok"},
		{Kind: core.TransactConditionKindPrimaryKeyExists},
		{Kind: core.TransactConditionKindPrimaryKeyNotExists},
		{Kind: core.TransactConditionKindVersionEquals, Value: int64(1)},
		{Kind: core.TransactConditionKindExpression, Expression: "status = :v", Values: map[string]any{":v": "ok"}},
	})
	require.NoError(t, err)
	require.Len(t, raw, 1)
}

func TestBuilder_buildBuilderUpdate_AppliesBuilderAndRawConditions(t *testing.T) {
	converter := pkgTypes.NewConverter()
	b := &Builder{converter: converter}
	meta := cov5ItemMetadata(t)

	item := cov5Item{PK: "p1", Status: "ok", Version: 1}
	op := transactOperation{
		model:    &item,
		metadata: meta,
		typ:      opUpdateWithBuilder,
		updateFn: func(ub core.UpdateBuilder) error {
			ub.Set("Status", "new")
			return nil
		},
		conditions: []core.TransactCondition{
			{Kind: core.TransactConditionKindPrimaryKeyExists},
			{Kind: core.TransactConditionKindVersionEquals, Value: int64(1)},
			{Kind: core.TransactConditionKindField, Field: "status", Operator: "=", Value: "ok"},
			{Kind: core.TransactConditionKindExpression, Expression: "attribute_exists(pk)", Values: map[string]any{}},
		},
	}

	update, err := b.buildBuilderUpdate(op, 0)
	require.NoError(t, err)
	require.NotNil(t, update)
	require.NotNil(t, update.UpdateExpression)
	require.NotEmpty(t, aws.ToString(update.UpdateExpression))
	require.NotNil(t, update.ConditionExpression)
	require.NotEmpty(t, aws.ToString(update.ConditionExpression))
	require.NotEmpty(t, update.Key)
}

func TestOperationType_String_CoversKnownAndUnknown_COV5(t *testing.T) {
	require.Equal(t, "Put", opPut.String())
	require.Equal(t, "Create", opCreate.String())
	require.Equal(t, "Update", opUpdate.String())
	require.Equal(t, "UpdateWithBuilder", opUpdateWithBuilder.String())
	require.Equal(t, "Delete", opDelete.String())
	require.Equal(t, "ConditionCheck", opConditionCheck.String())
	require.Equal(t, "Unknown", operationType(999).String())
}

func TestMetadataAdapter_AttributeMetadataAndVersionFieldName_COV5(t *testing.T) {
	meta := cov5ItemMetadata(t)

	adapter := &metadataAdapter{meta: meta}
	require.NotNil(t, adapter.AttributeMetadata("PK"))
	require.NotNil(t, adapter.AttributeMetadata("pk"))
	require.Nil(t, adapter.AttributeMetadata("missing"))

	require.Equal(t, "ver", adapter.VersionFieldName())

	meta.VersionField.DBName = ""
	require.Equal(t, "Version", adapter.VersionFieldName())

	meta.VersionField = nil
	require.Empty(t, adapter.VersionFieldName())

	adapter.meta = nil
	require.Nil(t, adapter.AttributeMetadata("PK"))
	require.Empty(t, adapter.VersionFieldName())
}

func TestBuilder_addOperation_RecordsErrors_COV5(t *testing.T) {
	b := &Builder{}
	b.addOperation(opPut, nil, nil, nil, nil)
	require.Error(t, b.err)

	b = &Builder{
		operations: make([]transactOperation, maxTransactOperations),
	}
	b.addOperation(opPut, &cov5Item{}, nil, nil, nil)
	require.Error(t, b.err)

	b = &Builder{
		registry: model.NewRegistry(),
	}
	b.addOperation(opPut, 42, nil, nil, nil)
	require.Error(t, b.err)
}

func TestTransaction_extractPrimaryKey_ErrorsOnEmptyPartitionKey_COV5(t *testing.T) {
	tx := &Transaction{converter: pkgTypes.NewConverter()}
	meta := cov5ItemMetadata(t)

	_, err := tx.extractPrimaryKey(&cov5Item{}, meta)
	require.Error(t, err)
}

func TestTransaction_extractPrimaryKey_RequiresSortKeyWhenPresent_COV5(t *testing.T) {
	tx := &Transaction{converter: pkgTypes.NewConverter()}
	meta := cov5CompositeItemMetadata(t)

	_, err := tx.extractPrimaryKey(&cov5CompositeItem{PK: "p1"}, meta)
	require.Error(t, err)

	key, err := tx.extractPrimaryKey(&cov5CompositeItem{PK: "p1", SK: "s1"}, meta)
	require.NoError(t, err)
	require.Contains(t, key, "pk")
	require.Contains(t, key, "sk")
}
