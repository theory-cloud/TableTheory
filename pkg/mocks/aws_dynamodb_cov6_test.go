package mocks_test

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/theory-cloud/tabletheory/pkg/mocks"
)

func TestMockDynamoDBClient_AdditionalOperations_COV6(t *testing.T) {
	mockClient := new(mocks.MockDynamoDBClient)
	ctx := context.Background()

	t.Run("query", func(t *testing.T) {
		input := &dynamodb.QueryInput{TableName: aws.String("tbl")}
		mockClient.On("Query", ctx, input, mock.Anything).Return(&dynamodb.QueryOutput{}, nil).Once()

		out, err := mockClient.Query(ctx, input)
		assert.NoError(t, err)
		assert.NotNil(t, out)
	})

	t.Run("scan", func(t *testing.T) {
		input := &dynamodb.ScanInput{TableName: aws.String("tbl")}
		mockClient.On("Scan", ctx, input, mock.Anything).Return(&dynamodb.ScanOutput{}, nil).Once()

		out, err := mockClient.Scan(ctx, input)
		assert.NoError(t, err)
		assert.NotNil(t, out)
	})

	t.Run("update item error returns nil output", func(t *testing.T) {
		input := &dynamodb.UpdateItemInput{TableName: aws.String("tbl")}
		expectedErr := errors.New("update failed")
		mockClient.On("UpdateItem", ctx, input, mock.Anything).Return(nil, expectedErr).Once()

		out, err := mockClient.UpdateItem(ctx, input)
		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, out)
	})

	t.Run("batch get", func(t *testing.T) {
		input := &dynamodb.BatchGetItemInput{}
		mockClient.On("BatchGetItem", ctx, input, mock.Anything).Return(&dynamodb.BatchGetItemOutput{}, nil).Once()

		out, err := mockClient.BatchGetItem(ctx, input)
		assert.NoError(t, err)
		assert.NotNil(t, out)
	})

	t.Run("batch write", func(t *testing.T) {
		input := &dynamodb.BatchWriteItemInput{}
		mockClient.On("BatchWriteItem", ctx, input, mock.Anything).Return(&dynamodb.BatchWriteItemOutput{}, nil).Once()

		out, err := mockClient.BatchWriteItem(ctx, input)
		assert.NoError(t, err)
		assert.NotNil(t, out)
	})

	mockClient.AssertExpectations(t)
}

func TestMockDynamoDBClient_PanicsOnUnexpectedReturnTypes_COV6(t *testing.T) {
	mockClient := new(mocks.MockDynamoDBClient)
	ctx := context.Background()

	input := &dynamodb.QueryInput{TableName: aws.String("tbl")}
	mockClient.On("Query", ctx, input, mock.Anything).Return("bad-type", nil).Once()

	assert.Panics(t, func() {
		_, err := mockClient.Query(ctx, input)
		assert.NoError(t, err)
	})

	mockClient.AssertExpectations(t)
}

func TestMockDynamoDBClient_ErrorBranches_COV6(t *testing.T) {
	ctx := context.Background()
	expectedErr := errors.New("boom")

	cases := []struct {
		call func(client *mocks.MockDynamoDBClient) (any, error)
		name string
	}{
		{
			name: "DescribeTable",
			call: func(client *mocks.MockDynamoDBClient) (any, error) {
				input := &dynamodb.DescribeTableInput{TableName: aws.String("tbl")}
				client.On("DescribeTable", ctx, input, mock.Anything).Return(nil, expectedErr).Once()
				return client.DescribeTable(ctx, input)
			},
		},
		{
			name: "DeleteTable",
			call: func(client *mocks.MockDynamoDBClient) (any, error) {
				input := &dynamodb.DeleteTableInput{TableName: aws.String("tbl")}
				client.On("DeleteTable", ctx, input, mock.Anything).Return(nil, expectedErr).Once()
				return client.DeleteTable(ctx, input)
			},
		},
		{
			name: "UpdateTimeToLive",
			call: func(client *mocks.MockDynamoDBClient) (any, error) {
				input := &dynamodb.UpdateTimeToLiveInput{TableName: aws.String("tbl")}
				client.On("UpdateTimeToLive", ctx, input, mock.Anything).Return(nil, expectedErr).Once()
				return client.UpdateTimeToLive(ctx, input)
			},
		},
		{
			name: "GetItem",
			call: func(client *mocks.MockDynamoDBClient) (any, error) {
				input := &dynamodb.GetItemInput{TableName: aws.String("tbl")}
				client.On("GetItem", ctx, input, mock.Anything).Return(nil, expectedErr).Once()
				return client.GetItem(ctx, input)
			},
		},
		{
			name: "PutItem",
			call: func(client *mocks.MockDynamoDBClient) (any, error) {
				input := &dynamodb.PutItemInput{TableName: aws.String("tbl")}
				client.On("PutItem", ctx, input, mock.Anything).Return(nil, expectedErr).Once()
				return client.PutItem(ctx, input)
			},
		},
		{
			name: "DeleteItem",
			call: func(client *mocks.MockDynamoDBClient) (any, error) {
				input := &dynamodb.DeleteItemInput{TableName: aws.String("tbl")}
				client.On("DeleteItem", ctx, input, mock.Anything).Return(nil, expectedErr).Once()
				return client.DeleteItem(ctx, input)
			},
		},
		{
			name: "Query",
			call: func(client *mocks.MockDynamoDBClient) (any, error) {
				input := &dynamodb.QueryInput{TableName: aws.String("tbl")}
				client.On("Query", ctx, input, mock.Anything).Return(nil, expectedErr).Once()
				return client.Query(ctx, input)
			},
		},
		{
			name: "Scan",
			call: func(client *mocks.MockDynamoDBClient) (any, error) {
				input := &dynamodb.ScanInput{TableName: aws.String("tbl")}
				client.On("Scan", ctx, input, mock.Anything).Return(nil, expectedErr).Once()
				return client.Scan(ctx, input)
			},
		},
		{
			name: "BatchGetItem",
			call: func(client *mocks.MockDynamoDBClient) (any, error) {
				input := &dynamodb.BatchGetItemInput{}
				client.On("BatchGetItem", ctx, input, mock.Anything).Return(nil, expectedErr).Once()
				return client.BatchGetItem(ctx, input)
			},
		},
		{
			name: "BatchWriteItem",
			call: func(client *mocks.MockDynamoDBClient) (any, error) {
				input := &dynamodb.BatchWriteItemInput{}
				client.On("BatchWriteItem", ctx, input, mock.Anything).Return(nil, expectedErr).Once()
				return client.BatchWriteItem(ctx, input)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := new(mocks.MockDynamoDBClient)
			out, err := tc.call(client)
			assert.ErrorIs(t, err, expectedErr)
			assert.Nil(t, out)
			client.AssertExpectations(t)
		})
	}
}

