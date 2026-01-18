package query

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/theory-cloud/tabletheory/pkg/core"
	customerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/naming"
)

// DynamoDBAPI defines the interface for all DynamoDB operations
type DynamoDBAPI interface {
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	Scan(ctx context.Context, params *dynamodb.ScanInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error)
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
	DeleteItem(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
	BatchGetItem(ctx context.Context, params *dynamodb.BatchGetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchGetItemOutput, error)
	BatchWriteItem(ctx context.Context, params *dynamodb.BatchWriteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchWriteItemOutput, error)
}

// MainExecutor is the main executor that implements all executor interfaces
type MainExecutor struct {
	client DynamoDBAPI
	ctx    context.Context
}

func applyCompiledQueryReadFields(
	input *core.CompiledQuery,
	indexName **string,
	filterExpression **string,
	projectionExpression **string,
	expressionAttributeNames *map[string]string,
	expressionAttributeValues *map[string]types.AttributeValue,
	limit **int32,
	exclusiveStartKey *map[string]types.AttributeValue,
	consistentRead **bool,
) {
	if input.IndexName != "" {
		*indexName = &input.IndexName
	}
	if input.FilterExpression != "" {
		*filterExpression = &input.FilterExpression
	}
	if input.ProjectionExpression != "" {
		*projectionExpression = &input.ProjectionExpression
	}
	if len(input.ExpressionAttributeNames) > 0 {
		*expressionAttributeNames = input.ExpressionAttributeNames
	}
	if len(input.ExpressionAttributeValues) > 0 {
		*expressionAttributeValues = input.ExpressionAttributeValues
	}
	if input.Limit != nil {
		*limit = input.Limit
	}
	if len(input.ExclusiveStartKey) > 0 {
		*exclusiveStartKey = input.ExclusiveStartKey
	}
	if input.ConsistentRead != nil {
		*consistentRead = input.ConsistentRead
	}
}

func buildDynamoQueryInput(input *core.CompiledQuery) *dynamodb.QueryInput {
	queryInput := &dynamodb.QueryInput{
		TableName: &input.TableName,
	}

	applyCompiledQueryReadFields(
		input,
		&queryInput.IndexName,
		&queryInput.FilterExpression,
		&queryInput.ProjectionExpression,
		&queryInput.ExpressionAttributeNames,
		&queryInput.ExpressionAttributeValues,
		&queryInput.Limit,
		&queryInput.ExclusiveStartKey,
		&queryInput.ConsistentRead,
	)

	// Set key condition expression
	if input.KeyConditionExpression != "" {
		queryInput.KeyConditionExpression = &input.KeyConditionExpression
	}

	// Set scan index forward
	if input.ScanIndexForward != nil {
		queryInput.ScanIndexForward = input.ScanIndexForward
	}

	return queryInput
}

func buildDynamoScanInput(input *core.CompiledQuery) *dynamodb.ScanInput {
	scanInput := &dynamodb.ScanInput{
		TableName: &input.TableName,
	}

	applyCompiledQueryReadFields(
		input,
		&scanInput.IndexName,
		&scanInput.FilterExpression,
		&scanInput.ProjectionExpression,
		&scanInput.ExpressionAttributeNames,
		&scanInput.ExpressionAttributeValues,
		&scanInput.Limit,
		&scanInput.ExclusiveStartKey,
		&scanInput.ConsistentRead,
	)

	// Set segment and total segments for parallel scan
	if input.Segment != nil {
		scanInput.Segment = input.Segment
	}
	if input.TotalSegments != nil {
		scanInput.TotalSegments = input.TotalSegments
	}

	return scanInput
}

type pagedReadExecutor interface {
	fetch(exclusiveStartKey map[string]types.AttributeValue) ([]map[string]types.AttributeValue, map[string]types.AttributeValue, error)
}

type queryPager struct {
	client DynamoDBAPI
	ctx    context.Context
	input  *dynamodb.QueryInput
}

func (p queryPager) fetch(exclusiveStartKey map[string]types.AttributeValue) ([]map[string]types.AttributeValue, map[string]types.AttributeValue, error) {
	if exclusiveStartKey != nil {
		p.input.ExclusiveStartKey = exclusiveStartKey
	}

	output, err := p.client.Query(p.ctx, p.input)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to execute query: %w", err)
	}
	return output.Items, output.LastEvaluatedKey, nil
}

type scanPager struct {
	client DynamoDBAPI
	ctx    context.Context
	input  *dynamodb.ScanInput
}

func (p scanPager) fetch(exclusiveStartKey map[string]types.AttributeValue) ([]map[string]types.AttributeValue, map[string]types.AttributeValue, error) {
	if exclusiveStartKey != nil {
		p.input.ExclusiveStartKey = exclusiveStartKey
	}

	output, err := p.client.Scan(p.ctx, p.input)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to execute scan: %w", err)
	}
	return output.Items, output.LastEvaluatedKey, nil
}

