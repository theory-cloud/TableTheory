package fakedb_test

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v4/pkg/testing/fakedb"
)

func sAttr(value string) types.AttributeValue {
	return &types.AttributeValueMemberS{Value: value}
}

// TestFakeKeepsDistinctCompositeKeysDistinct proves the fake does not alias two
// distinct DynamoDB composite-key tuples whose naive concatenation collides:
// (PK="a|S:b", SK="c") and (PK="a", SK="b|S:c").
func TestFakeKeepsDistinctCompositeKeysDistinct(t *testing.T) {
	ctx := context.Background()
	fake := fakedb.New()
	tableName := "composite_keys"

	_, err := fake.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(tableName),
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("PK"), KeyType: types.KeyTypeHash},
			{AttributeName: aws.String("SK"), KeyType: types.KeyTypeRange},
		},
	})
	require.NoError(t, err)

	firstItem := map[string]types.AttributeValue{
		"PK":   sAttr("a|S:b"),
		"SK":   sAttr("c"),
		"name": sAttr("first"),
	}
	secondItem := map[string]types.AttributeValue{
		"PK":   sAttr("a"),
		"SK":   sAttr("b|S:c"),
		"name": sAttr("second"),
	}
	firstKey := map[string]types.AttributeValue{"PK": sAttr("a|S:b"), "SK": sAttr("c")}
	secondKey := map[string]types.AttributeValue{"PK": sAttr("a"), "SK": sAttr("b|S:c")}

	// batch write puts both; a collision would leave only one item.
	_, err = fake.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{
		RequestItems: map[string][]types.WriteRequest{
			tableName: {
				{PutRequest: &types.PutRequest{Item: firstItem}},
				{PutRequest: &types.PutRequest{Item: secondItem}},
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, fake.Items(tableName), 2, "distinct composite keys must not alias")

	// get resolves each key to its own item.
	gotFirst, err := fake.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(tableName),
		Key:       firstKey,
	})
	require.NoError(t, err)
	require.Equal(t, sAttr("first"), gotFirst.Item["name"])

	gotSecond, err := fake.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(tableName),
		Key:       secondKey,
	})
	require.NoError(t, err)
	require.Equal(t, sAttr("second"), gotSecond.Item["name"])

	// batch get returns both.
	batch, err := fake.BatchGetItem(ctx, &dynamodb.BatchGetItemInput{
		RequestItems: map[string]types.KeysAndAttributes{
			tableName: {Keys: []map[string]types.AttributeValue{firstKey, secondKey}},
		},
	})
	require.NoError(t, err)
	require.Len(t, batch.Responses[tableName], 2)

	// transaction get still addresses the second item.
	transact, err := fake.TransactGetItems(ctx, &dynamodb.TransactGetItemsInput{
		TransactItems: []types.TransactGetItem{
			{Get: &types.Get{TableName: aws.String(tableName), Key: secondKey}},
		},
	})
	require.NoError(t, err)
	require.Equal(t, sAttr("second"), transact.Responses[0].Item["name"])

	// delete removes only the addressed item.
	_, err = fake.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(tableName),
		Key:       firstKey,
	})
	require.NoError(t, err)

	gotSecond, err = fake.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(tableName),
		Key:       secondKey,
	})
	require.NoError(t, err)
	require.Equal(t, sAttr("second"), gotSecond.Item["name"])

	// ordinary delimiter-free keys are unchanged.
	_, err = fake.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{Put: &types.Put{
				TableName: aws.String(tableName),
				Item: map[string]types.AttributeValue{
					"PK":   sAttr("USER#1"),
					"SK":   sAttr("A"),
					"name": sAttr("ordinary"),
				},
			}},
		},
	})
	require.NoError(t, err)

	ordinary, err := fake.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(tableName),
		Key:       map[string]types.AttributeValue{"PK": sAttr("USER#1"), "SK": sAttr("A")},
	})
	require.NoError(t, err)
	require.Equal(t, sAttr("ordinary"), ordinary.Item["name"])
}
