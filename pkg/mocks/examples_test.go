package mocks_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/theory-cloud/tabletheory/pkg/mocks"
)

// ExampleConnectionStore demonstrates how to test infrastructure code
// that uses AWS SDK operations directly
type ExampleConnectionStore struct {
	client    *mocks.MockDynamoDBClient
	tableName string
}

func (s *ExampleConnectionStore) CreateTable(ctx context.Context) error {
	_, err := s.client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(s.tableName),
		KeySchema: []types.KeySchemaElement{
			{
				AttributeName: aws.String("pk"),
				KeyType:       types.KeyTypeHash,
			},
		},
		AttributeDefinitions: []types.AttributeDefinition{
			{
				AttributeName: aws.String("pk"),
				AttributeType: types.ScalarAttributeTypeS,
			},
		},
		BillingMode: types.BillingModePayPerRequest,
	})
	return err
}

func (s *ExampleConnectionStore) WaitForTable(ctx context.Context, waiter *mocks.MockTableExistsWaiter) error {
	return waiter.Wait(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(s.tableName),
	}, 5*time.Minute)
}

func (s *ExampleConnectionStore) EnableTTL(ctx context.Context) error {
	_, err := s.client.UpdateTimeToLive(ctx, &dynamodb.UpdateTimeToLiveInput{
		TableName: aws.String(s.tableName),
		TimeToLiveSpecification: &types.TimeToLiveSpecification{
			AttributeName: aws.String("ttl"),
			Enabled:       aws.Bool(true),
		},
	})
	return err
}

// TestInfrastructureCodeWithAWSMocks demonstrates testing infrastructure code
// that directly uses AWS SDK operations
func TestInfrastructureCodeWithAWSMocks(t *testing.T) {
	// Setup mocks
	mockClient := new(mocks.MockDynamoDBClient)
	mockWaiter := new(mocks.MockTableExistsWaiter)

	store := &ExampleConnectionStore{
		client:    mockClient,
		tableName: "test-connections",
	}

	ctx := context.Background()

	// Test CreateTable
	t.Run("CreateTable", func(t *testing.T) {
		expectedOutput := mocks.NewMockCreateTableOutput("test-connections")
		mockClient.On("CreateTable", ctx, mock.AnythingOfType("*dynamodb.CreateTableInput"), mock.Anything).
			Return(expectedOutput, nil)

		err := store.CreateTable(ctx)
		assert.NoError(t, err)

		mockClient.AssertExpectations(t)
	})

	// Test WaitForTable
	t.Run("WaitForTable", func(t *testing.T) {
		mockWaiter.On("Wait", ctx, mock.AnythingOfType("*dynamodb.DescribeTableInput"), 5*time.Minute, mock.Anything).
			Return(nil)

		err := store.WaitForTable(ctx, mockWaiter)
		assert.NoError(t, err)

		mockWaiter.AssertExpectations(t)
	})

	// Test EnableTTL
	t.Run("EnableTTL", func(t *testing.T) {
		expectedOutput := mocks.NewMockUpdateTimeToLiveOutput("test-connections")
		mockClient.On("UpdateTimeToLive", ctx, mock.AnythingOfType("*dynamodb.UpdateTimeToLiveInput"), mock.Anything).
			Return(expectedOutput, nil)

		err := store.EnableTTL(ctx)
		assert.NoError(t, err)

		mockClient.AssertExpectations(t)
	})
}

// TestCompleteInfrastructureWorkflow demonstrates testing a complete
// infrastructure setup workflow
func TestCompleteInfrastructureWorkflow(t *testing.T) {
	mockClient := new(mocks.MockDynamoDBClient)
	mockWaiter := new(mocks.MockTableExistsWaiter)

	store := &ExampleConnectionStore{
		client:    mockClient,
		tableName: "connections",
	}

	ctx := context.Background()

	// Setup all expectations for the complete workflow

	// 1. CreateTable expectation
	createOutput := mocks.NewMockCreateTableOutput("connections")
	mockClient.On("CreateTable", ctx, mock.AnythingOfType("*dynamodb.CreateTableInput"), mock.Anything).
		Return(createOutput, nil)

	// 2. Wait for table expectation
	mockWaiter.On("Wait", ctx, mock.AnythingOfType("*dynamodb.DescribeTableInput"), 5*time.Minute, mock.Anything).
		Return(nil)

	// 3. Enable TTL expectation
	ttlOutput := mocks.NewMockUpdateTimeToLiveOutput("connections")
	mockClient.On("UpdateTimeToLive", ctx, mock.AnythingOfType("*dynamodb.UpdateTimeToLiveInput"), mock.Anything).
		Return(ttlOutput, nil)

	// Execute the complete workflow
	err := store.CreateTable(ctx)
	assert.NoError(t, err)

	err = store.WaitForTable(ctx, mockWaiter)
	assert.NoError(t, err)

	err = store.EnableTTL(ctx)
	assert.NoError(t, err)

	// Verify all expectations were met
	mockClient.AssertExpectations(t)
	mockWaiter.AssertExpectations(t)
}