func executePagedItems(limit *int32, pager pagedReadExecutor) ([]map[string]types.AttributeValue, error) {
	var allItems []map[string]types.AttributeValue
	var lastEvaluatedKey map[string]types.AttributeValue

	for {
		items, nextKey, err := pager.fetch(lastEvaluatedKey)
		if err != nil {
			return nil, err
		}

		allItems = append(allItems, items...)

		if nextKey == nil || (limit != nil && int64(len(allItems)) >= int64(*limit)) {
			break
		}
		lastEvaluatedKey = nextKey
	}

	return allItems, nil
}

// NewExecutor creates a new MainExecutor instance
func NewExecutor(client DynamoDBAPI, ctx context.Context) *MainExecutor { //nolint:revive // context-as-argument: keep signature for compatibility
	return &MainExecutor{
		client: client,
		ctx:    ctx,
	}
}

// SetContext updates the context used for subsequent DynamoDB calls.
func (e *MainExecutor) SetContext(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	e.ctx = ctx
}

// ExecuteQuery implements QueryExecutor.ExecuteQuery
func (e *MainExecutor) ExecuteQuery(input *core.CompiledQuery, dest any) error {
	if input == nil {
		return fmt.Errorf("compiled query cannot be nil")
	}

	pager := queryPager{
		client: e.client,
		ctx:    e.ctx,
		input:  buildDynamoQueryInput(input),
	}
	allItems, err := executePagedItems(input.Limit, pager)
	if err != nil {
		return err
	}

	// Unmarshal the results into dest
	return UnmarshalItems(allItems, dest)
}

// ExecuteScan implements QueryExecutor.ExecuteScan
func (e *MainExecutor) ExecuteScan(input *core.CompiledQuery, dest any) error {
	if input == nil {
		return fmt.Errorf("compiled query cannot be nil")
	}

	pager := scanPager{
		client: e.client,
		ctx:    e.ctx,
		input:  buildDynamoScanInput(input),
	}
	allItems, err := executePagedItems(input.Limit, pager)
	if err != nil {
		return err
	}

	// Unmarshal the results into dest
	return UnmarshalItems(allItems, dest)
}

// ExecuteGetItem implements GetItemExecutor.ExecuteGetItem.
func (e *MainExecutor) ExecuteGetItem(input *core.CompiledQuery, key map[string]types.AttributeValue, dest any) error {
	if input == nil {
		return fmt.Errorf("compiled query cannot be nil")
	}
	if len(key) == 0 {
		return fmt.Errorf("key cannot be empty")
	}

	getInput := &dynamodb.GetItemInput{
		TableName: aws.String(input.TableName),
		Key:       key,
	}

	if input.ProjectionExpression != "" {
		getInput.ProjectionExpression = aws.String(input.ProjectionExpression)
	}
	if len(input.ExpressionAttributeNames) > 0 {
		getInput.ExpressionAttributeNames = input.ExpressionAttributeNames
	}
	if input.ConsistentRead != nil {
		getInput.ConsistentRead = input.ConsistentRead
	}

	output, err := e.client.GetItem(e.ctx, getInput)
	if err != nil {
		return fmt.Errorf("failed to execute get item: %w", err)
	}
	if output.Item == nil {
		return customerrors.ErrItemNotFound
	}

	if rawDest, ok := dest.(*map[string]types.AttributeValue); ok && rawDest != nil {
		*rawDest = output.Item
		return nil
	}

	return UnmarshalItem(output.Item, dest)
}

func applyCompiledQueryWriteConditions(
	input *core.CompiledQuery,
	conditionExpression **string,
	expressionAttributeNames *map[string]string,
	expressionAttributeValues *map[string]types.AttributeValue,
) {
	if input.ConditionExpression != "" {
		*conditionExpression = &input.ConditionExpression
	}
	if len(input.ExpressionAttributeNames) > 0 {
		*expressionAttributeNames = input.ExpressionAttributeNames
	}
	if len(input.ExpressionAttributeValues) > 0 {
		*expressionAttributeValues = input.ExpressionAttributeValues
	}
}

type conditionalWriteRequest interface {
	applyCompiledQuery(input *core.CompiledQuery)
	setAttributes(attributes map[string]types.AttributeValue)
	execute(ctx context.Context, client DynamoDBAPI) error
}

type putItemRequest struct {
	input *dynamodb.PutItemInput
}

func newPutItemRequest(tableName *string) conditionalWriteRequest {
	return &putItemRequest{
		input: &dynamodb.PutItemInput{
			TableName: tableName,
		},
	}
}

func (r *putItemRequest) applyCompiledQuery(input *core.CompiledQuery) {
	applyCompiledQueryWriteConditions(input, &r.input.ConditionExpression, &r.input.ExpressionAttributeNames, &r.input.ExpressionAttributeValues)
}

func (r *putItemRequest) setAttributes(attributes map[string]types.AttributeValue) {
	r.input.Item = attributes
}

func (r *putItemRequest) execute(ctx context.Context, client DynamoDBAPI) error {
	_, err := client.PutItem(ctx, r.input)
	return err
}

type deleteItemRequest struct {
	input *dynamodb.DeleteItemInput
}

