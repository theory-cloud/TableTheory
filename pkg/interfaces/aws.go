// Package interfaces provides abstractions for AWS SDK operations to enable mocking
package interfaces

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

// DynamoDBClientInterface abstracts the AWS DynamoDB client operations
// that need to be mockable for testing infrastructure code.
type DynamoDBClientInterface interface {
	// Table Management Operations
	CreateTable(ctx context.Context, params *dynamodb.CreateTableInput, optFns ...func(*dynamodb.Options)) (*dynamodb.CreateTableOutput, error)
	DescribeTable(ctx context.Context, params *dynamodb.DescribeTableInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DescribeTableOutput, error)
	DeleteTable(ctx context.Context, params *dynamodb.DeleteTableInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteTableOutput, error)
	UpdateTimeToLive(ctx context.Context, params *dynamodb.UpdateTimeToLiveInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateTimeToLiveOutput, error)

	// Data Operations (for completeness)
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	DeleteItem(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	Scan(ctx context.Context, params *dynamodb.ScanInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error)
	UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
	BatchGetItem(ctx context.Context, params *dynamodb.BatchGetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchGetItemOutput, error)
	BatchWriteItem(ctx context.Context, params *dynamodb.BatchWriteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchWriteItemOutput, error)
}

// TableWaiterInterface abstracts DynamoDB table waiters for mocking
type TableWaiterInterface interface {
	// Wait waits for a table to reach the desired state
	Wait(ctx context.Context, params *dynamodb.DescribeTableInput, maxWaitDur time.Duration, optFns ...func(*dynamodb.TableExistsWaiterOptions)) error
}

// TableNotExistsWaiterInterface abstracts DynamoDB table not exists waiters for mocking
type TableNotExistsWaiterInterface interface {
	// Wait waits for a table to be deleted
	Wait(ctx context.Context, params *dynamodb.DescribeTableInput, maxWaitDur time.Duration, optFns ...func(*dynamodb.TableNotExistsWaiterOptions)) error
}

// DynamoDBClientWrapper wraps the real AWS DynamoDB client to implement our interface
type DynamoDBClientWrapper struct {
	client *dynamodb.Client
}

// NewDynamoDBClientWrapper creates a new wrapper around the AWS DynamoDB client
func NewDynamoDBClientWrapper(client *dynamodb.Client) *DynamoDBClientWrapper {
	return &DynamoDBClientWrapper{client: client}
}

// Table Management Operations
func (w *DynamoDBClientWrapper) CreateTable(ctx context.Context, params *dynamodb.CreateTableInput, optFns ...func(*dynamodb.Options)) (*dynamodb.CreateTableOutput, error) {
	return w.client.CreateTable(ctx, params, optFns...)
}

func (w *DynamoDBClientWrapper) DescribeTable(ctx context.Context, params *dynamodb.DescribeTableInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DescribeTableOutput, error) {
	return w.client.DescribeTable(ctx, params, optFns...)
}

func (w *DynamoDBClientWrapper) DeleteTable(ctx context.Context, params *dynamodb.DeleteTableInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteTableOutput, error) {
	return w.client.DeleteTable(ctx, params, optFns...)
}

func (w *DynamoDBClientWrapper) UpdateTimeToLive(ctx context.Context, params *dynamodb.UpdateTimeToLiveInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateTimeToLiveOutput, error) {
	return w.client.UpdateTimeToLive(ctx, params, optFns...)
}

// Data Operations
func (w *DynamoDBClientWrapper) GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	return w.client.GetItem(ctx, params, optFns...)
}

func (w *DynamoDBClientWrapper) PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	return w.client.PutItem(ctx, params, optFns...)
}

func (w *DynamoDBClientWrapper) DeleteItem(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	return w.client.DeleteItem(ctx, params, optFns...)
}

func (w *DynamoDBClientWrapper) Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	return w.client.Query(ctx, params, optFns...)
}

func (w *DynamoDBClientWrapper) Scan(ctx context.Context, params *dynamodb.ScanInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
	return w.client.Scan(ctx, params, optFns...)
}

func (w *DynamoDBClientWrapper) UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	return w.client.UpdateItem(ctx, params, optFns...)
}

func (w *DynamoDBClientWrapper) BatchGetItem(ctx context.Context, params *dynamodb.BatchGetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchGetItemOutput, error) {
	return w.client.BatchGetItem(ctx, params, optFns...)
}

func (w *DynamoDBClientWrapper) BatchWriteItem(ctx context.Context, params *dynamodb.BatchWriteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchWriteItemOutput, error) {
	return w.client.BatchWriteItem(ctx, params, optFns...)
}

// TableExistsWaiterWrapper wraps the real AWS table exists waiter
type TableExistsWaiterWrapper struct {
	waiter *dynamodb.TableExistsWaiter
}

// NewTableExistsWaiterWrapper creates a new wrapper around the AWS table exists waiter
func NewTableExistsWaiterWrapper(client *dynamodb.Client) *TableExistsWaiterWrapper {
	return &TableExistsWaiterWrapper{
		waiter: dynamodb.NewTableExistsWaiter(client),
	}
}

func (w *TableExistsWaiterWrapper) Wait(ctx context.Context, params *dynamodb.DescribeTableInput, maxWaitDur time.Duration, optFns ...func(*dynamodb.TableExistsWaiterOptions)) error {
	return w.waiter.Wait(ctx, params, maxWaitDur, optFns...)
}

// TableNotExistsWaiterWrapper wraps the real AWS table not exists waiter
type TableNotExistsWaiterWrapper struct {
	waiter *dynamodb.TableNotExistsWaiter
}

// NewTableNotExistsWaiterWrapper creates a new wrapper around the AWS table not exists waiter
func NewTableNotExistsWaiterWrapper(client *dynamodb.Client) *TableNotExistsWaiterWrapper {
	return &TableNotExistsWaiterWrapper{
		waiter: dynamodb.NewTableNotExistsWaiter(client),
	}
}

func (w *TableNotExistsWaiterWrapper) Wait(ctx context.Context, params *dynamodb.DescribeTableInput, maxWaitDur time.Duration, optFns ...func(*dynamodb.TableNotExistsWaiterOptions)) error {
	return w.waiter.Wait(ctx, params, maxWaitDur, optFns...)
}