// TestErrorScenarios demonstrates testing error conditions
func TestErrorScenarios(t *testing.T) {
	mockClient := new(mocks.MockDynamoDBClient)
	mockWaiter := new(mocks.MockTableExistsWaiter)

	store := &ExampleConnectionStore{
		client:    mockClient,
		tableName: "test-table",
	}

	ctx := context.Background()

	t.Run("CreateTable_TableAlreadyExists", func(t *testing.T) {
		// Reset mock for this test
		mockClient.ExpectedCalls = nil

		expectedErr := &types.ResourceInUseException{
			Message: aws.String("Table already exists"),
		}
		mockClient.On("CreateTable", ctx, mock.AnythingOfType("*dynamodb.CreateTableInput"), mock.Anything).
			Return(nil, expectedErr)

		err := store.CreateTable(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Table already exists")

		mockClient.AssertExpectations(t)
	})

	t.Run("WaitForTable_Timeout", func(t *testing.T) {
		// Reset mock for this test
		mockWaiter.ExpectedCalls = nil

		expectedErr := errors.New("waiter timeout")
		mockWaiter.On("Wait", ctx, mock.AnythingOfType("*dynamodb.DescribeTableInput"), 5*time.Minute, mock.Anything).
			Return(expectedErr)

		err := store.WaitForTable(ctx, mockWaiter)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "waiter timeout")

		mockWaiter.AssertExpectations(t)
	})
}

// TestDataOperationsWithMocks demonstrates testing data operations
func TestDataOperationsWithMocks(t *testing.T) {
	mockClient := new(mocks.MockDynamoDBClient)
	ctx := context.Background()

	t.Run("GetItem_Success", func(t *testing.T) {
		getInput := &dynamodb.GetItemInput{
			TableName: aws.String("connections"),
			Key: map[string]types.AttributeValue{
				"pk": &types.AttributeValueMemberS{Value: "CONNECTION#123"},
			},
		}

		expectedOutput := &dynamodb.GetItemOutput{
			Item: map[string]types.AttributeValue{
				"pk":         &types.AttributeValueMemberS{Value: "CONNECTION#123"},
				"id":         &types.AttributeValueMemberS{Value: "123"},
				"user_id":    &types.AttributeValueMemberS{Value: "user-456"},
				"created_at": &types.AttributeValueMemberS{Value: "2023-01-01T00:00:00Z"},
			},
		}

		mockClient.On("GetItem", ctx, getInput, mock.Anything).Return(expectedOutput, nil)

		result, err := mockClient.GetItem(ctx, getInput)
		assert.NoError(t, err)
		assert.NotNil(t, result.Item)
		assert.Equal(t, "CONNECTION#123", result.Item["pk"].(*types.AttributeValueMemberS).Value)

		mockClient.AssertExpectations(t)
	})

	t.Run("PutItem_Success", func(t *testing.T) {
		putInput := &dynamodb.PutItemInput{
			TableName: aws.String("connections"),
			Item: map[string]types.AttributeValue{
				"pk":         &types.AttributeValueMemberS{Value: "CONNECTION#123"},
				"id":         &types.AttributeValueMemberS{Value: "123"},
				"user_id":    &types.AttributeValueMemberS{Value: "user-456"},
				"created_at": &types.AttributeValueMemberS{Value: "2023-01-01T00:00:00Z"},
			},
		}

		expectedOutput := &dynamodb.PutItemOutput{}
		mockClient.On("PutItem", ctx, putInput, mock.Anything).Return(expectedOutput, nil)

		result, err := mockClient.PutItem(ctx, putInput)
		assert.NoError(t, err)
		assert.NotNil(t, result)

		mockClient.AssertExpectations(t)
	})
}

// TestCombinedTableTheoryAndAWSMocks demonstrates using both levels of mocking together
func TestCombinedTableTheoryAndAWSMocks(t *testing.T) {
	// TableTheory level mocks for application logic
	mockDB := new(mocks.MockExtendedDB)
	mockQuery := new(mocks.MockQuery)

	// AWS SDK level mocks for infrastructure logic
	mockClient := new(mocks.MockDynamoDBClient)

	ctx := context.Background()

	// Test application logic with TableTheory mocks
	t.Run("ApplicationLogic", func(t *testing.T) {
		type User struct {
			ID   string
			Name string
		}

		mockDB.On("Model", mock.AnythingOfType("*mocks_test.User")).Return(mockQuery)
		mockQuery.On("Where", "ID", "=", "123").Return(mockQuery)
		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			user := args.Get(0).(*User)
			user.ID = "123"
			user.Name = "Test User"
		}).Return(nil)

		// Simulate application code
		var user User
		err := mockDB.Model(&User{}).Where("ID", "=", "123").First(&user)

		assert.NoError(t, err)
		assert.Equal(t, "123", user.ID)
		assert.Equal(t, "Test User", user.Name)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	// Test infrastructure logic with AWS SDK mocks
	t.Run("InfrastructureLogic", func(t *testing.T) {
		describeInput := &dynamodb.DescribeTableInput{
			TableName: aws.String("users"),
		}

		expectedOutput := mocks.NewMockDescribeTableOutput("users", types.TableStatusActive)
		mockClient.On("DescribeTable", ctx, describeInput, mock.Anything).Return(expectedOutput, nil)

		// Simulate infrastructure code
		result, err := mockClient.DescribeTable(ctx, describeInput)

		assert.NoError(t, err)
		assert.Equal(t, "users", *result.Table.TableName)
		assert.Equal(t, types.TableStatusActive, result.Table.TableStatus)

		mockClient.AssertExpectations(t)
	})
}