func newDeleteItemRequest(tableName *string) conditionalWriteRequest {
	return &deleteItemRequest{
		input: &dynamodb.DeleteItemInput{
			TableName: tableName,
		},
	}
}

func (r *deleteItemRequest) applyCompiledQuery(input *core.CompiledQuery) {
	applyCompiledQueryWriteConditions(input, &r.input.ConditionExpression, &r.input.ExpressionAttributeNames, &r.input.ExpressionAttributeValues)
}

func (r *deleteItemRequest) setAttributes(attributes map[string]types.AttributeValue) {
	r.input.Key = attributes
}

func (r *deleteItemRequest) execute(ctx context.Context, client DynamoDBAPI) error {
	_, err := client.DeleteItem(ctx, r.input)
	return err
}

func (e *MainExecutor) executeConditionalWrite(
	input *core.CompiledQuery,
	attributes map[string]types.AttributeValue,
	emptyWhat string,
	operation string,
	exec func(context.Context, *core.CompiledQuery, map[string]types.AttributeValue) error,
) error {
	if input == nil {
		return fmt.Errorf("compiled query cannot be nil")
	}

	if len(attributes) == 0 {
		return fmt.Errorf("%s cannot be empty", emptyWhat)
	}

	err := exec(e.ctx, input, attributes)
	if err == nil {
		return nil
	}

	if isConditionalCheckFailed(err) {
		return fmt.Errorf("%w: %v", customerrors.ErrConditionFailed, err)
	}
	return fmt.Errorf("failed to %s item: %w", operation, err)
}

func (e *MainExecutor) executeConditionalWriteRequest(
	input *core.CompiledQuery,
	attributes map[string]types.AttributeValue,
	emptyWhat string,
	operation string,
	newRequest func(*string) conditionalWriteRequest,
) error {
	return e.executeConditionalWrite(input, attributes, emptyWhat, operation, func(ctx context.Context, input *core.CompiledQuery, attributes map[string]types.AttributeValue) error {
		req := newRequest(&input.TableName)
		req.setAttributes(attributes)
		req.applyCompiledQuery(input)
		return req.execute(ctx, e.client)
	})
}

// ExecutePutItem implements PutItemExecutor.ExecutePutItem
func (e *MainExecutor) ExecutePutItem(input *core.CompiledQuery, item map[string]types.AttributeValue) error {
	return e.executeConditionalWriteRequest(input, item, "item", "put", newPutItemRequest)
}

// ExecuteUpdateItem implements UpdateItemExecutor.ExecuteUpdateItem
func (e *MainExecutor) ExecuteUpdateItem(input *core.CompiledQuery, key map[string]types.AttributeValue) error {
	// Use the UpdateExecutor from core package
	updateExecutor := core.NewUpdateExecutor(e.client, e.ctx)
	return updateExecutor.ExecuteUpdateItem(input, key)
}

// ExecuteUpdateItemWithResult implements UpdateItemWithResultExecutor.ExecuteUpdateItemWithResult
func (e *MainExecutor) ExecuteUpdateItemWithResult(input *core.CompiledQuery, key map[string]types.AttributeValue) (*core.UpdateResult, error) {
	// Use the UpdateExecutor from core package
	updateExecutor := core.NewUpdateExecutor(e.client, e.ctx)
	return updateExecutor.ExecuteUpdateItemWithResult(input, key)
}

// ExecuteDeleteItem implements DeleteItemExecutor.ExecuteDeleteItem
func (e *MainExecutor) ExecuteDeleteItem(input *core.CompiledQuery, key map[string]types.AttributeValue) error {
	return e.executeConditionalWriteRequest(input, key, "key", "delete", newDeleteItemRequest)
}

// ExecuteQueryWithPagination implements PaginatedQueryExecutor.ExecuteQueryWithPagination
func (e *MainExecutor) ExecuteQueryWithPagination(input *core.CompiledQuery, dest any) (*QueryResult, error) {
	if input == nil {
		return nil, fmt.Errorf("compiled query cannot be nil")
	}

	// Build QueryInput
	queryInput := &dynamodb.QueryInput{
		TableName: &input.TableName,
	}

	// Set index name if specified
	if input.IndexName != "" {
		queryInput.IndexName = &input.IndexName
	}

	// Set key condition expression
	if input.KeyConditionExpression != "" {
		queryInput.KeyConditionExpression = &input.KeyConditionExpression
	}

	// Set filter expression
	if input.FilterExpression != "" {
		queryInput.FilterExpression = &input.FilterExpression
	}

	// Set projection expression
	if input.ProjectionExpression != "" {
		queryInput.ProjectionExpression = &input.ProjectionExpression
	}

	// Set expression attribute names
	if len(input.ExpressionAttributeNames) > 0 {
		queryInput.ExpressionAttributeNames = input.ExpressionAttributeNames
	}

	// Set expression attribute values
	if len(input.ExpressionAttributeValues) > 0 {
		queryInput.ExpressionAttributeValues = input.ExpressionAttributeValues
	}

	// Set limit
	if input.Limit != nil {
		queryInput.Limit = input.Limit
	}

	// Set exclusive start key
	if len(input.ExclusiveStartKey) > 0 {
		queryInput.ExclusiveStartKey = input.ExclusiveStartKey
	}

	// Set scan index forward
	if input.ScanIndexForward != nil {
		queryInput.ScanIndexForward = input.ScanIndexForward
	}

	// Set consistent read
	if input.ConsistentRead != nil {
		queryInput.ConsistentRead = input.ConsistentRead
	}

	// Execute the query (single page only for pagination)
	output, err := e.client.Query(e.ctx, queryInput)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}

	// Unmarshal the results into dest
	if err := UnmarshalItems(output.Items, dest); err != nil {
		return nil, err
	}

	// Return the result with pagination info
	return &QueryResult{
		Items:            output.Items,
		Count:            int64(len(output.Items)),
		ScannedCount:     int64(output.ScannedCount),
		LastEvaluatedKey: output.LastEvaluatedKey,
	}, nil
}

