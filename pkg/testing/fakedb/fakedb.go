// Package fakedb provides a deterministic, state-backed DynamoDBAPI fake for
// consumer tests.
//
// The fake is a local testing aid. It is intentionally bounded to TableTheory's
// scenario-validated behavior and should not be treated as a DynamoDB emulator.
package fakedb

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/theory-cloud/tabletheory/v3/pkg/session"
)

var _ session.DynamoDBAPI = (*Fake)(nil)

// Fake is an in-memory implementation of the TableTheory DynamoDBAPI seam.
type Fake struct {
	tables map[string]*tableState
	mu     sync.RWMutex
}

type tableState struct {
	indexes map[string]indexState
	items   map[string]map[string]types.AttributeValue
	name    string
	pk      string
	sk      string
	ttlAttr string
}

type indexState struct {
	pk string
	sk string
}

// New returns an empty in-memory DynamoDB fake.
func New() *Fake {
	return &Fake{tables: make(map[string]*tableState)}
}

// Reset removes all fake tables and items.
func (f *Fake) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tables = make(map[string]*tableState)
}

// Seed stores items in a table, creating a PK/SK shaped table when needed.
func (f *Fake) Seed(tableName string, items ...map[string]types.AttributeValue) error {
	if tableName == "" {
		return errors.New("table name is required")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	table := f.tableOrDefaultLocked(tableName)
	for _, item := range items {
		key, err := table.itemKey(item)
		if err != nil {
			return err
		}
		table.items[key] = cloneItem(item)
	}
	return nil
}

// Items returns a deterministic snapshot of the table's stored items.
func (f *Fake) Items(tableName string) []map[string]types.AttributeValue {
	f.mu.RLock()
	defer f.mu.RUnlock()
	table := f.tables[tableName]
	if table == nil {
		return nil
	}
	items := make([]map[string]types.AttributeValue, 0, len(table.items))
	for _, item := range table.items {
		items = append(items, cloneItem(item))
	}
	sort.Slice(items, func(i, j int) bool {
		return itemSortKey(table, items[i]) < itemSortKey(table, items[j])
	})
	return items
}

func (f *Fake) CreateTable(_ context.Context, params *dynamodb.CreateTableInput, _ ...func(*dynamodb.Options)) (*dynamodb.CreateTableOutput, error) {
	if params == nil || params.TableName == nil || aws.ToString(params.TableName) == "" {
		return nil, errors.New("CreateTable requires TableName")
	}
	name := aws.ToString(params.TableName)

	f.mu.Lock()
	defer f.mu.Unlock()
	if _, exists := f.tables[name]; exists {
		return nil, &types.ResourceInUseException{Message: aws.String("table already exists")}
	}
	table := &tableState{
		name:    name,
		indexes: make(map[string]indexState),
		items:   make(map[string]map[string]types.AttributeValue),
	}
	applyKeySchema(table, params.KeySchema)
	for _, gsi := range params.GlobalSecondaryIndexes {
		if gsi.IndexName == nil {
			continue
		}
		table.indexes[aws.ToString(gsi.IndexName)] = indexFromSchema(gsi.KeySchema)
	}
	for _, lsi := range params.LocalSecondaryIndexes {
		if lsi.IndexName == nil {
			continue
		}
		table.indexes[aws.ToString(lsi.IndexName)] = indexFromSchema(lsi.KeySchema)
	}
	if table.pk == "" {
		table.pk = "PK"
	}
	f.tables[name] = table
	return &dynamodb.CreateTableOutput{TableDescription: tableDescription(table)}, nil
}

func (f *Fake) CreateBackup(_ context.Context, params *dynamodb.CreateBackupInput, _ ...func(*dynamodb.Options)) (*dynamodb.CreateBackupOutput, error) {
	if params == nil || params.TableName == nil {
		return nil, errors.New("CreateBackup requires TableName")
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	if _, err := f.requiredTableLocked(aws.ToString(params.TableName)); err != nil {
		return nil, err
	}
	return &dynamodb.CreateBackupOutput{
		BackupDetails: &types.BackupDetails{
			BackupArn:    aws.String("arn:aws:dynamodb:local:000000000000:table/" + aws.ToString(params.TableName) + "/backup/" + aws.ToString(params.BackupName)),
			BackupName:   params.BackupName,
			BackupStatus: types.BackupStatusAvailable,
			BackupType:   types.BackupTypeUser,
		},
	}, nil
}

func (f *Fake) DescribeTable(_ context.Context, params *dynamodb.DescribeTableInput, _ ...func(*dynamodb.Options)) (*dynamodb.DescribeTableOutput, error) {
	tableName := tableNameFrom(params)
	f.mu.RLock()
	defer f.mu.RUnlock()
	table := f.tables[tableName]
	if table == nil {
		return nil, resourceNotFound(tableName)
	}
	return &dynamodb.DescribeTableOutput{Table: tableDescription(table)}, nil
}

func (f *Fake) DeleteTable(_ context.Context, params *dynamodb.DeleteTableInput, _ ...func(*dynamodb.Options)) (*dynamodb.DeleteTableOutput, error) {
	tableName := tableNameFrom(params)
	f.mu.Lock()
	defer f.mu.Unlock()
	table := f.tables[tableName]
	if table == nil {
		return nil, resourceNotFound(tableName)
	}
	delete(f.tables, tableName)
	return &dynamodb.DeleteTableOutput{TableDescription: tableDescription(table)}, nil
}

func (f *Fake) ListTables(_ context.Context, params *dynamodb.ListTablesInput, _ ...func(*dynamodb.Options)) (*dynamodb.ListTablesOutput, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	names := make([]string, 0, len(f.tables))
	for name := range f.tables {
		names = append(names, name)
	}
	sort.Strings(names)
	if params != nil && params.Limit != nil && *params.Limit > 0 && int(*params.Limit) < len(names) {
		names = names[:int(*params.Limit)]
	}
	return &dynamodb.ListTablesOutput{TableNames: names}, nil
}

func (f *Fake) UpdateTable(_ context.Context, params *dynamodb.UpdateTableInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateTableOutput, error) {
	if params == nil || params.TableName == nil {
		return nil, errors.New("UpdateTable requires TableName")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	table, err := f.requiredTableLocked(aws.ToString(params.TableName))
	if err != nil {
		return nil, err
	}
	for _, gsiUpdate := range params.GlobalSecondaryIndexUpdates {
		if gsiUpdate.Create == nil || gsiUpdate.Create.IndexName == nil {
			continue
		}
		table.indexes[aws.ToString(gsiUpdate.Create.IndexName)] = indexFromSchema(gsiUpdate.Create.KeySchema)
	}
	return &dynamodb.UpdateTableOutput{TableDescription: tableDescription(table)}, nil
}

func (f *Fake) UpdateTimeToLive(_ context.Context, params *dynamodb.UpdateTimeToLiveInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateTimeToLiveOutput, error) {
	tableName := tableNameFrom(params)
	f.mu.Lock()
	defer f.mu.Unlock()
	table := f.tables[tableName]
	if table == nil {
		return nil, resourceNotFound(tableName)
	}
	if params.TimeToLiveSpecification != nil && params.TimeToLiveSpecification.AttributeName != nil {
		table.ttlAttr = aws.ToString(params.TimeToLiveSpecification.AttributeName)
	}
	return &dynamodb.UpdateTimeToLiveOutput{TimeToLiveSpecification: params.TimeToLiveSpecification}, nil
}

func (f *Fake) GetItem(_ context.Context, params *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	if params == nil {
		return nil, errors.New("GetItemInput is required")
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	table, err := f.requiredTableLocked(aws.ToString(params.TableName))
	if err != nil {
		return nil, err
	}
	key, err := table.keyFromKeyMap(params.Key)
	if err != nil {
		return nil, err
	}
	item := cloneItem(table.items[key])
	if len(item) > 0 {
		item = projectItem(item, params.ProjectionExpression, params.ExpressionAttributeNames)
	}
	return &dynamodb.GetItemOutput{Item: item}, nil
}

func (f *Fake) PutItem(_ context.Context, params *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	if params == nil {
		return nil, errors.New("PutItemInput is required")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	table := f.tableOrDefaultLocked(aws.ToString(params.TableName))
	key, err := table.itemKey(params.Item)
	if err != nil {
		return nil, err
	}
	existing := table.items[key]
	if !evalCondition(params.ConditionExpression, existing, params.ExpressionAttributeNames, params.ExpressionAttributeValues) {
		return nil, conditionalFailed()
	}
	table.items[key] = cloneItem(params.Item)
	return &dynamodb.PutItemOutput{}, nil
}

func (f *Fake) DeleteItem(_ context.Context, params *dynamodb.DeleteItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	if params == nil {
		return nil, errors.New("DeleteItemInput is required")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	table, err := f.requiredTableLocked(aws.ToString(params.TableName))
	if err != nil {
		return nil, err
	}
	key, err := table.keyFromKeyMap(params.Key)
	if err != nil {
		return nil, err
	}
	existing := table.items[key]
	if !evalCondition(params.ConditionExpression, existing, params.ExpressionAttributeNames, params.ExpressionAttributeValues) {
		return nil, conditionalFailed()
	}
	delete(table.items, key)
	return &dynamodb.DeleteItemOutput{}, nil
}

func (f *Fake) UpdateItem(_ context.Context, params *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	if params == nil {
		return nil, errors.New("UpdateItemInput is required")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	table := f.tableOrDefaultLocked(aws.ToString(params.TableName))
	key, err := table.keyFromKeyMap(params.Key)
	if err != nil {
		return nil, err
	}
	item := cloneItem(table.items[key])
	if !evalCondition(params.ConditionExpression, item, params.ExpressionAttributeNames, params.ExpressionAttributeValues) {
		return nil, conditionalFailed()
	}
	if item == nil {
		item = cloneItem(params.Key)
	}
	if err := applyUpdateExpression(item, params.UpdateExpression, params.ExpressionAttributeNames, params.ExpressionAttributeValues); err != nil {
		return nil, err
	}
	table.items[key] = item
	out := &dynamodb.UpdateItemOutput{}
	if params.ReturnValues == types.ReturnValueAllNew || params.ReturnValues == types.ReturnValueUpdatedNew {
		out.Attributes = cloneItem(item)
	}
	return out, nil
}

func (f *Fake) Query(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	if params == nil {
		return nil, errors.New("QueryInput is required")
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	table, err := f.requiredTableLocked(aws.ToString(params.TableName))
	if err != nil {
		return nil, err
	}
	items := f.readItemsLocked(table, readInput{
		indexName:                 aws.ToString(params.IndexName),
		keyConditionExpression:    params.KeyConditionExpression,
		filterExpression:          params.FilterExpression,
		projectionExpression:      params.ProjectionExpression,
		expressionAttributeNames:  params.ExpressionAttributeNames,
		expressionAttributeValues: params.ExpressionAttributeValues,
		limit:                     params.Limit,
		exclusiveStartKey:         params.ExclusiveStartKey,
		scanIndexForward:          params.ScanIndexForward,
		selectMode:                string(params.Select),
	})
	return &dynamodb.QueryOutput{
		Items:            items.items,
		Count:            safeInt32(len(items.items)),
		ScannedCount:     safeInt32(items.scanned),
		LastEvaluatedKey: items.lastKey,
	}, nil
}

func (f *Fake) Scan(_ context.Context, params *dynamodb.ScanInput, _ ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
	if params == nil {
		return nil, errors.New("ScanInput is required")
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	table, err := f.requiredTableLocked(aws.ToString(params.TableName))
	if err != nil {
		return nil, err
	}
	items := f.readItemsLocked(table, readInput{
		indexName:                 aws.ToString(params.IndexName),
		filterExpression:          params.FilterExpression,
		projectionExpression:      params.ProjectionExpression,
		expressionAttributeNames:  params.ExpressionAttributeNames,
		expressionAttributeValues: params.ExpressionAttributeValues,
		limit:                     params.Limit,
		exclusiveStartKey:         params.ExclusiveStartKey,
		selectMode:                string(params.Select),
	})
	return &dynamodb.ScanOutput{
		Items:            items.items,
		Count:            safeInt32(len(items.items)),
		ScannedCount:     safeInt32(items.scanned),
		LastEvaluatedKey: items.lastKey,
	}, nil
}

func (f *Fake) BatchGetItem(_ context.Context, params *dynamodb.BatchGetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.BatchGetItemOutput, error) {
	if params == nil {
		return nil, errors.New("BatchGetItemInput is required")
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	responses := make(map[string][]map[string]types.AttributeValue, len(params.RequestItems))
	for tableName, request := range params.RequestItems {
		table, err := f.requiredTableLocked(tableName)
		if err != nil {
			return nil, err
		}
		for _, keyMap := range request.Keys {
			key, err := table.keyFromKeyMap(keyMap)
			if err != nil {
				return nil, err
			}
			if item := table.items[key]; len(item) > 0 {
				responses[tableName] = append(responses[tableName], projectItem(cloneItem(item), request.ProjectionExpression, request.ExpressionAttributeNames))
			}
		}
	}
	return &dynamodb.BatchGetItemOutput{Responses: responses}, nil
}

func (f *Fake) BatchWriteItem(_ context.Context, params *dynamodb.BatchWriteItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.BatchWriteItemOutput, error) {
	if params == nil {
		return nil, errors.New("BatchWriteItemInput is required")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for tableName, writes := range params.RequestItems {
		table := f.tableOrDefaultLocked(tableName)
		for _, write := range writes {
			if write.PutRequest != nil {
				key, err := table.itemKey(write.PutRequest.Item)
				if err != nil {
					return nil, err
				}
				table.items[key] = cloneItem(write.PutRequest.Item)
			}
			if write.DeleteRequest != nil {
				key, err := table.keyFromKeyMap(write.DeleteRequest.Key)
				if err != nil {
					return nil, err
				}
				delete(table.items, key)
			}
		}
	}
	return &dynamodb.BatchWriteItemOutput{}, nil
}

func (f *Fake) TransactGetItems(_ context.Context, params *dynamodb.TransactGetItemsInput, _ ...func(*dynamodb.Options)) (*dynamodb.TransactGetItemsOutput, error) {
	if params == nil {
		return nil, errors.New("TransactGetItemsInput is required")
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	responses := make([]types.ItemResponse, 0, len(params.TransactItems))
	for _, op := range params.TransactItems {
		if op.Get == nil {
			responses = append(responses, types.ItemResponse{})
			continue
		}
		table, err := f.requiredTableLocked(aws.ToString(op.Get.TableName))
		if err != nil {
			return nil, err
		}
		key, err := table.keyFromKeyMap(op.Get.Key)
		if err != nil {
			return nil, err
		}
		item := projectItem(cloneItem(table.items[key]), op.Get.ProjectionExpression, op.Get.ExpressionAttributeNames)
		responses = append(responses, types.ItemResponse{Item: item})
	}
	return &dynamodb.TransactGetItemsOutput{Responses: responses}, nil
}

func (f *Fake) TransactWriteItems(_ context.Context, params *dynamodb.TransactWriteItemsInput, _ ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	if params == nil {
		return nil, errors.New("TransactWriteItemsInput is required")
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	snapshot := f.cloneTablesLocked()
	for i, op := range params.TransactItems {
		if err := applyTransactWrite(snapshot, op); err != nil {
			return nil, transactionCanceled(i, err)
		}
	}
	f.tables = snapshot
	return &dynamodb.TransactWriteItemsOutput{}, nil
}

type readInput struct {
	indexName                 string
	keyConditionExpression    *string
	filterExpression          *string
	projectionExpression      *string
	expressionAttributeNames  map[string]string
	expressionAttributeValues map[string]types.AttributeValue
	limit                     *int32
	exclusiveStartKey         map[string]types.AttributeValue
	scanIndexForward          *bool
	selectMode                string
}

type readOutput struct {
	lastKey map[string]types.AttributeValue
	items   []map[string]types.AttributeValue
	scanned int
}

const maxDynamoDBCount = int64(1<<31 - 1)

func safeInt32(n int) int32 {
	if int64(n) > maxDynamoDBCount {
		return int32(maxDynamoDBCount)
	}
	return int32(n) // #nosec G115 -- n is bounded above by maxDynamoDBCount before conversion.
}

func (f *Fake) readItemsLocked(table *tableState, input readInput) readOutput {
	items := collectReadItems(table, input)
	sortReadItems(table, input, items)
	items = applyExclusiveStartKey(table, input, items)
	scanned := len(items)

	lastKey, items := limitReadItems(table, input, items)
	if strings.EqualFold(input.selectMode, string(types.SelectCount)) {
		return readOutput{items: nil, lastKey: lastKey, scanned: scanned}
	}
	for i := range items {
		items[i] = projectItem(items[i], input.projectionExpression, input.expressionAttributeNames)
	}
	return readOutput{items: items, lastKey: lastKey, scanned: scanned}
}

func collectReadItems(table *tableState, input readInput) []map[string]types.AttributeValue {
	items := make([]map[string]types.AttributeValue, 0, len(table.items))
	for _, item := range table.items {
		if !evalCondition(input.keyConditionExpression, item, input.expressionAttributeNames, input.expressionAttributeValues) {
			continue
		}
		if !evalCondition(input.filterExpression, item, input.expressionAttributeNames, input.expressionAttributeValues) {
			continue
		}
		items = append(items, cloneItem(item))
	}
	return items
}

func sortReadItems(table *tableState, input readInput, items []map[string]types.AttributeValue) {
	sortAttr := table.sk
	if idx, ok := table.indexes[input.indexName]; ok && idx.sk != "" {
		sortAttr = idx.sk
	}
	sort.Slice(items, func(i, j int) bool {
		cmp := compareAV(items[i][sortAttr], items[j][sortAttr])
		if cmp == 0 {
			return itemSortKey(table, items[i]) < itemSortKey(table, items[j])
		}
		return cmp < 0
	})
	if input.scanIndexForward != nil && !aws.ToBool(input.scanIndexForward) {
		for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
			items[i], items[j] = items[j], items[i]
		}
	}
}

func applyExclusiveStartKey(table *tableState, input readInput, items []map[string]types.AttributeValue) []map[string]types.AttributeValue {
	if len(input.exclusiveStartKey) > 0 {
		startKey, err := table.keyFromKeyMap(input.exclusiveStartKey)
		if err == nil {
			for i, item := range items {
				if itemSortKey(table, item) == startKey {
					items = items[i+1:]
					break
				}
			}
		}
	}
	return items
}

func limitReadItems(table *tableState, input readInput, items []map[string]types.AttributeValue) (map[string]types.AttributeValue, []map[string]types.AttributeValue) {
	limit := 0
	if input.limit != nil && *input.limit > 0 {
		limit = int(*input.limit)
	}
	var lastKey map[string]types.AttributeValue
	if limit > 0 && len(items) > limit {
		lastKey = table.keyMap(items[limit-1])
		items = items[:limit]
	}
	return lastKey, items
}

func applyTransactWrite(tables map[string]*tableState, op types.TransactWriteItem) error {
	switch {
	case op.Put != nil:
		return applyTransactPut(tables, op.Put)
	case op.Update != nil:
		return applyTransactUpdate(tables, op.Update)
	case op.Delete != nil:
		return applyTransactDelete(tables, op.Delete)
	case op.ConditionCheck != nil:
		return applyTransactConditionCheck(tables, op.ConditionCheck)
	}
	return nil
}

func applyTransactPut(tables map[string]*tableState, op *types.Put) error {
	table := tableOrDefault(tables, aws.ToString(op.TableName))
	key, err := table.itemKey(op.Item)
	if err != nil {
		return err
	}
	existing := table.items[key]
	if !evalCondition(op.ConditionExpression, existing, op.ExpressionAttributeNames, op.ExpressionAttributeValues) {
		return conditionalFailed()
	}
	table.items[key] = cloneItem(op.Item)
	return nil
}

func applyTransactUpdate(tables map[string]*tableState, op *types.Update) error {
	table := tableOrDefault(tables, aws.ToString(op.TableName))
	key, err := table.keyFromKeyMap(op.Key)
	if err != nil {
		return err
	}
	item := cloneItem(table.items[key])
	if !evalCondition(op.ConditionExpression, item, op.ExpressionAttributeNames, op.ExpressionAttributeValues) {
		return conditionalFailed()
	}
	if item == nil {
		item = cloneItem(op.Key)
	}
	if err := applyUpdateExpression(item, op.UpdateExpression, op.ExpressionAttributeNames, op.ExpressionAttributeValues); err != nil {
		return err
	}
	table.items[key] = item
	return nil
}

func applyTransactDelete(tables map[string]*tableState, op *types.Delete) error {
	table, err := requiredTable(tables, aws.ToString(op.TableName))
	if err != nil {
		return err
	}
	key, err := table.keyFromKeyMap(op.Key)
	if err != nil {
		return err
	}
	existing := table.items[key]
	if !evalCondition(op.ConditionExpression, existing, op.ExpressionAttributeNames, op.ExpressionAttributeValues) {
		return conditionalFailed()
	}
	delete(table.items, key)
	return nil
}

func applyTransactConditionCheck(tables map[string]*tableState, op *types.ConditionCheck) error {
	table, err := requiredTable(tables, aws.ToString(op.TableName))
	if err != nil {
		return err
	}
	key, err := table.keyFromKeyMap(op.Key)
	if err != nil {
		return err
	}
	if !evalCondition(op.ConditionExpression, table.items[key], op.ExpressionAttributeNames, op.ExpressionAttributeValues) {
		return conditionalFailed()
	}
	return nil
}

func (f *Fake) tableOrDefaultLocked(name string) *tableState {
	return tableOrDefault(f.tables, name)
}

func tableOrDefault(tables map[string]*tableState, name string) *tableState {
	if name == "" {
		name = "default"
	}
	if table := tables[name]; table != nil {
		return table
	}
	table := &tableState{
		name:    name,
		pk:      "PK",
		sk:      "SK",
		indexes: make(map[string]indexState),
		items:   make(map[string]map[string]types.AttributeValue),
	}
	tables[name] = table
	return table
}

func (f *Fake) requiredTableLocked(name string) (*tableState, error) {
	return requiredTable(f.tables, name)
}

func requiredTable(tables map[string]*tableState, name string) (*tableState, error) {
	table := tables[name]
	if table == nil {
		return nil, resourceNotFound(name)
	}
	return table, nil
}

func (f *Fake) cloneTablesLocked() map[string]*tableState {
	out := make(map[string]*tableState, len(f.tables))
	for name, table := range f.tables {
		cloned := &tableState{
			name:    table.name,
			pk:      table.pk,
			sk:      table.sk,
			ttlAttr: table.ttlAttr,
			indexes: make(map[string]indexState, len(table.indexes)),
			items:   make(map[string]map[string]types.AttributeValue, len(table.items)),
		}
		for k, v := range table.indexes {
			cloned.indexes[k] = v
		}
		for k, item := range table.items {
			cloned.items[k] = cloneItem(item)
		}
		out[name] = cloned
	}
	return out
}

func applyKeySchema(table *tableState, schema []types.KeySchemaElement) {
	for _, key := range schema {
		switch key.KeyType {
		case types.KeyTypeHash:
			table.pk = aws.ToString(key.AttributeName)
		case types.KeyTypeRange:
			table.sk = aws.ToString(key.AttributeName)
		}
	}
}

func indexFromSchema(schema []types.KeySchemaElement) indexState {
	var index indexState
	for _, key := range schema {
		switch key.KeyType {
		case types.KeyTypeHash:
			index.pk = aws.ToString(key.AttributeName)
		case types.KeyTypeRange:
			index.sk = aws.ToString(key.AttributeName)
		}
	}
	return index
}

func tableDescription(table *tableState) *types.TableDescription {
	if table == nil {
		return nil
	}
	return &types.TableDescription{
		TableName:   aws.String(table.name),
		TableStatus: types.TableStatusActive,
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String(table.pk), KeyType: types.KeyTypeHash},
			{AttributeName: aws.String(table.sk), KeyType: types.KeyTypeRange},
		},
		ItemCount: aws.Int64(int64(len(table.items))),
	}
}

func tableNameFrom(input any) string {
	switch v := input.(type) {
	case *dynamodb.DescribeTableInput:
		return aws.ToString(v.TableName)
	case *dynamodb.DeleteTableInput:
		return aws.ToString(v.TableName)
	case *dynamodb.UpdateTimeToLiveInput:
		return aws.ToString(v.TableName)
	default:
		return ""
	}
}

func (t *tableState) keyFromKeyMap(key map[string]types.AttributeValue) (string, error) {
	if len(key) == 0 {
		return "", errors.New("key is required")
	}
	item := map[string]types.AttributeValue{t.pk: key[t.pk]}
	if t.sk != "" {
		item[t.sk] = key[t.sk]
	}
	return t.itemKey(item)
}

func (t *tableState) itemKey(item map[string]types.AttributeValue) (string, error) {
	if t.pk == "" {
		t.pk = "PK"
	}
	pk := item[t.pk]
	if pk == nil {
		return "", fmt.Errorf("missing partition key %s", t.pk)
	}
	key := avKeyComponent(pk)
	if t.sk != "" {
		sk := item[t.sk]
		if sk == nil {
			return "", fmt.Errorf("missing sort key %s", t.sk)
		}
		key += "|" + avKeyComponent(sk)
	}
	return key, nil
}

func (t *tableState) keyMap(item map[string]types.AttributeValue) map[string]types.AttributeValue {
	key := map[string]types.AttributeValue{t.pk: cloneAV(item[t.pk])}
	if t.sk != "" {
		key[t.sk] = cloneAV(item[t.sk])
	}
	return key
}

func itemSortKey(table *tableState, item map[string]types.AttributeValue) string {
	key, err := table.itemKey(item)
	if err != nil {
		return ""
	}
	return key
}

func evalCondition(expr *string, item map[string]types.AttributeValue, names map[string]string, values map[string]types.AttributeValue) bool {
	if expr == nil || strings.TrimSpace(*expr) == "" {
		return true
	}
	return evalExpr(strings.TrimSpace(*expr), item, names, values)
}

func evalExpr(expr string, item map[string]types.AttributeValue, names map[string]string, values map[string]types.AttributeValue) bool {
	expr = trimOuterParens(strings.TrimSpace(expr))
	if expr == "" {
		return true
	}
	if result, ok := evalLogicalExpr(expr, item, names, values); ok {
		return result
	}
	if result, ok := evalFunctionExpr(expr, item, names, values); ok {
		return result
	}
	if result, ok := evalRangeOrMembershipExpr(expr, item, names, values); ok {
		return result
	}
	if result, ok := evalComparisonExpr(expr, item, names, values); ok {
		return result
	}
	return false
}

func evalLogicalExpr(expr string, item map[string]types.AttributeValue, names map[string]string, values map[string]types.AttributeValue) (bool, bool) {
	if parts := splitLogical(expr, "OR"); len(parts) > 1 {
		for _, part := range parts {
			if evalExpr(part, item, names, values) {
				return true, true
			}
		}
		return false, true
	}
	if parts := splitLogical(expr, "AND"); len(parts) > 1 {
		for _, part := range parts {
			if !evalExpr(part, item, names, values) {
				return false, true
			}
		}
		return true, true
	}
	return false, false
}

func evalFunctionExpr(expr string, item map[string]types.AttributeValue, names map[string]string, values map[string]types.AttributeValue) (bool, bool) {
	lower := strings.ToLower(expr)
	if strings.HasPrefix(lower, "attribute_not_exists(") && strings.HasSuffix(expr, ")") {
		attr := resolveName(expr[len("attribute_not_exists("):len(expr)-1], names)
		return item == nil || item[attr] == nil, true
	}
	if strings.HasPrefix(lower, "attribute_exists(") && strings.HasSuffix(expr, ")") {
		attr := resolveName(expr[len("attribute_exists("):len(expr)-1], names)
		return item != nil && item[attr] != nil, true
	}
	if strings.HasPrefix(lower, "begins_with(") && strings.HasSuffix(expr, ")") {
		return evalTwoArgFunction(expr, "begins_with", item, names, values, strings.HasPrefix), true
	}
	if strings.HasPrefix(lower, "contains(") && strings.HasSuffix(expr, ")") {
		args := splitCSV(expr[len("contains(") : len(expr)-1])
		if len(args) != 2 {
			return false, true
		}
		return containsAV(item[resolveName(args[0], names)], values[strings.TrimSpace(args[1])]), true
	}
	return false, false
}

func evalTwoArgFunction(
	expr string,
	name string,
	item map[string]types.AttributeValue,
	names map[string]string,
	values map[string]types.AttributeValue,
	match func(string, string) bool,
) bool {
	args := splitCSV(expr[len(name)+1 : len(expr)-1])
	if len(args) != 2 {
		return false
	}
	have := stringValue(item[resolveName(args[0], names)])
	want := stringValue(values[strings.TrimSpace(args[1])])
	return match(have, want)
}

func evalRangeOrMembershipExpr(expr string, item map[string]types.AttributeValue, names map[string]string, values map[string]types.AttributeValue) (bool, bool) {
	if attr, lo, hi, ok := parseBetween(expr); ok {
		have := item[resolveName(attr, names)]
		return compareAV(have, values[lo]) >= 0 && compareAV(have, values[hi]) <= 0, true
	}
	if attr, valueRefs, ok := parseIn(expr); ok {
		have := item[resolveName(attr, names)]
		for _, ref := range valueRefs {
			if compareAV(have, values[ref]) == 0 {
				return true, true
			}
		}
		return false, true
	}
	return false, false
}

func evalComparisonExpr(expr string, item map[string]types.AttributeValue, names map[string]string, values map[string]types.AttributeValue) (bool, bool) {
	for _, op := range []string{"<>", ">=", "<=", "=", ">", "<"} {
		left, right, ok := splitComparison(expr, op)
		if !ok {
			continue
		}
		cmp := compareAV(item[resolveName(left, names)], values[strings.TrimSpace(right)])
		return compareByOperator(cmp, op), true
	}
	return false, false
}

func compareByOperator(cmp int, op string) bool {
	switch op {
	case "=":
		return cmp == 0
	case "<>":
		return cmp != 0
	case ">":
		return cmp > 0
	case ">=":
		return cmp >= 0
	case "<":
		return cmp < 0
	case "<=":
		return cmp <= 0
	default:
		return false
	}
}

func splitLogical(expr, op string) []string {
	target := " " + op + " "
	var parts []string
	depth := 0
	between := false
	start := 0
	upper := strings.ToUpper(expr)
	for i := 0; i < len(expr); i++ {
		switch expr[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		}
		if depth != 0 {
			continue
		}
		if strings.HasPrefix(upper[i:], " BETWEEN ") {
			between = true
			continue
		}
		if strings.HasPrefix(upper[i:], target) {
			if between && op == "AND" {
				between = false
				i += len(target) - 1
				continue
			}
			parts = append(parts, strings.TrimSpace(expr[start:i]))
			start = i + len(target)
			i += len(target) - 1
		}
	}
	if len(parts) == 0 {
		return nil
	}
	parts = append(parts, strings.TrimSpace(expr[start:]))
	return parts
}

func trimOuterParens(expr string) string {
	for strings.HasPrefix(expr, "(") && strings.HasSuffix(expr, ")") && parensBalanced(expr[1:len(expr)-1]) {
		expr = strings.TrimSpace(expr[1 : len(expr)-1])
	}
	return expr
}

func parensBalanced(expr string) bool {
	depth := 0
	for i := 0; i < len(expr); i++ {
		switch expr[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth < 0 {
				return false
			}
		}
	}
	return depth == 0
}

func splitComparison(expr, op string) (string, string, bool) {
	idx := strings.Index(expr, " "+op+" ")
	if idx < 0 {
		return "", "", false
	}
	return strings.TrimSpace(expr[:idx]), strings.TrimSpace(expr[idx+len(op)+2:]), true
}

func parseBetween(expr string) (string, string, string, bool) {
	parts := strings.Split(expr, " BETWEEN ")
	if len(parts) != 2 {
		return "", "", "", false
	}
	bounds := strings.Split(parts[1], " AND ")
	if len(bounds) != 2 {
		return "", "", "", false
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(bounds[0]), strings.TrimSpace(bounds[1]), true
}

func parseIn(expr string) (string, []string, bool) {
	idx := strings.Index(strings.ToUpper(expr), " IN ")
	if idx < 0 {
		return "", nil, false
	}
	attr := strings.TrimSpace(expr[:idx])
	rest := strings.TrimSpace(expr[idx+4:])
	if !strings.HasPrefix(rest, "(") || !strings.HasSuffix(rest, ")") {
		return "", nil, false
	}
	values := splitCSV(rest[1 : len(rest)-1])
	for i := range values {
		values[i] = strings.TrimSpace(values[i])
	}
	return attr, values, true
}

func applyUpdateExpression(item map[string]types.AttributeValue, expr *string, names map[string]string, values map[string]types.AttributeValue) error {
	if expr == nil || strings.TrimSpace(*expr) == "" {
		return nil
	}
	sections := updateSections(*expr)
	for action, body := range sections {
		if err := applyUpdateSection(item, action, body, names, values); err != nil {
			return err
		}
	}
	return nil
}

func applyUpdateSection(item map[string]types.AttributeValue, action, body string, names map[string]string, values map[string]types.AttributeValue) error {
	switch action {
	case "SET":
		return applySetSection(item, body, names, values)
	case "ADD":
		return applyAddSection(item, body, names, values)
	case "REMOVE":
		applyRemoveSection(item, body, names)
	case "DELETE":
		applyDeleteSection(item, body, names)
	}
	return nil
}

func applySetSection(item map[string]types.AttributeValue, body string, names map[string]string, values map[string]types.AttributeValue) error {
	for _, assignment := range splitCSV(body) {
		left, right, ok := strings.Cut(assignment, "=")
		if !ok {
			return fmt.Errorf("invalid SET assignment %q", assignment)
		}
		attr := resolveName(left, names)
		value, err := evalUpdateValue(strings.TrimSpace(right), item, names, values)
		if err != nil {
			return err
		}
		item[attr] = value
	}
	return nil
}

func applyAddSection(item map[string]types.AttributeValue, body string, names map[string]string, values map[string]types.AttributeValue) error {
	for _, addition := range splitCSV(body) {
		fields := strings.Fields(addition)
		if len(fields) != 2 {
			return fmt.Errorf("invalid ADD expression %q", addition)
		}
		attr := resolveName(fields[0], names)
		item[attr] = addAV(item[attr], values[fields[1]])
	}
	return nil
}

func applyRemoveSection(item map[string]types.AttributeValue, body string, names map[string]string) {
	for _, attrExpr := range splitCSV(body) {
		delete(item, resolveName(attrExpr, names))
	}
}

func applyDeleteSection(item map[string]types.AttributeValue, body string, names map[string]string) {
	for _, deleteExpr := range splitCSV(body) {
		fields := strings.Fields(deleteExpr)
		if len(fields) > 0 {
			delete(item, resolveName(fields[0], names))
		}
	}
}

var updateKeywordRE = regexp.MustCompile(`\b(SET|ADD|REMOVE|DELETE)\b`)

func updateSections(expr string) map[string]string {
	matches := updateKeywordRE.FindAllStringIndex(expr, -1)
	out := make(map[string]string, len(matches))
	for i, match := range matches {
		action := expr[match[0]:match[1]]
		start := match[1]
		end := len(expr)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		out[action] = strings.TrimSpace(expr[start:end])
	}
	return out
}

func evalUpdateValue(expr string, item map[string]types.AttributeValue, names map[string]string, values map[string]types.AttributeValue) (types.AttributeValue, error) {
	if strings.Contains(expr, " + ") {
		parts := strings.Split(expr, " + ")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid addition expression %q", expr)
		}
		left, err := evalUpdateValue(strings.TrimSpace(parts[0]), item, names, values)
		if err != nil {
			return nil, err
		}
		right, err := evalUpdateValue(strings.TrimSpace(parts[1]), item, names, values)
		if err != nil {
			return nil, err
		}
		return addAV(left, right), nil
	}
	if strings.HasPrefix(strings.ToLower(expr), "if_not_exists(") && strings.HasSuffix(expr, ")") {
		args := splitCSV(expr[len("if_not_exists(") : len(expr)-1])
		if len(args) != 2 {
			return nil, fmt.Errorf("invalid if_not_exists expression %q", expr)
		}
		attr := resolveName(args[0], names)
		if item[attr] != nil {
			return cloneAV(item[attr]), nil
		}
		return evalUpdateValue(strings.TrimSpace(args[1]), item, names, values)
	}
	if strings.HasPrefix(expr, ":") {
		return cloneAV(values[expr]), nil
	}
	return cloneAV(item[resolveName(expr, names)]), nil
}

func splitCSV(s string) []string {
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	last := strings.TrimSpace(s[start:])
	if last != "" {
		parts = append(parts, last)
	}
	return parts
}

func resolveName(token string, names map[string]string) string {
	token = strings.TrimSpace(token)
	if resolved, ok := names[token]; ok {
		return resolved
	}
	return token
}

func projectItem(item map[string]types.AttributeValue, projection *string, names map[string]string) map[string]types.AttributeValue {
	if len(item) == 0 {
		return item
	}
	if projection == nil || strings.TrimSpace(*projection) == "" {
		return cloneItem(item)
	}
	out := make(map[string]types.AttributeValue)
	for _, token := range splitCSV(*projection) {
		attr := resolveName(token, names)
		if av := item[attr]; av != nil {
			out[attr] = cloneAV(av)
		}
	}
	return out
}

func compareAV(left, right types.AttributeValue) int {
	if cmp, ok := compareNilAV(left, right); ok {
		return cmp
	}
	switch l := left.(type) {
	case *types.AttributeValueMemberS:
		return compareStringMember(l, right)
	case *types.AttributeValueMemberN:
		return compareNumberMember(l, right)
	case *types.AttributeValueMemberB:
		return compareBinaryMember(l, right)
	case *types.AttributeValueMemberBOOL:
		return compareBoolMember(l, right)
	default:
		if reflect.DeepEqual(left, right) {
			return 0
		}
		return strings.Compare(avKeyComponent(left), avKeyComponent(right))
	}
}

func compareNilAV(left, right types.AttributeValue) (int, bool) {
	if left == nil && right == nil {
		return 0, true
	}
	if left == nil {
		return -1, true
	}
	if right == nil {
		return 1, true
	}
	return 0, false
}

func compareStringMember(left *types.AttributeValueMemberS, right types.AttributeValue) int {
	rightString, ok := right.(*types.AttributeValueMemberS)
	if !ok {
		return strings.Compare(avKeyComponent(left), avKeyComponent(right))
	}
	return strings.Compare(left.Value, rightString.Value)
}

func compareNumberMember(left *types.AttributeValueMemberN, right types.AttributeValue) int {
	rightNumber, ok := right.(*types.AttributeValueMemberN)
	if !ok {
		return strings.Compare(avKeyComponent(left), avKeyComponent(right))
	}
	leftRat, leftOK := new(big.Rat).SetString(left.Value)
	rightRat, rightOK := new(big.Rat).SetString(rightNumber.Value)
	if !leftOK || !rightOK {
		return strings.Compare(left.Value, rightNumber.Value)
	}
	return leftRat.Cmp(rightRat)
}

func compareBinaryMember(left *types.AttributeValueMemberB, right types.AttributeValue) int {
	rightBinary, ok := right.(*types.AttributeValueMemberB)
	if !ok {
		return strings.Compare(avKeyComponent(left), avKeyComponent(right))
	}
	return bytes.Compare(left.Value, rightBinary.Value)
}

func compareBoolMember(left *types.AttributeValueMemberBOOL, right types.AttributeValue) int {
	rightBool, ok := right.(*types.AttributeValueMemberBOOL)
	if !ok {
		return strings.Compare(avKeyComponent(left), avKeyComponent(right))
	}
	if left.Value == rightBool.Value {
		return 0
	}
	if !left.Value {
		return -1
	}
	return 1
}

func stringValue(av types.AttributeValue) string {
	switch v := av.(type) {
	case *types.AttributeValueMemberS:
		return v.Value
	case *types.AttributeValueMemberN:
		return v.Value
	case *types.AttributeValueMemberB:
		return string(v.Value)
	default:
		return avKeyComponent(av)
	}
}

func containsAV(container, needle types.AttributeValue) bool {
	switch v := container.(type) {
	case *types.AttributeValueMemberS:
		return strings.Contains(v.Value, stringValue(needle))
	case *types.AttributeValueMemberSS:
		want := stringValue(needle)
		for _, value := range v.Value {
			if value == want {
				return true
			}
		}
	case *types.AttributeValueMemberNS:
		want := stringValue(needle)
		for _, value := range v.Value {
			if value == want {
				return true
			}
		}
	case *types.AttributeValueMemberL:
		for _, value := range v.Value {
			if compareAV(value, needle) == 0 {
				return true
			}
		}
	}
	return false
}

func addAV(left, right types.AttributeValue) types.AttributeValue {
	if left == nil {
		return cloneAV(right)
	}
	ln, lok := left.(*types.AttributeValueMemberN)
	rn, rok := right.(*types.AttributeValueMemberN)
	if lok && rok {
		return &types.AttributeValueMemberN{Value: addNumberStrings(ln.Value, rn.Value)}
	}
	return cloneAV(right)
}

func addNumberStrings(left, right string) string {
	l, lok := new(big.Rat).SetString(left)
	r, rok := new(big.Rat).SetString(right)
	if !lok || !rok {
		li, lerr := strconv.ParseInt(left, 10, 64)
		ri, rerr := strconv.ParseInt(right, 10, 64)
		if lerr != nil || rerr != nil {
			return right
		}
		return strconv.FormatInt(li+ri, 10)
	}
	l.Add(l, r)
	if l.IsInt() {
		return l.Num().String()
	}
	out := l.FloatString(10)
	return strings.TrimRight(strings.TrimRight(out, "0"), ".")
}

func avKeyComponent(av types.AttributeValue) string {
	switch v := av.(type) {
	case *types.AttributeValueMemberS:
		return "S:" + v.Value
	case *types.AttributeValueMemberN:
		return "N:" + v.Value
	case *types.AttributeValueMemberB:
		return "B:" + string(v.Value)
	case *types.AttributeValueMemberBOOL:
		return fmt.Sprintf("BOOL:%t", v.Value)
	default:
		return fmt.Sprintf("%T:%v", av, av)
	}
}

func cloneItem(item map[string]types.AttributeValue) map[string]types.AttributeValue {
	if item == nil {
		return nil
	}
	out := make(map[string]types.AttributeValue, len(item))
	for key, value := range item {
		out[key] = cloneAV(value)
	}
	return out
}

func cloneAV(av types.AttributeValue) types.AttributeValue {
	switch v := av.(type) {
	case *types.AttributeValueMemberS:
		return &types.AttributeValueMemberS{Value: v.Value}
	case *types.AttributeValueMemberN:
		return &types.AttributeValueMemberN{Value: v.Value}
	case *types.AttributeValueMemberB:
		return &types.AttributeValueMemberB{Value: append([]byte(nil), v.Value...)}
	case *types.AttributeValueMemberBOOL:
		return &types.AttributeValueMemberBOOL{Value: v.Value}
	case *types.AttributeValueMemberNULL:
		return &types.AttributeValueMemberNULL{Value: v.Value}
	case *types.AttributeValueMemberSS:
		return &types.AttributeValueMemberSS{Value: append([]string(nil), v.Value...)}
	case *types.AttributeValueMemberNS:
		return &types.AttributeValueMemberNS{Value: append([]string(nil), v.Value...)}
	case *types.AttributeValueMemberBS:
		out := make([][]byte, len(v.Value))
		for i := range v.Value {
			out[i] = append([]byte(nil), v.Value[i]...)
		}
		return &types.AttributeValueMemberBS{Value: out}
	case *types.AttributeValueMemberL:
		out := make([]types.AttributeValue, len(v.Value))
		for i := range v.Value {
			out[i] = cloneAV(v.Value[i])
		}
		return &types.AttributeValueMemberL{Value: out}
	case *types.AttributeValueMemberM:
		return &types.AttributeValueMemberM{Value: cloneItem(v.Value)}
	default:
		return av
	}
}

func resourceNotFound(tableName string) error {
	return &types.ResourceNotFoundException{Message: aws.String(fmt.Sprintf("table not found: %s", tableName))}
}

func conditionalFailed() error {
	return &types.ConditionalCheckFailedException{Message: aws.String("conditional request failed")}
}

func transactionCanceled(index int, err error) error {
	if err == nil {
		return nil
	}
	code := "ConditionalCheckFailed"
	message := err.Error()
	return &types.TransactionCanceledException{
		Message: aws.String("transaction canceled"),
		CancellationReasons: []types.CancellationReason{
			{
				Code:    aws.String(code),
				Message: aws.String(fmt.Sprintf("operation %d: %s", index, message)),
			},
		},
	}
}
