// Package mocks provides mock implementations for TableTheory interfaces and AWS SDK operations
package mocks

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/mock"
)

// MockDynamoDBClient provides a mock implementation of the AWS DynamoDB client
// for testing infrastructure code that directly uses the AWS SDK.
//
// This complements the existing TableTheory interface mocks by providing
// low-level AWS SDK operation mocking.
//
// Example usage:
//
//	mockClient := new(mocks.MockDynamoDBClient)
//	mockClient.On("CreateTable", mock.Anything, mock.Anything).Return(&dynamodb.CreateTableOutput{}, nil)
//
//	// Use in your infrastructure code
//	store := &DynamoDBConnectionStore{client: mockClient}
type MockDynamoDBClient struct {
	mock.Mock
}

// Table Management Operations

// CreateTable mocks the DynamoDB CreateTable operation
func (m *MockDynamoDBClient) CreateTable(ctx context.Context, params *dynamodb.CreateTableInput, optFns ...func(*dynamodb.Options)) (*dynamodb.CreateTableOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	output, ok := args.Get(0).(*dynamodb.CreateTableOutput)
	if !ok {
		panic("unexpected type: expected *dynamodb.CreateTableOutput")
	}
	return output, args.Error(1)
}

// DescribeTable mocks the DynamoDB DescribeTable operation
func (m *MockDynamoDBClient) DescribeTable(ctx context.Context, params *dynamodb.DescribeTableInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DescribeTableOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	output, ok := args.Get(0).(*dynamodb.DescribeTableOutput)
	if !ok {
		panic("unexpected type: expected *dynamodb.DescribeTableOutput")
	}
	return output, args.Error(1)
}

// DeleteTable mocks the DynamoDB DeleteTable operation
func (m *MockDynamoDBClient) DeleteTable(ctx context.Context, params *dynamodb.DeleteTableInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteTableOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	output, ok := args.Get(0).(*dynamodb.DeleteTableOutput)
	if !ok {
		panic("unexpected type: expected *dynamodb.DeleteTableOutput")
	}
	return output, args.Error(1)
}

// UpdateTimeToLive mocks the DynamoDB UpdateTimeToLive operation
func (m *MockDynamoDBClient) UpdateTimeToLive(ctx context.Context, params *dynamodb.UpdateTimeToLiveInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateTimeToLiveOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	output, ok := args.Get(0).(*dynamodb.UpdateTimeToLiveOutput)
	if !ok {
		panic("unexpected type: expected *dynamodb.UpdateTimeToLiveOutput")
	}
	return output, args.Error(1)
}

// Data Operations (for completeness with infrastructure testing)

// GetItem mocks the DynamoDB GetItem operation
func (m *MockDynamoDBClient) GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	output, ok := args.Get(0).(*dynamodb.GetItemOutput)
	if !ok {
		panic("unexpected type: expected *dynamodb.GetItemOutput")
	}
	return output, args.Error(1)
}

// PutItem mocks the DynamoDB PutItem operation
func (m *MockDynamoDBClient) PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	output, ok := args.Get(0).(*dynamodb.PutItemOutput)
	if !ok {
		panic("unexpected type: expected *dynamodb.PutItemOutput")
	}
	return output, args.Error(1)
}

// DeleteItem mocks the DynamoDB DeleteItem operation
func (m *MockDynamoDBClient) DeleteItem(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	output, ok := args.Get(0).(*dynamodb.DeleteItemOutput)
	if !ok {
		panic("unexpected type: expected *dynamodb.DeleteItemOutput")
	}
	return output, args.Error(1)
}

// Query mocks the DynamoDB Query operation
func (m *MockDynamoDBClient) Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	output, ok := args.Get(0).(*dynamodb.QueryOutput)
	if !ok {
		panic("unexpected type: expected *dynamodb.QueryOutput")
	}
	return output, args.Error(1)
}

// Scan mocks the DynamoDB Scan operation
func (m *MockDynamoDBClient) Scan(ctx context.Context, params *dynamodb.ScanInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	output, ok := args.Get(0).(*dynamodb.ScanOutput)
	if !ok {
		panic("unexpected type: expected *dynamodb.ScanOutput")
	}
	return output, args.Error(1)
}

// UpdateItem mocks the DynamoDB UpdateItem operation
func (m *MockDynamoDBClient) UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	output, ok := args.Get(0).(*dynamodb.UpdateItemOutput)
	if !ok {
		panic("unexpected type: expected *dynamodb.UpdateItemOutput")
	}
	return output, args.Error(1)
}