func isConditionalCheckFailed(err error) bool {
	if err == nil {
		return false
	}
	var condErr *types.ConditionalCheckFailedException
	if errors.As(err, &condErr) {
		return true
	}
	return strings.Contains(err.Error(), "ConditionalCheckFailed")
}

// ExecuteScanWithPagination implements PaginatedQueryExecutor.ExecuteScanWithPagination
func (e *MainExecutor) ExecuteScanWithPagination(input *core.CompiledQuery, dest any) (*ScanResult, error) {
	if input == nil {
		return nil, fmt.Errorf("compiled query cannot be nil")
	}

	// Build ScanInput
	scanInput := &dynamodb.ScanInput{
		TableName: &input.TableName,
	}

	// Set index name if specified
	if input.IndexName != "" {
		scanInput.IndexName = &input.IndexName
	}

	// Set filter expression
	if input.FilterExpression != "" {
		scanInput.FilterExpression = &input.FilterExpression
	}

	// Set projection expression
	if input.ProjectionExpression != "" {
		scanInput.ProjectionExpression = &input.ProjectionExpression
	}

	// Set expression attribute names
	if len(input.ExpressionAttributeNames) > 0 {
		scanInput.ExpressionAttributeNames = input.ExpressionAttributeNames
	}

	// Set expression attribute values
	if len(input.ExpressionAttributeValues) > 0 {
		scanInput.ExpressionAttributeValues = input.ExpressionAttributeValues
	}

	// Set limit
	if input.Limit != nil {
		scanInput.Limit = input.Limit
	}

	// Set exclusive start key
	if len(input.ExclusiveStartKey) > 0 {
		scanInput.ExclusiveStartKey = input.ExclusiveStartKey
	}

	// Set segment and total segments for parallel scan
	if input.Segment != nil {
		scanInput.Segment = input.Segment
	}
	if input.TotalSegments != nil {
		scanInput.TotalSegments = input.TotalSegments
	}

	// Set consistent read
	if input.ConsistentRead != nil {
		scanInput.ConsistentRead = input.ConsistentRead
	}

	// Execute the scan (single page only for pagination)
	output, err := e.client.Scan(e.ctx, scanInput)
	if err != nil {
		return nil, fmt.Errorf("failed to execute scan: %w", err)
	}

	// Unmarshal the results into dest
	if err := UnmarshalItems(output.Items, dest); err != nil {
		return nil, err
	}

	// Return the result with pagination info
	return &ScanResult{
		Items:            output.Items,
		Count:            int64(len(output.Items)),
		ScannedCount:     int64(output.ScannedCount),
		LastEvaluatedKey: output.LastEvaluatedKey,
	}, nil
}

// ExecuteBatchGet implements BatchExecutor.ExecuteBatchGet
func (e *MainExecutor) ExecuteBatchGet(input *CompiledBatchGet, opts *core.BatchGetOptions) ([]map[string]types.AttributeValue, error) {
	if input == nil {
		return nil, fmt.Errorf("compiled batch get cannot be nil")
	}

	if len(input.Keys) == 0 {
		return nil, nil
	}

	userProvidedOpts := opts != nil

	if opts == nil {
		opts = core.DefaultBatchGetOptions()
	} else {
		opts = opts.Clone()
	}

	if opts.RetryPolicy != nil {
		opts.RetryPolicy = opts.RetryPolicy.Clone()
	} else if !userProvidedOpts {
		opts.RetryPolicy = core.DefaultRetryPolicy()
	}

	requestItems := map[string]types.KeysAndAttributes{
		input.TableName: buildKeysAndAttributes(input),
	}

	var collected []map[string]types.AttributeValue
	retryAttempt := 0

	for len(requestItems) > 0 {
		output, err := e.client.BatchGetItem(e.ctx, &dynamodb.BatchGetItemInput{
			RequestItems: requestItems,
		})
		if err != nil {
			return collected, fmt.Errorf("failed to batch get items: %w", err)
		}

		if items, exists := output.Responses[input.TableName]; exists {
			collected = append(collected, items...)
		}

		unprocessed := output.UnprocessedKeys
		if len(unprocessed) == 0 {
			break
		}

		remaining := countUnprocessedKeys(unprocessed)
		if remaining == 0 {
			break
		}

		if opts.RetryPolicy == nil || retryAttempt >= opts.RetryPolicy.MaxRetries {
			return collected, fmt.Errorf("batch get exhausted retries with %d unprocessed keys", remaining)
		}

		delay := calculateBatchRetryDelay(opts.RetryPolicy, retryAttempt)
		retryAttempt++
		time.Sleep(delay)

		requestItems = unprocessed
	}

	return collected, nil
}

