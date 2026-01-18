package mocks

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/core"
)

func TestMockDynamoDBClient_DataOperations(t *testing.T) {
	client := new(MockDynamoDBClient)
	ctx := context.Background()

	client.On("DeleteItem", mock.Anything, mock.Anything, mock.Anything).
		Return(&dynamodb.DeleteItemOutput{}, nil).Once()
	client.On("Query", mock.Anything, mock.Anything, mock.Anything).
		Return(&dynamodb.QueryOutput{}, nil).Once()
	client.On("Scan", mock.Anything, mock.Anything, mock.Anything).
		Return(&dynamodb.ScanOutput{}, nil).Once()
	client.On("UpdateItem", mock.Anything, mock.Anything, mock.Anything).
		Return(&dynamodb.UpdateItemOutput{}, nil).Once()
	client.On("BatchGetItem", mock.Anything, mock.Anything, mock.Anything).
		Return(&dynamodb.BatchGetItemOutput{}, nil).Once()
	client.On("BatchWriteItem", mock.Anything, mock.Anything, mock.Anything).
		Return(&dynamodb.BatchWriteItemOutput{}, nil).Once()

	_, err := client.DeleteItem(ctx, &dynamodb.DeleteItemInput{})
	require.NoError(t, err)
	_, err = client.Query(ctx, &dynamodb.QueryInput{})
	require.NoError(t, err)
	_, err = client.Scan(ctx, &dynamodb.ScanInput{})
	require.NoError(t, err)
	_, err = client.UpdateItem(ctx, &dynamodb.UpdateItemInput{})
	require.NoError(t, err)
	_, err = client.BatchGetItem(ctx, &dynamodb.BatchGetItemInput{})
	require.NoError(t, err)
	_, err = client.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{})
	require.NoError(t, err)

	client.AssertExpectations(t)
}

func TestMockExtendedDB_MethodCoverage(t *testing.T) {
	db := NewMockExtendedDBStrict()

	db.On("AutoMigrateWithOptions", mock.Anything, mock.Anything).Return(nil).Once()
	db.On("RegisterTypeConverter", mock.Anything, mock.Anything).Return(nil).Once()
	db.On("CreateTable", mock.Anything, mock.Anything).Return(nil).Once()
	db.On("EnsureTable", mock.Anything).Return(nil).Once()
	db.On("DeleteTable", mock.Anything).Return(nil).Once()

	desc := struct{ Table string }{Table: "t"}
	db.On("DescribeTable", mock.Anything).Return(desc, nil).Once()
	db.On("WithLambdaTimeout", mock.Anything).Return(db).Once()
	db.On("WithLambdaTimeoutBuffer", mock.Anything).Return(db).Once()
	db.On("TransactionFunc", mock.Anything).Return(nil).Once()
	db.On("Transact").Return(nil).Once()
	db.On("TransactWrite", mock.Anything, mock.Anything).Return(nil).Once()

	require.NoError(t, db.AutoMigrateWithOptions(&struct{}{}, "opt"))
	require.NoError(t, db.RegisterTypeConverter(reflect.TypeOf(""), nil))
	require.NoError(t, db.CreateTable(&struct{}{}, "opt"))
	require.NoError(t, db.EnsureTable(&struct{}{}))
	require.NoError(t, db.DeleteTable(&struct{}{}))

	got, err := db.DescribeTable(&struct{}{})
	require.NoError(t, err)
	require.Equal(t, desc, got)

	timeoutDB, ok := db.WithLambdaTimeout(ctxKeyContext(t)).(*MockExtendedDB)
	require.True(t, ok)
	require.Same(t, db, timeoutDB)

	bufferedDB, ok := db.WithLambdaTimeoutBuffer(10 * time.Millisecond).(*MockExtendedDB)
	require.True(t, ok)
	require.Same(t, db, bufferedDB)

	require.NoError(t, db.TransactionFunc(func(any) error { return nil }))
	require.Nil(t, db.Transact())
	require.NoError(t, db.TransactWrite(context.Background(), func(core.TransactionBuilder) error { return nil }))

	db.AssertExpectations(t)
}

func TestMockQuery_MethodCoverage(t *testing.T) {
	q := new(MockQuery)

	q.On("OrFilter", "a", "=", 1).Return(q).Once()
	q.On("FilterGroup", mock.Anything).Return(q).Once()
	q.On("OrFilterGroup", mock.Anything).Return(q).Once()
	q.On("IfNotExists").Return(q).Once()
	q.On("IfExists").Return(q).Once()
	q.On("WithCondition", "a", "=", 1).Return(q).Once()
	q.On("WithConditionExpression", "a = :v", mock.Anything).Return(q).Once()
	q.On("Select", []string{"a", "b"}).Return(q).Once()
	q.On("CreateOrUpdate").Return(nil).Once()
	q.On("BatchGetWithOptions", mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
	builder := new(MockBatchGetBuilder)
	q.On("BatchGetBuilder").Return(builder).Once()
	q.On("SetCursor", "cursor").Return(nil).Once()
	q.On("WithContext", mock.Anything).Return(q).Once()
	q.On("BatchUpdateWithOptions", mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()

	require.Same(t, q, q.OrFilter("a", "=", 1))
	require.Same(t, q, q.FilterGroup(func(core.Query) {}))
	require.Same(t, q, q.OrFilterGroup(func(core.Query) {}))
	require.Same(t, q, q.IfNotExists())
	require.Same(t, q, q.IfExists())
	require.Same(t, q, q.WithCondition("a", "=", 1))
	require.Same(t, q, q.WithConditionExpression("a = :v", map[string]any{":v": 1}))
	require.Same(t, q, q.Select("a", "b"))

	require.NoError(t, q.CreateOrUpdate())
	require.NoError(t, q.BatchGetWithOptions([]any{"k"}, &[]any{}, &core.BatchGetOptions{}))
	require.Same(t, builder, q.BatchGetBuilder())

	require.NoError(t, q.SetCursor("cursor"))
	require.Same(t, q, q.WithContext(context.Background()))

	require.NoError(t, q.BatchUpdateWithOptions([]any{}, []string{"f"}, "opt"))

	q.AssertExpectations(t)
}

func ctxKeyContext(t *testing.T) context.Context {
	t.Helper()
	type ctxKey struct{}
	return context.WithValue(context.Background(), ctxKey{}, errors.New("x"))
}
