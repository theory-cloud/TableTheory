package core

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// BatchWriteResult contains the result of a batch write operation
type BatchWriteResult struct {
	UnprocessedItems map[string][]types.WriteRequest
	ConsumedCapacity []types.ConsumedCapacity
}

// BatchWriteExecutor implements batch write operations for DynamoDB
type BatchWriteExecutor struct {
	client *dynamodb.Client
	ctx    context.Context
}

// NewBatchWriteExecutor creates a new batch write executor
func NewBatchWriteExecutor(client *dynamodb.Client, ctx context.Context) *BatchWriteExecutor { //nolint:revive // context-as-argument: keep signature for compatibility
	return &BatchWriteExecutor{
		client: client,
		ctx:    ctx,
	}
}

// ExecuteBatchWriteItem executes a batch write operation
func (e *BatchWriteExecutor) ExecuteBatchWriteItem(tableName string, writeRequests []types.WriteRequest) (*BatchWriteResult, error) {
	if len(writeRequests) == 0 {
		return &BatchWriteResult{}, nil
	}

	// DynamoDB BatchWriteItem supports max 25 items per request
	if len(writeRequests) > 25 {
		return nil, fmt.Errorf("batch write supports maximum 25 items per request, got %d", len(writeRequests))
	}

	// Build the request
	input := &dynamodb.BatchWriteItemInput{
		RequestItems: map[string][]types.WriteRequest{
			tableName: writeRequests,
		},
		ReturnConsumedCapacity: types.ReturnConsumedCapacityTotal,
	}

	// Execute the batch write
	output, err := e.client.BatchWriteItem(e.ctx, input)
	if err != nil {
		return nil, fmt.Errorf("batch write failed: %w", err)
	}

	// Build the result
	result := &BatchWriteResult{
		UnprocessedItems: output.UnprocessedItems,
		ConsumedCapacity: output.ConsumedCapacity,
	}

	return result, nil
}

// ExecuteQuery implements the QueryExecutor interface
func (e *BatchWriteExecutor) ExecuteQuery(input *CompiledQuery, dest any) error {
	_ = input
	_ = dest
	// BatchWriteExecutor is optimized for batch write operations.
	// For query operations, use the query package's MainExecutor or theorydb.Model().
	return fmt.Errorf("BatchWriteExecutor does not support ExecuteQuery - this executor is specialized for batch write operations only. Use theorydb.Model() for queries")
}

// ExecuteScan implements the QueryExecutor interface
func (e *BatchWriteExecutor) ExecuteScan(input *CompiledQuery, dest any) error {
	_ = input
	_ = dest
	// BatchWriteExecutor is optimized for batch write operations.
	// For scan operations, use the query package's MainExecutor or theorydb.Model().
	return fmt.Errorf("BatchWriteExecutor does not support ExecuteScan - this executor is specialized for batch write operations only. Use theorydb.Model() for scans")
}

// BatchDeleteWithResult performs batch delete and returns detailed results
func (e *BatchWriteExecutor) BatchDeleteWithResult(tableName string, keys []map[string]types.AttributeValue) (*BatchDeleteResult, error) {
	if len(keys) == 0 {
		return &BatchDeleteResult{
			Succeeded: 0,
			Failed:    0,
		}, nil
	}

	// Convert keys to write requests
	writeRequests := make([]types.WriteRequest, 0, len(keys))
	for _, key := range keys {
		writeRequests = append(writeRequests, types.WriteRequest{
			DeleteRequest: &types.DeleteRequest{
				Key: key,
			},
		})
	}

	// Split into batches of 25
	result := &BatchDeleteResult{
		Errors: make([]error, 0),
	}

	for i := 0; i < len(writeRequests); i += 25 {
		end := i + 25
		if end > len(writeRequests) {
			end = len(writeRequests)
		}

		batch := writeRequests[i:end]
		batchResult, err := e.ExecuteBatchWriteItem(tableName, batch)
		if err != nil {
			result.Failed += len(batch)
			result.Errors = append(result.Errors, err)
			continue
		}

		// Count successful deletes
		unprocessedCount := 0
		for _, items := range batchResult.UnprocessedItems {
			unprocessedCount += len(items)
		}

		result.Succeeded += len(batch) - unprocessedCount
		result.Failed += unprocessedCount

		// Add unprocessed items to result
		if unprocessedCount > 0 {
			for _, items := range batchResult.UnprocessedItems {
				for _, item := range items {
					if item.DeleteRequest != nil {
						result.UnprocessedKeys = append(result.UnprocessedKeys, item.DeleteRequest.Key)
					}
				}
			}
		}
	}

	return result, nil
}