func cryptoFloat64() (float64, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0, err
	}

	// Use the top 53 bits (IEEE-754 float64 mantissa) for a uniform [0,1) fraction.
	u := binary.BigEndian.Uint64(b[:]) >> 11
	return float64(u) / (1 << 53), nil
}

func buildKeysAndAttributes(input *CompiledBatchGet) types.KeysAndAttributes {
	kaa := types.KeysAndAttributes{
		Keys: input.Keys,
	}

	if input.ProjectionExpression != "" {
		expr := input.ProjectionExpression
		kaa.ProjectionExpression = &expr
	}

	if len(input.ExpressionAttributeNames) > 0 {
		kaa.ExpressionAttributeNames = input.ExpressionAttributeNames
	}

	if input.ConsistentRead {
		consistent := input.ConsistentRead
		kaa.ConsistentRead = &consistent
	}

	return kaa
}

func countUnprocessedKeys(unprocessed map[string]types.KeysAndAttributes) int {
	if len(unprocessed) == 0 {
		return 0
	}
	total := 0
	for _, entry := range unprocessed {
		total += len(entry.Keys)
	}
	return total
}

func calculateBatchRetryDelay(policy *core.RetryPolicy, attempt int) time.Duration {
	if policy == nil {
		return 0
	}

	delay := policy.InitialDelay
	if delay <= 0 {
		delay = 50 * time.Millisecond
	}

	if attempt > 0 {
		delay = time.Duration(float64(delay) * math.Pow(policy.BackoffFactor, float64(attempt)))
	}

	if policy.MaxDelay > 0 && delay > policy.MaxDelay {
		delay = policy.MaxDelay
	}

	if policy.Jitter > 0 {
		if r, err := cryptoFloat64(); err == nil {
			offset := (r*2 - 1) * policy.Jitter * float64(delay)
			delay += time.Duration(offset)
		}
		if delay < 0 {
			delay = policy.InitialDelay
			if delay <= 0 {
				delay = 50 * time.Millisecond
			}
		}
	}

	return delay
}

// ExecuteBatchWrite implements BatchExecutor.ExecuteBatchWrite
func (e *MainExecutor) ExecuteBatchWrite(input *CompiledBatchWrite) error {
	if input == nil {
		return fmt.Errorf("compiled batch write cannot be nil")
	}

	if len(input.Items) == 0 {
		return nil // No items to write
	}

	// Process items in batches of 25 (DynamoDB limit)
	const batchSize = 25

	for i := 0; i < len(input.Items); i += batchSize {
		end := i + batchSize
		if end > len(input.Items) {
			end = len(input.Items)
		}

		// Build write requests for this batch
		writeRequests := make([]types.WriteRequest, 0, end-i)
		for j := i; j < end; j++ {
			writeRequests = append(writeRequests, types.WriteRequest{
				PutRequest: &types.PutRequest{
					Item: input.Items[j],
				},
			})
		}

		// Build BatchWriteItem input
		batchWriteInput := &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]types.WriteRequest{
				input.TableName: writeRequests,
			},
		}

		// Execute batch write with retry for unprocessed items
		for {
			output, err := e.client.BatchWriteItem(e.ctx, batchWriteInput)
			if err != nil {
				return fmt.Errorf("failed to batch write items: %w", err)
			}

			// Check for unprocessed items
			if len(output.UnprocessedItems) == 0 {
				break
			}

			// Retry unprocessed items
			batchWriteInput.RequestItems = output.UnprocessedItems
		}
	}

	return nil
}

// UnmarshalItems unmarshals DynamoDB items into the destination.
// This function is exported for use with DynamoDB streams and other external data sources.
func UnmarshalItems(items []map[string]types.AttributeValue, dest any) error {
	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr {
		return fmt.Errorf("destination must be a pointer")
	}

	destElem := destValue.Elem()

	// Handle single item result
	if destElem.Kind() != reflect.Slice {
		if len(items) == 0 {
			return fmt.Errorf("no items found")
		}
		// For single item, unmarshal the first item
		return UnmarshalItem(items[0], dest)
	}

	// Handle slice result
	sliceType := destElem.Type()
	itemType := sliceType.Elem()

	// Create a new slice with the appropriate capacity
	newSlice := reflect.MakeSlice(sliceType, 0, len(items))

	for _, item := range items {
		// Create a new instance of the item type
		newItem := reflect.New(itemType)
		if itemType.Kind() == reflect.Ptr {
			newItem = reflect.New(itemType.Elem())
		}

		// Unmarshal the item
		if err := UnmarshalItem(item, newItem.Interface()); err != nil {
			return fmt.Errorf("failed to unmarshal item: %w", err)
		}

		// Append to slice
		if itemType.Kind() == reflect.Ptr {
			newSlice = reflect.Append(newSlice, newItem)
		} else {
			newSlice = reflect.Append(newSlice, newItem.Elem())
		}
	}

	// Set the result
	destElem.Set(newSlice)
	return nil
}

