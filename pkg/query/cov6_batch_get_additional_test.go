package query

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/core"
)

type cov6BatchGetItem struct {
	_  struct{} `theorydb:"naming:snake_case"`
	PK string   `theorydb:"pk"`
}

func TestBatchGetBuilder_SelectEmptyAndParallelDisable_COV6(t *testing.T) {
	exec := &cov5BatchExecutor{}
	q := New(&cov6BatchGetItem{}, cov5Metadata{
		table: "tbl",
		primaryKey: core.KeySchema{
			PartitionKey: "PK",
		},
		attributes: map[string]*core.AttributeMetadata{
			"PK": {Name: "PK", DynamoDBName: "pk"},
		},
	}, exec)

	keys := []any{core.NewKeyPair("p1"), core.NewKeyPair("p2")}

	var out []map[string]types.AttributeValue
	require.NoError(t, q.BatchGetBuilder().
		Keys(keys).
		Parallel(1).
		Select().
		Execute(&out))
	require.Len(t, out, 2)
}

func TestQuery_BatchGetWithOptions_ValidatesInputsAndUnmarshalsStructs_COV6(t *testing.T) {
	exec := &cov5BatchExecutor{}

	t.Run("metadata required", func(t *testing.T) {
		q := New(&cov6BatchGetItem{}, nil, exec)
		var out []cov6BatchGetItem
		require.ErrorContains(t, q.BatchGetWithOptions([]any{"p1"}, &out, nil), "model metadata is required")
	})

	t.Run("keys required", func(t *testing.T) {
		q := New(&cov6BatchGetItem{}, cov5Metadata{table: "tbl", primaryKey: core.KeySchema{PartitionKey: "PK"}}, exec)
		var out []cov6BatchGetItem
		require.ErrorContains(t, q.BatchGetWithOptions(nil, &out, nil), "no keys provided")
	})

	t.Run("dest must be pointer to slice", func(t *testing.T) {
		q := New(&cov6BatchGetItem{}, cov5Metadata{table: "tbl", primaryKey: core.KeySchema{PartitionKey: "PK"}}, exec)
		require.ErrorContains(t, q.BatchGetWithOptions([]any{"p1"}, nil, nil), "dest must be a pointer to slice")
		var bad []cov6BatchGetItem
		require.ErrorContains(t, q.BatchGetWithOptions([]any{"p1"}, bad, nil), "dest must be a pointer to slice")
	})

	t.Run("executor must support batch operations", func(t *testing.T) {
		q := New(&cov6BatchGetItem{}, cov5Metadata{table: "tbl", primaryKey: core.KeySchema{PartitionKey: "PK"}}, &struct{ QueryExecutor }{})
		var out []cov6BatchGetItem
		require.ErrorContains(t, q.BatchGetWithOptions([]any{"p1"}, &out, nil), "executor does not support batch operations")
	})

	t.Run("unmarshals into typed destination", func(t *testing.T) {
		q := New(&cov6BatchGetItem{}, cov5Metadata{
			table: "tbl",
			primaryKey: core.KeySchema{
				PartitionKey: "PK",
			},
			attributes: map[string]*core.AttributeMetadata{
				"PK": {Name: "PK", DynamoDBName: "pk"},
			},
		}, exec)

		var out []cov6BatchGetItem
		require.NoError(t, q.BatchGetWithOptions([]any{"p1"}, &out, nil))
		require.Len(t, out, 1)
		require.Equal(t, "p1", out[0].PK)
	})
}

func TestQuery_buildBatchGetKey_HandlesCommonKeyForms_COV6(t *testing.T) {
	qPartitionOnly := New(&cov6BatchGetItem{}, cov5Metadata{
		table: "tbl",
		primaryKey: core.KeySchema{
			PartitionKey: "PK",
		},
		attributes: map[string]*core.AttributeMetadata{
			"PK": {Name: "PK", DynamoDBName: "pk"},
		},
	}, &cov5BatchExecutor{})

	_, err := qPartitionOnly.buildBatchGetKey(nil)
	require.Error(t, err)

	_, err = qPartitionOnly.buildBatchGetKey(map[string]types.AttributeValue{})
	require.ErrorContains(t, err, "key map cannot be empty")

	_, err = qPartitionOnly.buildBatchGetKey(map[string]any{"pk": func() {}})
	require.ErrorContains(t, err, "failed to convert key attribute")

	attrs, err := qPartitionOnly.buildBatchGetKey("p1")
	require.NoError(t, err)
	_, ok := attrs["pk"].(*types.AttributeValueMemberS)
	require.True(t, ok)

	type keyStruct struct {
		PK string
	}
	attrs, err = qPartitionOnly.buildBatchGetKey(keyStruct{PK: "p2"})
	require.NoError(t, err)
	_, ok = attrs["pk"].(*types.AttributeValueMemberS)
	require.True(t, ok)

	qComposite := New(&cov6BatchGetItem{}, cov5Metadata{
		table: "tbl",
		primaryKey: core.KeySchema{
			PartitionKey: "PK",
			SortKey:      "SK",
		},
		attributes: map[string]*core.AttributeMetadata{
			"PK": {Name: "PK", DynamoDBName: "pk"},
			"SK": {Name: "SK", DynamoDBName: "sk"},
		},
	}, &cov5BatchExecutor{})

	_, err = qComposite.partitionOnlyKey("p1")
	require.ErrorContains(t, err, "composite key requires")

	_, err = qComposite.buildBatchGetKey(core.NewKeyPair("p1"))
	require.ErrorContains(t, err, "sort key value is required")
}

func TestAttributeValuesEqual_HandlesS_N_B_AndFallback_COV6(t *testing.T) {
	require.True(t, attributeValuesEqual(&types.AttributeValueMemberS{Value: "x"}, &types.AttributeValueMemberS{Value: "x"}))
	require.False(t, attributeValuesEqual(&types.AttributeValueMemberS{Value: "x"}, &types.AttributeValueMemberS{Value: "y"}))
	require.False(t, attributeValuesEqual(&types.AttributeValueMemberS{Value: "x"}, &types.AttributeValueMemberN{Value: "1"}))

	require.True(t, attributeValuesEqual(&types.AttributeValueMemberN{Value: "1"}, &types.AttributeValueMemberN{Value: "1"}))

	require.True(t, attributeValuesEqual(&types.AttributeValueMemberB{Value: []byte("a")}, &types.AttributeValueMemberB{Value: []byte("a")}))
	require.False(t, attributeValuesEqual(&types.AttributeValueMemberB{Value: []byte("a")}, &types.AttributeValueMemberB{Value: []byte("b")}))

	require.True(t, attributeValuesEqual(&types.AttributeValueMemberBOOL{Value: true}, &types.AttributeValueMemberBOOL{Value: true}))
}

func TestIsStructLike_HandlesPointersAndNonStructs_COV6(t *testing.T) {
	require.False(t, isStructLike(123))

	type s struct{ A int }
	require.True(t, isStructLike(s{}))
	require.True(t, isStructLike(&s{}))

	var sp *s
	require.False(t, isStructLike(sp))
}
