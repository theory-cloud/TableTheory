package transaction

import (
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/internal/expr"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/model"
	queryPkg "github.com/theory-cloud/tabletheory/pkg/query"
	pkgTypes "github.com/theory-cloud/tabletheory/pkg/types"
)

type noopQueryExecutor struct{}

func (noopQueryExecutor) ExecuteQuery(*core.CompiledQuery, any) error { return nil }
func (noopQueryExecutor) ExecuteScan(*core.CompiledQuery, any) error  { return nil }

func TestBuilder_versionAttributeName(t *testing.T) {
	b := &Builder{converter: pkgTypes.NewConverter()}

	meta := &model.Metadata{
		VersionField: &model.FieldMetadata{Name: "Version", DBName: "ver"},
		Fields:       make(map[string]*model.FieldMetadata),
	}
	got, err := b.versionAttributeName(meta)
	require.NoError(t, err)
	require.Equal(t, "ver", got)

	meta.VersionField = &model.FieldMetadata{Name: "Version"}
	got, err = b.versionAttributeName(meta)
	require.NoError(t, err)
	require.Equal(t, "Version", got)

	meta.VersionField = nil
	meta.Fields["Version"] = &model.FieldMetadata{Name: "Version", DBName: "v"}
	got, err = b.versionAttributeName(meta)
	require.NoError(t, err)
	require.Equal(t, "v", got)

	_, err = b.versionAttributeName(&model.Metadata{Fields: make(map[string]*model.FieldMetadata)})
	require.Error(t, err)
}

func TestBuilder_buildRawCondition(t *testing.T) {
	b := &Builder{converter: pkgTypes.NewConverter()}

	rc, err := b.buildRawCondition(core.TransactCondition{
		Expression: "a = :v",
		Values:     map[string]any{":v": "x"},
	})
	require.NoError(t, err)
	require.Equal(t, "a = :v", rc.expression)
	require.Contains(t, rc.values, ":v")

	_, err = b.buildRawCondition(core.TransactCondition{Expression: "   "})
	require.Error(t, err)

	_, err = b.buildRawCondition(core.TransactCondition{
		Expression: "a = :v",
		Values:     map[string]any{":v": make(chan int)},
	})
	require.Error(t, err)
}

func TestBuilder_addBuilderPrimaryKeyCondition_AndResolveVersionFieldName(t *testing.T) {
	b := &Builder{converter: pkgTypes.NewConverter()}

	q := queryPkg.New(&struct{}{}, nil, noopQueryExecutor{})
	ubAny := queryPkg.NewUpdateBuilder(q)
	ub, ok := ubAny.(*queryPkg.UpdateBuilder)
	require.True(t, ok)

	meta := &model.Metadata{
		PrimaryKey: &model.KeySchema{
			PartitionKey: &model.FieldMetadata{Name: "PK", DBName: "pk"},
			SortKey:      &model.FieldMetadata{Name: "SK", DBName: "sk"},
		},
		Fields: map[string]*model.FieldMetadata{
			"Version": {Name: "Version", DBName: "version"},
		},
	}

	require.NoError(t, b.addBuilderPrimaryKeyCondition(ub, meta, "attribute_exists"))

	versionName, err := b.resolveVersionFieldName(meta)
	require.NoError(t, err)
	require.Equal(t, "Version", versionName)

	_, err = b.resolveVersionFieldName(&model.Metadata{Fields: make(map[string]*model.FieldMetadata)})
	require.Error(t, err)

	require.Error(t, b.addBuilderPrimaryKeyCondition(ub, nil, "attribute_exists"))
}

func TestMetadataAdapter_IndexesAndVersionFieldName(t *testing.T) {
	meta := &model.Metadata{
		TableName: "tbl",
		PrimaryKey: &model.KeySchema{
			PartitionKey: &model.FieldMetadata{Name: "PK", DBName: "pk"},
			SortKey:      &model.FieldMetadata{Name: "SK", DBName: "sk"},
		},
		VersionField: &model.FieldMetadata{Name: "Version", DBName: "ver"},
		Fields: map[string]*model.FieldMetadata{
			"PK": {Name: "PK", DBName: "pk", Type: reflect.TypeOf("")},
		},
		FieldsByDBName: map[string]*model.FieldMetadata{
			"pk": {Name: "PK", DBName: "pk", Type: reflect.TypeOf("")},
		},
		Indexes: []model.IndexSchema{
			{
				Name:           "gsi",
				Type:           model.GlobalSecondaryIndex,
				PartitionKey:   &model.FieldMetadata{Name: "GPK", DBName: "gpk"},
				SortKey:        &model.FieldMetadata{Name: "GSK", DBName: "gsk"},
				ProjectionType: "ALL",
			},
		},
	}

	adapter := adaptMetadata(meta)
	require.NotNil(t, adapter)
	require.Equal(t, "tbl", adapter.TableName())
	require.Equal(t, "ver", adapter.VersionFieldName())
	require.NotEmpty(t, adapter.Indexes())
	require.NotNil(t, adapter.AttributeMetadata("PK"))
}

func TestCapturingUpdateExecutor_Methods(t *testing.T) {
	exec := &capturingUpdateExecutor{}

	require.ErrorContains(t, exec.ExecuteQuery(nil, nil), "query execution not supported")
	require.ErrorContains(t, exec.ExecuteScan(nil, nil), "scan execution not supported")

	compiled := &core.CompiledQuery{Operation: "UpdateItem", TableName: "tbl"}
	key := map[string]types.AttributeValue{
		"pk": &types.AttributeValueMemberS{Value: "p1"},
	}

	result, err := exec.ExecuteUpdateItemWithResult(compiled, key)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, exec.executed)
	require.Equal(t, 1, exec.compilations)
	require.Same(t, compiled, exec.compiled)
	require.Equal(t, key, exec.key)

	exec.executed = false
	exec.compilations = 0
	exec.compiled = nil
	exec.key = nil

	require.NoError(t, exec.ExecuteUpdateItem(compiled, key))
	require.True(t, exec.executed)
}

func TestBuilder_buildRawCondition_DuplicatePlaceholders(t *testing.T) {
	b := &Builder{converter: pkgTypes.NewConverter()}

	rc, err := b.buildRawCondition(core.TransactCondition{
		Expression: "a = :v",
		Values:     map[string]any{":v": "x"},
	})
	require.NoError(t, err)

	_, _, _, err = b.mergeRawConditions("a = :v", nil, map[string]types.AttributeValue{":v": &types.AttributeValueMemberS{Value: "y"}}, []rawCondition{rc})
	require.Error(t, err)
}

func TestBuilder_resolveAttributeName_FieldValidation(t *testing.T) {
	b := &Builder{converter: pkgTypes.NewConverter()}

	_, err := b.resolveAttributeName(&model.Metadata{}, "")
	require.Error(t, err)

	got, err := b.resolveAttributeName(nil, "field")
	require.NoError(t, err)
	require.Equal(t, "field", got)
}

func TestBuilder_addPrimaryKeyCondition_MissingMetadata(t *testing.T) {
	b := &Builder{converter: pkgTypes.NewConverter()}

	builder := expr.NewBuilder()
	err := b.addPrimaryKeyCondition(&model.Metadata{}, builder, "attribute_exists")
	require.Error(t, err)
	require.ErrorContains(t, err, "missing primary key metadata")
}