// UnmarshalItem unmarshals a single DynamoDB item into a Go struct.
// This function respects both "dynamodb" and "theorydb" struct tags.
func UnmarshalItem(item map[string]types.AttributeValue, dest any) error {
	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr || destValue.IsNil() {
		return fmt.Errorf("destination must be a pointer")
	}

	destElem := destValue.Elem()
	if destElem.Kind() != reflect.Struct {
		return fmt.Errorf("destination must be a pointer to a struct")
	}
	destType := destElem.Type()
	convention := detectNamingConvention(destType)

	// For each field in the struct
	for i := 0; i < destType.NumField(); i++ {
		field := destType.Field(i)
		fieldValue := destElem.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Determine the DynamoDB attribute name for this field.
		dynamodbTag := field.Tag.Get("dynamodb")
		if dynamodbTag == "-" {
			continue
		}

		attrName := ""
		if dynamodbTag != "" {
			attrName = parseAttributeName(dynamodbTag)
			if attrName == "" {
				attrName = field.Name
			}
		} else {
			var skip bool
			attrName, skip = naming.ResolveAttrNameWithConvention(field, convention)
			if skip || attrName == "" {
				continue
			}
		}

		// Get the attribute value
		if av, exists := item[attrName]; exists {
			if fieldHasEncryptedTag(field) && looksLikeEncryptedEnvelope(av) {
				return &customerrors.EncryptedFieldError{
					Operation: "decrypt",
					Field:     field.Name,
					Err:       customerrors.ErrEncryptionNotConfigured,
				}
			}
			if err := unmarshalAttributeValue(av, fieldValue); err != nil {
				return fmt.Errorf("failed to unmarshal field %s: %w", field.Name, err)
			}
		}
	}

	return nil
}

// unmarshalAttributeValue unmarshals a DynamoDB attribute value into a reflect.Value
func unmarshalAttributeValue(av types.AttributeValue, dest reflect.Value) error {
	if !dest.CanSet() {
		return fmt.Errorf("cannot set value")
	}

	if dest.Kind() == reflect.Ptr {
		return unmarshalPointerAttributeValue(av, dest)
	}

	if dest.Kind() == reflect.Interface && dest.Type().NumMethod() == 0 {
		return unmarshalAnyAttributeValue(av, dest)
	}

	switch v := av.(type) {
	case *types.AttributeValueMemberS:
		return unmarshalStringAttribute(v.Value, dest)
	case *types.AttributeValueMemberN:
		return unmarshalNumberAttribute(v.Value, dest)
	case *types.AttributeValueMemberBOOL:
		return unmarshalBoolAttribute(v.Value, dest)
	case *types.AttributeValueMemberNULL:
		dest.Set(reflect.Zero(dest.Type()))
		return nil
	case *types.AttributeValueMemberL:
		return unmarshalListAttribute(v.Value, dest)
	case *types.AttributeValueMemberM:
		return unmarshalMapAttribute(v.Value, dest)
	case *types.AttributeValueMemberSS:
		return unmarshalStringSetAttribute(v.Value, dest)
	case *types.AttributeValueMemberNS:
		return unmarshalNumberSetAttribute(v.Value, dest)
	case *types.AttributeValueMemberBS:
		return unmarshalBinarySetAttribute(v.Value, dest)
	case *types.AttributeValueMemberB:
		return unmarshalBinaryAttribute(v.Value, dest)
	default:
		return fmt.Errorf("unsupported attribute value type: %T", av)
	}
}

func unmarshalPointerAttributeValue(av types.AttributeValue, dest reflect.Value) error {
	if av == nil {
		dest.Set(reflect.Zero(dest.Type()))
		return nil
	}
	if _, ok := av.(*types.AttributeValueMemberNULL); ok {
		dest.Set(reflect.Zero(dest.Type()))
		return nil
	}
	if dest.IsNil() {
		dest.Set(reflect.New(dest.Type().Elem()))
	}
	return unmarshalAttributeValue(av, dest.Elem())
}

func unmarshalAnyAttributeValue(av types.AttributeValue, dest reflect.Value) error {
	value, err := attributeValueToInterface(av)
	if err != nil {
		return err
	}
	if value == nil {
		dest.Set(reflect.Zero(dest.Type()))
		return nil
	}
	dest.Set(reflect.ValueOf(value))
	return nil
}

func unmarshalStringAttribute(value string, dest reflect.Value) error {
	switch dest.Kind() {
	case reflect.String:
		dest.SetString(value)
		return nil
	case reflect.Struct:
		return unmarshalStringToStruct(value, dest)
	case reflect.Map, reflect.Slice:
		return unmarshalJSONString(value, dest)
	default:
		return fmt.Errorf("cannot unmarshal string into %v", dest.Kind())
	}
}

