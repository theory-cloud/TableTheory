package query

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/core"
	customerrors "github.com/theory-cloud/tabletheory/pkg/errors"
)

type stubDynamoDB struct {
	putErr error
	delErr error

	lastQuery *dynamodb.QueryInput
	lastScan  *dynamodb.ScanInput

	queryCalls int
	scanCalls  int
	putCalls   int
	delCalls   int
	batchCalls int
}

func (s *stubDynamoDB) Query(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	s.queryCalls++
	s.lastQuery = params

	if s.queryCalls == 1 {
		return &dynamodb.QueryOutput{
			Items: []map[string]ddbTypes.AttributeValue{
				{"id": &ddbTypes.AttributeValueMemberS{Value: "q1"}},
			},
			LastEvaluatedKey: map[string]ddbTypes.AttributeValue{
				"id": &ddbTypes.AttributeValueMemberS{Value: "q1"},
			},
		}, nil
	}

	return &dynamodb.QueryOutput{
		Items: []map[string]ddbTypes.AttributeValue{
			{"id": &ddbTypes.AttributeValueMemberS{Value: "q2"}},
		},
	}, nil
}

func (s *stubDynamoDB) Scan(_ context.Context, params *dynamodb.ScanInput, _ ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
	s.scanCalls++
	s.lastScan = params

	return &dynamodb.ScanOutput{
		Items: []map[string]ddbTypes.AttributeValue{
			{"id": &ddbTypes.AttributeValueMemberS{Value: "s1"}},
		},
	}, nil
}

func (s *stubDynamoDB) GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	return &dynamodb.GetItemOutput{}, nil
}

func (s *stubDynamoDB) PutItem(context.Context, *dynamodb.PutItemInput, ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	s.putCalls++
	if s.putErr != nil {
		return nil, s.putErr
	}
	return &dynamodb.PutItemOutput{}, nil
}

func (s *stubDynamoDB) UpdateItem(context.Context, *dynamodb.UpdateItemInput, ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	return &dynamodb.UpdateItemOutput{}, nil
}

func (s *stubDynamoDB) DeleteItem(context.Context, *dynamodb.DeleteItemInput, ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	s.delCalls++
	if s.delErr != nil {
		return nil, s.delErr
	}
	return &dynamodb.DeleteItemOutput{}, nil
}

func (s *stubDynamoDB) BatchGetItem(context.Context, *dynamodb.BatchGetItemInput, ...func(*dynamodb.Options)) (*dynamodb.BatchGetItemOutput, error) {
	return &dynamodb.BatchGetItemOutput{}, nil
}

func (s *stubDynamoDB) BatchWriteItem(context.Context, *dynamodb.BatchWriteItemInput, ...func(*dynamodb.Options)) (*dynamodb.BatchWriteItemOutput, error) {
	s.batchCalls++
	return &dynamodb.BatchWriteItemOutput{}, nil
}

func TestMainExecutor_ExecuteQueryAndScan(t *testing.T) {
	client := &stubDynamoDB{}
	exec := NewExecutor(client, context.Background())

	type item struct {
		ID string `dynamodb:"id"`
	}

	limit := int32(2)
	compiled := &core.CompiledQuery{
		Operation: "Query",
		TableName: "tbl",
		IndexName: "idx",
		Limit:     &limit,
	}

	var out []item
	require.NoError(t, exec.ExecuteQuery(compiled, &out))
	require.Len(t, out, 2)
	require.NotNil(t, client.lastQuery)
	require.Equal(t, "idx", *client.lastQuery.IndexName)

	out = nil
	require.NoError(t, exec.ExecuteScan(&core.CompiledQuery{Operation: "Scan", TableName: "tbl"}, &out))
	require.Len(t, out, 1)
	require.NotNil(t, client.lastScan)
}