// BatchGetItem mocks the DynamoDB BatchGetItem operation
func (m *MockDynamoDBClient) BatchGetItem(ctx context.Context, params *dynamodb.BatchGetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchGetItemOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	output, ok := args.Get(0).(*dynamodb.BatchGetItemOutput)
	if !ok {
		panic("unexpected type: expected *dynamodb.BatchGetItemOutput")
	}
	return output, args.Error(1)
}

// BatchWriteItem mocks the DynamoDB BatchWriteItem operation
func (m *MockDynamoDBClient) BatchWriteItem(ctx context.Context, params *dynamodb.BatchWriteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchWriteItemOutput, error) {
	args := m.Called(ctx, params, optFns)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	output, ok := args.Get(0).(*dynamodb.BatchWriteItemOutput)
	if !ok {
		panic("unexpected type: expected *dynamodb.BatchWriteItemOutput")
	}
	return output, args.Error(1)
}

// MockTableExistsWaiter provides a mock implementation of the DynamoDB table exists waiter
//
// Example usage:
//
//	mockWaiter := new(mocks.MockTableExistsWaiter)
//	mockWaiter.On("Wait", mock.Anything, mock.Anything, mock.Anything).Return(nil)
type MockTableExistsWaiter struct {
	mock.Mock
}

// Wait mocks waiting for a table to exist
func (m *MockTableExistsWaiter) Wait(ctx context.Context, params *dynamodb.DescribeTableInput, maxWaitDur time.Duration, optFns ...func(*dynamodb.TableExistsWaiterOptions)) error {
	args := m.Called(ctx, params, maxWaitDur, optFns)
	return args.Error(0)
}

// MockTableNotExistsWaiter provides a mock implementation of the DynamoDB table not exists waiter
//
// Example usage:
//
//	mockWaiter := new(mocks.MockTableNotExistsWaiter)
//	mockWaiter.On("Wait", mock.Anything, mock.Anything, mock.Anything).Return(nil)
type MockTableNotExistsWaiter struct {
	mock.Mock
}

// Wait mocks waiting for a table to not exist (be deleted)
func (m *MockTableNotExistsWaiter) Wait(ctx context.Context, params *dynamodb.DescribeTableInput, maxWaitDur time.Duration, optFns ...func(*dynamodb.TableNotExistsWaiterOptions)) error {
	args := m.Called(ctx, params, maxWaitDur, optFns)
	return args.Error(0)
}

// Helper functions for creating common mock responses

// NewMockCreateTableOutput creates a mock CreateTable response
func NewMockCreateTableOutput(tableName string) *dynamodb.CreateTableOutput {
	return &dynamodb.CreateTableOutput{
		TableDescription: &types.TableDescription{
			TableName:   &tableName,
			TableStatus: types.TableStatusCreating,
		},
	}
}

// NewMockDescribeTableOutput creates a mock DescribeTable response
func NewMockDescribeTableOutput(tableName string, status types.TableStatus) *dynamodb.DescribeTableOutput {
	return &dynamodb.DescribeTableOutput{
		Table: &types.TableDescription{
			TableName:   &tableName,
			TableStatus: status,
		},
	}
}

// NewMockDeleteTableOutput creates a mock DeleteTable response
func NewMockDeleteTableOutput(tableName string) *dynamodb.DeleteTableOutput {
	return &dynamodb.DeleteTableOutput{
		TableDescription: &types.TableDescription{
			TableName:   &tableName,
			TableStatus: types.TableStatusDeleting,
		},
	}
}

// NewMockUpdateTimeToLiveOutput creates a mock UpdateTimeToLive response
func NewMockUpdateTimeToLiveOutput(tableName string) *dynamodb.UpdateTimeToLiveOutput {
	_ = tableName

	return &dynamodb.UpdateTimeToLiveOutput{
		TimeToLiveSpecification: &types.TimeToLiveSpecification{
			AttributeName: stringPtr("ttl"),
			Enabled:       boolPtr(true),
		},
	}
}

// Helper functions for pointer creation
func stringPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}

// Type aliases for convenience
type (
	// DynamoDBClient is an alias for MockDynamoDBClient
	DynamoDBClient = MockDynamoDBClient

	// TableExistsWaiter is an alias for MockTableExistsWaiter
	TableExistsWaiter = MockTableExistsWaiter

	// TableNotExistsWaiter is an alias for MockTableNotExistsWaiter
	TableNotExistsWaiter = MockTableNotExistsWaiter
)