func unmarshalStringToStruct(value string, dest reflect.Value) error {
	if dest.Type() == reflect.TypeOf(time.Time{}) {
		t, err := parseTimeString(value)
		if err != nil {
			return err
		}
		dest.Set(reflect.ValueOf(t))
		return nil
	}
	return unmarshalJSONString(value, dest)
}

func parseTimeString(value string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, value)
	if err == nil {
		return t, nil
	}

	t, err = time.Parse(time.RFC3339Nano, value)
	if err == nil {
		return t, nil
	}

	unix, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse time from string %q: %w", value, err)
	}
	return time.Unix(unix, 0), nil
}

func unmarshalJSONString(value string, dest reflect.Value) error {
	if !dest.CanAddr() {
		return fmt.Errorf("cannot unmarshal JSON string into %v", dest.Kind())
	}
	if err := json.Unmarshal([]byte(value), dest.Addr().Interface()); err != nil {
		return fmt.Errorf("failed to unmarshal JSON string: %w", err)
	}
	return nil
}

func unmarshalNumberAttribute(value string, dest reflect.Value) error {
	switch dest.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return err
		}
		dest.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return err
		}
		dest.SetUint(n)
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return err
		}
		dest.SetFloat(f)
	}
	return nil
}

func unmarshalBoolAttribute(value bool, dest reflect.Value) error {
	if dest.Kind() != reflect.Bool {
		return fmt.Errorf("cannot unmarshal bool into %v", dest.Kind())
	}
	dest.SetBool(value)
	return nil
}

func unmarshalListAttribute(values []types.AttributeValue, dest reflect.Value) error {
	if dest.Kind() != reflect.Slice {
		return fmt.Errorf("cannot unmarshal list into non-slice type")
	}

	sliceType := dest.Type()
	newSlice := reflect.MakeSlice(sliceType, len(values), len(values))
	for i, item := range values {
		if err := unmarshalAttributeValue(item, newSlice.Index(i)); err != nil {
			return err
		}
	}
	dest.Set(newSlice)
	return nil
}

func unmarshalMapAttribute(values map[string]types.AttributeValue, dest reflect.Value) error {
	switch dest.Kind() {
	case reflect.Map:
		return unmarshalMapIntoMap(values, dest)
	case reflect.Struct:
		return unmarshalMapIntoStruct(values, dest)
	default:
		return nil
	}
}

func unmarshalMapIntoMap(values map[string]types.AttributeValue, dest reflect.Value) error {
	mapType := dest.Type()
	keyType := mapType.Key()
	if keyType.Kind() != reflect.String {
		return fmt.Errorf("cannot unmarshal map into %v", dest.Kind())
	}

	elemType := mapType.Elem()
	newMap := reflect.MakeMap(mapType)
	for key, mapVal := range values {
		keyValue := reflect.New(keyType).Elem()
		keyValue.SetString(key)

		elemValue := reflect.New(elemType).Elem()
		if err := unmarshalAttributeValue(mapVal, elemValue); err != nil {
			return err
		}
		newMap.SetMapIndex(keyValue, elemValue)
	}
	dest.Set(newMap)
	return nil
}

func unmarshalMapIntoStruct(values map[string]types.AttributeValue, dest reflect.Value) error {
	for key, structVal := range values {
		field := dest.FieldByName(key)
		if !field.IsValid() || !field.CanSet() {
			continue
		}
		if err := unmarshalAttributeValue(structVal, field); err != nil {
			return err
		}
	}
	return nil
}

func unmarshalStringSetAttribute(values []string, dest reflect.Value) error {
	if dest.Kind() != reflect.Slice || dest.Type().Elem().Kind() != reflect.String {
		return fmt.Errorf("cannot unmarshal string set into %v", dest.Kind())
	}

	slice := reflect.MakeSlice(dest.Type(), len(values), len(values))
	for i, value := range values {
		slice.Index(i).SetString(value)
	}
	dest.Set(slice)
	return nil
}

func unmarshalNumberSetAttribute(values []string, dest reflect.Value) error {
	if dest.Kind() != reflect.Slice {
		return fmt.Errorf("cannot unmarshal number set into %v", dest.Kind())
	}

	slice := reflect.MakeSlice(dest.Type(), len(values), len(values))
	for i, value := range values {
		if err := unmarshalNumberAttribute(value, slice.Index(i)); err != nil {
			return err
		}
	}
	dest.Set(slice)
	return nil
}

func unmarshalBinarySetAttribute(values [][]byte, dest reflect.Value) error {
	if dest.Kind() != reflect.Slice || dest.Type().Elem().Kind() != reflect.Slice || dest.Type().Elem().Elem().Kind() != reflect.Uint8 {
		return fmt.Errorf("cannot unmarshal binary set into %v", dest.Kind())
	}

	slice := reflect.MakeSlice(dest.Type(), len(values), len(values))
	for i, value := range values {
		slice.Index(i).SetBytes(value)
	}
	dest.Set(slice)
	return nil
}