func TestMainExecutor_ConditionalWritesAndBatchWrite(t *testing.T) {
	client := &stubDynamoDB{}
	exec := NewExecutor(client, context.Background())

	require.Error(t, exec.ExecutePutItem(&core.CompiledQuery{TableName: "tbl"}, nil))
	require.Error(t, exec.ExecuteDeleteItem(&core.CompiledQuery{TableName: "tbl"}, nil))

	require.NoError(t, exec.ExecutePutItem(&core.CompiledQuery{TableName: "tbl"}, map[string]ddbTypes.AttributeValue{
		"id": &ddbTypes.AttributeValueMemberS{Value: "1"},
	}))
	require.NoError(t, exec.ExecuteDeleteItem(&core.CompiledQuery{TableName: "tbl"}, map[string]ddbTypes.AttributeValue{
		"id": &ddbTypes.AttributeValueMemberS{Value: "1"},
	}))

	require.Equal(t, 1, client.putCalls)
	require.Equal(t, 1, client.delCalls)

	require.NoError(t, exec.ExecuteBatchWrite(&CompiledBatchWrite{
		TableName: "tbl",
		Items: []map[string]ddbTypes.AttributeValue{
			{"id": &ddbTypes.AttributeValueMemberS{Value: "1"}},
		},
	}))
	require.Equal(t, 1, client.batchCalls)
}

func TestMainExecutor_ConditionalFailureWrapped(t *testing.T) {
	client := &stubDynamoDB{
		putErr: &ddbTypes.ConditionalCheckFailedException{},
	}
	exec := NewExecutor(client, context.Background())

	err := exec.ExecutePutItem(&core.CompiledQuery{TableName: "tbl"}, map[string]ddbTypes.AttributeValue{
		"id": &ddbTypes.AttributeValueMemberS{Value: "1"},
	})
	require.Error(t, err)
	require.ErrorIs(t, err, customerrors.ErrConditionFailed)
}

func TestUnmarshalItems_CoversPointerSetsAndMaps(t *testing.T) {
	type nested struct {
		Field string
	}
	type target struct {
		Any       any      `dynamodb:"any"`
		Binary    []byte   `dynamodb:"bin"`
		BinarySet [][]byte `dynamodb:"bset"`
		ID        string   `dynamodb:"id"`
		Nested    nested   `dynamodb:"nested"`
		Numbers   []int    `dynamodb:"nums"`
		Ptr       *string  `dynamodb:"ptr"`
		Tags      []string `dynamodb:"tags"`
	}

	items := []map[string]ddbTypes.AttributeValue{
		{
			"id":   &ddbTypes.AttributeValueMemberS{Value: "1"},
			"ptr":  &ddbTypes.AttributeValueMemberS{Value: "x"},
			"tags": &ddbTypes.AttributeValueMemberSS{Value: []string{"a", "b"}},
			"nums": &ddbTypes.AttributeValueMemberNS{Value: []string{"1", "2"}},
			"bin":  &ddbTypes.AttributeValueMemberB{Value: []byte("data")},
			"bset": &ddbTypes.AttributeValueMemberBS{Value: [][]byte{[]byte("a")}},
			"nested": &ddbTypes.AttributeValueMemberM{Value: map[string]ddbTypes.AttributeValue{
				"Field": &ddbTypes.AttributeValueMemberS{Value: "v"},
			}},
			"any": &ddbTypes.AttributeValueMemberL{Value: []ddbTypes.AttributeValue{
				&ddbTypes.AttributeValueMemberN{Value: "1"},
				&ddbTypes.AttributeValueMemberM{Value: map[string]ddbTypes.AttributeValue{
					"k": &ddbTypes.AttributeValueMemberS{Value: "v"},
				}},
			}},
		},
	}

	var out []target
	require.NoError(t, UnmarshalItems(items, &out))
	require.Len(t, out, 1)
	require.NotNil(t, out[0].Ptr)
	require.Equal(t, "x", *out[0].Ptr)
	require.ElementsMatch(t, []string{"a", "b"}, out[0].Tags)
	require.Equal(t, []int{1, 2}, out[0].Numbers)
	require.Equal(t, "v", out[0].Nested.Field)
	require.NotNil(t, out[0].Any)

	var single target
	require.NoError(t, UnmarshalItems(items, &single))
	require.Equal(t, "1", single.ID)

	var empty target
	require.Error(t, UnmarshalItems(nil, &empty))
}