func TestMockDynamoDBClient_PanicBranches_COV6(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		call func(t *testing.T, client *mocks.MockDynamoDBClient)
		name string
	}{
		{
			name: "CreateTable",
			call: func(t *testing.T, client *mocks.MockDynamoDBClient) {
				input := &dynamodb.CreateTableInput{TableName: aws.String("tbl")}
				client.On("CreateTable", ctx, input, mock.Anything).Return("bad-type", nil).Once()
				_, err := client.CreateTable(ctx, input)
				assert.NoError(t, err)
			},
		},
		{
			name: "DescribeTable",
			call: func(t *testing.T, client *mocks.MockDynamoDBClient) {
				input := &dynamodb.DescribeTableInput{TableName: aws.String("tbl")}
				client.On("DescribeTable", ctx, input, mock.Anything).Return("bad-type", nil).Once()
				_, err := client.DescribeTable(ctx, input)
				assert.NoError(t, err)
			},
		},
		{
			name: "DeleteTable",
			call: func(t *testing.T, client *mocks.MockDynamoDBClient) {
				input := &dynamodb.DeleteTableInput{TableName: aws.String("tbl")}
				client.On("DeleteTable", ctx, input, mock.Anything).Return("bad-type", nil).Once()
				_, err := client.DeleteTable(ctx, input)
				assert.NoError(t, err)
			},
		},
		{
			name: "UpdateTimeToLive",
			call: func(t *testing.T, client *mocks.MockDynamoDBClient) {
				input := &dynamodb.UpdateTimeToLiveInput{TableName: aws.String("tbl")}
				client.On("UpdateTimeToLive", ctx, input, mock.Anything).Return("bad-type", nil).Once()
				_, err := client.UpdateTimeToLive(ctx, input)
				assert.NoError(t, err)
			},
		},
		{
			name: "GetItem",
			call: func(t *testing.T, client *mocks.MockDynamoDBClient) {
				input := &dynamodb.GetItemInput{TableName: aws.String("tbl")}
				client.On("GetItem", ctx, input, mock.Anything).Return("bad-type", nil).Once()
				_, err := client.GetItem(ctx, input)
				assert.NoError(t, err)
			},
		},
		{
			name: "PutItem",
			call: func(t *testing.T, client *mocks.MockDynamoDBClient) {
				input := &dynamodb.PutItemInput{TableName: aws.String("tbl")}
				client.On("PutItem", ctx, input, mock.Anything).Return("bad-type", nil).Once()
				_, err := client.PutItem(ctx, input)
				assert.NoError(t, err)
			},
		},
		{
			name: "DeleteItem",
			call: func(t *testing.T, client *mocks.MockDynamoDBClient) {
				input := &dynamodb.DeleteItemInput{TableName: aws.String("tbl")}
				client.On("DeleteItem", ctx, input, mock.Anything).Return("bad-type", nil).Once()
				_, err := client.DeleteItem(ctx, input)
				assert.NoError(t, err)
			},
		},
		{
			name: "Scan",
			call: func(t *testing.T, client *mocks.MockDynamoDBClient) {
				input := &dynamodb.ScanInput{TableName: aws.String("tbl")}
				client.On("Scan", ctx, input, mock.Anything).Return("bad-type", nil).Once()
				_, err := client.Scan(ctx, input)
				assert.NoError(t, err)
			},
		},
		{
			name: "UpdateItem",
			call: func(t *testing.T, client *mocks.MockDynamoDBClient) {
				input := &dynamodb.UpdateItemInput{TableName: aws.String("tbl")}
				client.On("UpdateItem", ctx, input, mock.Anything).Return("bad-type", nil).Once()
				_, err := client.UpdateItem(ctx, input)
				assert.NoError(t, err)
			},
		},
		{
			name: "BatchGetItem",
			call: func(t *testing.T, client *mocks.MockDynamoDBClient) {
				input := &dynamodb.BatchGetItemInput{}
				client.On("BatchGetItem", ctx, input, mock.Anything).Return("bad-type", nil).Once()
				_, err := client.BatchGetItem(ctx, input)
				assert.NoError(t, err)
			},
		},
		{
			name: "BatchWriteItem",
			call: func(t *testing.T, client *mocks.MockDynamoDBClient) {
				input := &dynamodb.BatchWriteItemInput{}
				client.On("BatchWriteItem", ctx, input, mock.Anything).Return("bad-type", nil).Once()
				_, err := client.BatchWriteItem(ctx, input)
				assert.NoError(t, err)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := new(mocks.MockDynamoDBClient)
			assert.Panics(t, func() { tc.call(t, client) })
			client.AssertExpectations(t)
		})
	}
}