// BatchDeleteResult represents the result of a batch delete operation
type BatchDeleteResult struct {
	UnprocessedKeys []map[string]types.AttributeValue
	Errors          []error
	Succeeded       int
	Failed          int
}

// ExecutorWithBatchSupport wraps an executor to add batch write support
type ExecutorWithBatchSupport struct {
	*UpdateExecutor
	*BatchWriteExecutor
	deleteClient *dynamodb.Client
}

func (e *ExecutorWithBatchSupport) ctxOrBackground() context.Context {
	if e.BatchWriteExecutor.ctx != nil {
		return e.BatchWriteExecutor.ctx
	}
	return context.Background()
}

func compiledQueryWriteConditions(input *CompiledQuery) (*string, map[string]string, map[string]types.AttributeValue) {
	var conditionExpression *string
	if input.ConditionExpression != "" {
		conditionExpression = aws.String(input.ConditionExpression)
	}

	var expressionAttributeNames map[string]string
	if len(input.ExpressionAttributeNames) > 0 {
		expressionAttributeNames = input.ExpressionAttributeNames
	}

	var expressionAttributeValues map[string]types.AttributeValue
	if len(input.ExpressionAttributeValues) > 0 {
		expressionAttributeValues = input.ExpressionAttributeValues
	}

	return conditionExpression, expressionAttributeNames, expressionAttributeValues
}

// NewExecutorWithBatchSupport creates a new executor with batch support
func NewExecutorWithBatchSupport(client *dynamodb.Client, ctx context.Context) *ExecutorWithBatchSupport { //nolint:revive // context-as-argument: keep signature for compatibility
	return &ExecutorWithBatchSupport{
		UpdateExecutor:     NewUpdateExecutor(client, ctx),
		BatchWriteExecutor: NewBatchWriteExecutor(client, ctx),
		deleteClient:       client,
	}
}

// ExecuteDeleteItem implements DeleteItemExecutor interface
func (e *ExecutorWithBatchSupport) ExecuteDeleteItem(input *CompiledQuery, key map[string]types.AttributeValue) error {
	conditionExpression, expressionAttributeNames, expressionAttributeValues := compiledQueryWriteConditions(input)
	deleteInput := &dynamodb.DeleteItemInput{
		TableName:                 aws.String(input.TableName),
		Key:                       key,
		ConditionExpression:       conditionExpression,
		ExpressionAttributeNames:  expressionAttributeNames,
		ExpressionAttributeValues: expressionAttributeValues,
	}

	// Execute delete
	_, err := e.deleteClient.DeleteItem(e.ctxOrBackground(), deleteInput)
	if err != nil {
		return fmt.Errorf("failed to delete item: %w", err)
	}

	return nil
}

// ExecutePutItem implements PutItemExecutor interface
func (e *ExecutorWithBatchSupport) ExecutePutItem(input *CompiledQuery, item map[string]types.AttributeValue) error {
	conditionExpression, expressionAttributeNames, expressionAttributeValues := compiledQueryWriteConditions(input)
	putInput := &dynamodb.PutItemInput{
		TableName:                 aws.String(input.TableName),
		Item:                      item,
		ConditionExpression:       conditionExpression,
		ExpressionAttributeNames:  expressionAttributeNames,
		ExpressionAttributeValues: expressionAttributeValues,
	}

	// Execute put
	_, err := e.deleteClient.PutItem(e.ctxOrBackground(), putInput)
	if err != nil {
		return fmt.Errorf("failed to put item: %w", err)
	}

	return nil
}