func unmarshalBinaryAttribute(value []byte, dest reflect.Value) error {
	if dest.Kind() != reflect.Slice || dest.Type().Elem().Kind() != reflect.Uint8 {
		return fmt.Errorf("cannot unmarshal binary into %v", dest.Kind())
	}
	dest.SetBytes(value)
	return nil
}

// parseAttributeName extracts the attribute name from a DynamoDB tag, ignoring modifiers like omitempty
func parseAttributeName(tag string) string {
	// Split by comma to separate attribute name from modifiers
	parts := strings.Split(tag, ",")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}

func detectNamingConvention(modelType reflect.Type) naming.Convention {
	for i := 0; i < modelType.NumField(); i++ {
		field := modelType.Field(i)
		tag := field.Tag.Get("theorydb")
		if tag == "" {
			continue
		}

		parts := strings.Split(tag, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if !strings.HasPrefix(part, "naming:") {
				continue
			}

			convention := strings.TrimPrefix(part, "naming:")
			switch convention {
			case "snake_case":
				return naming.SnakeCase
			case "camel_case", "camelCase":
				return naming.CamelCase
			}
		}
	}

	return naming.CamelCase
}

func fieldHasEncryptedTag(field reflect.StructField) bool {
	tag := field.Tag.Get("theorydb")
	if tag == "" || tag == "-" {
		return false
	}

	parts := strings.Split(tag, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if part == "encrypted" || strings.HasPrefix(part, "encrypted:") {
			return true
		}
	}

	return false
}

func looksLikeEncryptedEnvelope(av types.AttributeValue) bool {
	env, ok := av.(*types.AttributeValueMemberM)
	if !ok || env == nil || len(env.Value) == 0 {
		return false
	}

	v, ok := env.Value["v"].(*types.AttributeValueMemberN)
	if !ok || v == nil || v.Value == "" {
		return false
	}
	edk, ok := env.Value["edk"].(*types.AttributeValueMemberB)
	if !ok || edk == nil || len(edk.Value) == 0 {
		return false
	}
	nonce, ok := env.Value["nonce"].(*types.AttributeValueMemberB)
	if !ok || nonce == nil || len(nonce.Value) == 0 {
		return false
	}
	ct, ok := env.Value["ct"].(*types.AttributeValueMemberB)
	if !ok || ct == nil {
		return false
	}

	return true
}

// attributeValueToInterface converts a DynamoDB AttributeValue to a Go interface{} value.
func attributeValueToInterface(av types.AttributeValue) (interface{}, error) {
	switch v := av.(type) {
	case *types.AttributeValueMemberS:
		return v.Value, nil
	case *types.AttributeValueMemberN:
		return parseNumberToInterface(v.Value)
	case *types.AttributeValueMemberBOOL:
		return v.Value, nil
	case *types.AttributeValueMemberNULL:
		return nil, nil
	case *types.AttributeValueMemberL:
		return attributeValueListToInterface(v.Value)
	case *types.AttributeValueMemberM:
		return attributeValueMapToInterface(v.Value)
	case *types.AttributeValueMemberSS:
		return v.Value, nil
	case *types.AttributeValueMemberNS:
		return attributeValueNumberSetToFloat64(v.Value)
	case *types.AttributeValueMemberBS:
		return v.Value, nil
	case *types.AttributeValueMemberB:
		return v.Value, nil
	default:
		return nil, fmt.Errorf("unsupported attribute value type: %T", av)
	}
}

func parseNumberToInterface(value string) (interface{}, error) {
	if intVal, err := strconv.ParseInt(value, 10, 64); err == nil {
		return intVal, nil
	}
	if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
		return floatVal, nil
	}
	return nil, fmt.Errorf("invalid number format: %s", value)
}

func attributeValueListToInterface(values []types.AttributeValue) ([]interface{}, error) {
	result := make([]interface{}, len(values))
	for i, item := range values {
		val, err := attributeValueToInterface(item)
		if err != nil {
			return nil, err
		}
		result[i] = val
	}
	return result, nil
}

func attributeValueMapToInterface(values map[string]types.AttributeValue) (map[string]interface{}, error) {
	result := make(map[string]interface{}, len(values))
	for k, val := range values {
		converted, err := attributeValueToInterface(val)
		if err != nil {
			return nil, err
		}
		result[k] = converted
	}
	return result, nil
}

func attributeValueNumberSetToFloat64(values []string) ([]float64, error) {
	result := make([]float64, len(values))
	for i, numStr := range values {
		f, err := strconv.ParseFloat(numStr, 64)
		if err != nil {
			return nil, err
		}
		result[i] = f
	}
	return result, nil
}

// Verify that MainExecutor implements all required interfaces
var (
	_ QueryExecutor                = (*MainExecutor)(nil)
	_ PutItemExecutor              = (*MainExecutor)(nil)
	_ UpdateItemExecutor           = (*MainExecutor)(nil)
	_ UpdateItemWithResultExecutor = (*MainExecutor)(nil)
	_ DeleteItemExecutor           = (*MainExecutor)(nil)
	_ PaginatedQueryExecutor       = (*MainExecutor)(nil)
	_ BatchExecutor                = (*MainExecutor)(nil)
)
