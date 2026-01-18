package core

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// DynamoDBUpdateAPI defines the interface for DynamoDB update operations
type DynamoDBUpdateAPI interface {
	UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
}

// UpdateExecutor implements UpdateItemExecutor interface for DynamoDB update operations
type UpdateExecutor struct {
	client DynamoDBUpdateAPI
	ctx    context.Context
}

// NewUpdateExecutor creates a new UpdateExecutor instance
func NewUpdateExecutor(client DynamoDBUpdateAPI, ctx context.Context) *UpdateExecutor { //nolint:revive // context-as-argument: keep signature for compatibility
	return &UpdateExecutor{
		client: client,
		ctx:    ctx,
	}
}

// ExecuteUpdateItem performs a DynamoDB UpdateItem operation
func (e *UpdateExecutor) ExecuteUpdateItem(input *CompiledQuery, key map[string]types.AttributeValue) error {
	if input == nil {
		return fmt.Errorf("compiled query cannot be nil")
	}

	if len(key) == 0 {
		return fmt.Errorf("key cannot be empty")
	}

	// Build UpdateItem input
	updateInput := &dynamodb.UpdateItemInput{
		TableName: aws.String(input.TableName),
		Key:       key,
	}

	// Set update expression
	if input.UpdateExpression != "" {
		updateInput.UpdateExpression = aws.String(input.UpdateExpression)
	}

	// Set condition expression
	if input.ConditionExpression != "" {
		updateInput.ConditionExpression = aws.String(input.ConditionExpression)
	}

	// Set expression attribute names
	if len(input.ExpressionAttributeNames) > 0 {
		updateInput.ExpressionAttributeNames = input.ExpressionAttributeNames
	}

	// Set expression attribute values
	if len(input.ExpressionAttributeValues) > 0 {
		updateInput.ExpressionAttributeValues = input.ExpressionAttributeValues
	}

	// Set return values (default to NONE if not specified)
	if input.ReturnValues != "" {
		updateInput.ReturnValues = types.ReturnValue(input.ReturnValues)
	} else {
		updateInput.ReturnValues = types.ReturnValueNone
	}

	// Execute the update
	_, err := e.client.UpdateItem(e.ctx, updateInput)
	if err != nil {
		return fmt.Errorf("failed to update item: %w", err)
	}

	return nil
}

// ExecuteUpdateItemWithResult performs UpdateItem and returns the result
func (e *UpdateExecutor) ExecuteUpdateItemWithResult(input *CompiledQuery, key map[string]types.AttributeValue) (*UpdateResult, error) {
	if input == nil {
		return nil, fmt.Errorf("compiled query cannot be nil")
	}

	if len(key) == 0 {
		return nil, fmt.Errorf("key cannot be empty")
	}

	// Build UpdateItem input
	updateInput := &dynamodb.UpdateItemInput{
		TableName: aws.String(input.TableName),
		Key:       key,
	}

	// Set update expression
	if input.UpdateExpression != "" {
		updateInput.UpdateExpression = aws.String(input.UpdateExpression)
	}

	// Set condition expression
	if input.ConditionExpression != "" {
		updateInput.ConditionExpression = aws.String(input.ConditionExpression)
	}

	// Set expression attribute names
	if len(input.ExpressionAttributeNames) > 0 {
		updateInput.ExpressionAttributeNames = input.ExpressionAttributeNames
	}

	// Set expression attribute values
	if len(input.ExpressionAttributeValues) > 0 {
		updateInput.ExpressionAttributeValues = input.ExpressionAttributeValues
	}

	// Set return values
	if input.ReturnValues != "" {
		updateInput.ReturnValues = types.ReturnValue(input.ReturnValues)
	} else {
		// Default to ALL_NEW for result returns
		updateInput.ReturnValues = types.ReturnValueAllNew
	}

	// Execute the update
	output, err := e.client.UpdateItem(e.ctx, updateInput)
	if err != nil {
		return nil, fmt.Errorf("failed to update item: %w", err)
	}

	// Build result
	result := &UpdateResult{
		Attributes: output.Attributes,
	}

	// Add consumed capacity if available
	if output.ConsumedCapacity != nil {
		result.ConsumedCapacity = &ConsumedCapacity{
			TableName:     aws.ToString(output.ConsumedCapacity.TableName),
			CapacityUnits: aws.ToFloat64(output.ConsumedCapacity.CapacityUnits),
		}

		if output.ConsumedCapacity.ReadCapacityUnits != nil {
			result.ConsumedCapacity.ReadCapacityUnits = aws.ToFloat64(output.ConsumedCapacity.ReadCapacityUnits)
		}
		if output.ConsumedCapacity.WriteCapacityUnits != nil {
			result.ConsumedCapacity.WriteCapacityUnits = aws.ToFloat64(output.ConsumedCapacity.WriteCapacityUnits)
		}
	}

	return result, nil
}

// UpdateResult represents the result of an UpdateItem operation
type UpdateResult struct {
	Attributes       map[string]types.AttributeValue
	ConsumedCapacity *ConsumedCapacity
}

// ConsumedCapacity represents consumed capacity information
type ConsumedCapacity struct {
	TableName          string
	CapacityUnits      float64
	ReadCapacityUnits  float64
	WriteCapacityUnits float64
}
