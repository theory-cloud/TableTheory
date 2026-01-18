package theorydb

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/theory-cloud/tabletheory/internal/encryption"
	"github.com/theory-cloud/tabletheory/pkg/core"
	customerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/model"
	"github.com/theory-cloud/tabletheory/pkg/query"
	"github.com/theory-cloud/tabletheory/pkg/session"
)

type queryExecutor struct {
	db       *DB
	metadata *model.Metadata
	ctx      context.Context
}

func (qe *queryExecutor) SetContext(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	qe.ctx = ctx
}

func (qe *queryExecutor) ctxOrBackground() context.Context {
	if qe.ctx != nil {
		return qe.ctx
	}
	if qe.db != nil && qe.db.ctx != nil {
		return qe.db.ctx
	}
	return context.Background()
}

func (qe *queryExecutor) checkLambdaTimeout() error {
	if qe == nil || qe.db == nil || qe.db.lambdaDeadline.IsZero() {
		return nil
	}

	remaining := time.Until(qe.db.lambdaDeadline)
	if remaining <= 0 {
		return fmt.Errorf("lambda timeout exceeded")
	}

	buffer := qe.db.lambdaTimeoutBuffer
	if buffer == 0 {
		buffer = 100 * time.Millisecond
	}
	if remaining < buffer {
		return fmt.Errorf("lambda timeout imminent: only %v remaining", remaining)
	}

	return nil
}

func (qe *queryExecutor) encryptionService() (*encryption.Service, error) {
	if qe == nil {
		return nil, fmt.Errorf("%w: query executor is nil", customerrors.ErrEncryptionNotConfigured)
	}
	if qe.db == nil || qe.db.session == nil || qe.db.session.Config() == nil {
		return nil, fmt.Errorf("%w: session is nil", customerrors.ErrEncryptionNotConfigured)
	}

	cfg := qe.db.session.Config()
	keyARN := cfg.KMSKeyARN
	if keyARN == "" {
		return nil, fmt.Errorf("%w: session.Config.KMSKeyARN is empty", customerrors.ErrEncryptionNotConfigured)
	}

	if cfg.KMSClient != nil {
		return encryption.NewServiceWithRand(keyARN, cfg.KMSClient, cfg.EncryptionRand), nil
	}

	return encryption.NewServiceFromAWSConfigWithRand(keyARN, qe.db.session.AWSConfig(), cfg.EncryptionRand), nil
}

func (qe *queryExecutor) failClosedIfEncrypted() error {
	if qe == nil {
		return nil
	}
	return encryption.FailClosedIfEncryptedWithoutKMSKeyARN(qe.session(), qe.metadata)
}

func (qe *queryExecutor) session() *session.Session {
	if qe == nil || qe.db == nil {
		return nil
	}
	return qe.db.session
}

func (qe *queryExecutor) decryptItem(item map[string]types.AttributeValue) error {
	if len(item) == 0 || qe == nil || qe.metadata == nil || !encryption.MetadataHasEncryptedFields(qe.metadata) {
		return nil
	}
	if err := qe.failClosedIfEncrypted(); err != nil {
		return err
	}

	svc, err := qe.encryptionService()
	if err != nil {
		return err
	}

	for attrName, attrValue := range item {
		fieldMeta, ok := qe.metadata.FieldsByDBName[attrName]
		if !ok || fieldMeta == nil || !fieldMeta.IsEncrypted {
			continue
		}

		decrypted, err := svc.DecryptAttributeValue(qe.ctxOrBackground(), fieldMeta.DBName, attrValue)
		if err != nil {
			return &customerrors.EncryptedFieldError{
				Operation: "decrypt",
				Field:     fieldMeta.Name,
				Err:       err,
			}
		}
		item[attrName] = decrypted
	}

	return nil
}

func (qe *queryExecutor) encryptItem(item map[string]types.AttributeValue) error {
	if len(item) == 0 || qe == nil || qe.metadata == nil || !encryption.MetadataHasEncryptedFields(qe.metadata) {
		return nil
	}
	if err := qe.failClosedIfEncrypted(); err != nil {
		return err
	}

	svc, err := qe.encryptionService()
	if err != nil {
		return err
	}

	for _, fieldMeta := range qe.metadata.Fields {
		if fieldMeta == nil || !fieldMeta.IsEncrypted {
			continue
		}

		av, ok := item[fieldMeta.DBName]
		if !ok {
			continue
		}

		encryptedAV, err := svc.EncryptAttributeValue(qe.ctxOrBackground(), fieldMeta.DBName, av)
		if err != nil {
			return fmt.Errorf("failed to encrypt field %s: %w", fieldMeta.DBName, err)
		}
		item[fieldMeta.DBName] = encryptedAV
	}

	return nil
}

func (qe *queryExecutor) unmarshalItem(item map[string]types.AttributeValue, dest any) error {
	if qe == nil || qe.db == nil || qe.db.converter == nil {
		return fmt.Errorf("converter is required for unmarshal")
	}

	destValue, err := derefNonNilPointer(dest)
	if err != nil {
		return err
	}

	switch destValue.Kind() {
	case reflect.Map:
		return qe.unmarshalItemToMap(item, destValue)
	case reflect.Struct:
		return qe.unmarshalItemToStruct(item, destValue)
	default:
		return fmt.Errorf("destination must be a pointer to a struct or map")
	}
}

func derefNonNilPointer(dest any) (reflect.Value, error) {
	if dest == nil {
		return reflect.Value{}, fmt.Errorf("destination must be a pointer to a struct or map")
	}

	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr || destValue.IsNil() {
		return reflect.Value{}, fmt.Errorf("destination must be a pointer")
	}

	return destValue.Elem(), nil
}

func (qe *queryExecutor) unmarshalItemToMap(item map[string]types.AttributeValue, destValue reflect.Value) error {
	if destValue.Type().Key().Kind() != reflect.String {
		return fmt.Errorf("destination map key must be a string")
	}
	if destValue.IsNil() {
		destValue.Set(reflect.MakeMap(destValue.Type()))
	}

	valueType := destValue.Type().Elem()
	attributeValueType := reflect.TypeOf((*types.AttributeValue)(nil)).Elem()

	for attrName, attrValue := range item {
		var value reflect.Value
		switch {
		case valueType == attributeValueType:
			value = reflect.ValueOf(attrValue)
		case valueType.Kind() == reflect.Interface && valueType.NumMethod() == 0:
			decoded, err := attributeValueToInterface(attrValue)
			if err != nil {
				return fmt.Errorf("failed to unmarshal field %s: %w", attrName, err)
			}
			if decoded == nil {
				value = reflect.Zero(valueType)
			} else {
				value = reflect.ValueOf(decoded)
			}
		default:
			target := reflect.New(valueType)
			if err := qe.db.converter.FromAttributeValue(attrValue, target.Interface()); err != nil {
				return fmt.Errorf("failed to unmarshal field %s: %w", attrName, err)
			}
			value = target.Elem()
		}

		if !value.IsValid() {
			value = reflect.Zero(valueType)
		}

		destValue.SetMapIndex(reflect.ValueOf(attrName), value)
	}

	return nil
}

func attributeValueToInterface(av types.AttributeValue) (interface{}, error) {
	switch typed := av.(type) {
	case *types.AttributeValueMemberS:
		return typed.Value, nil
	case *types.AttributeValueMemberN:
		return parseNumberToInterface(typed.Value)
	case *types.AttributeValueMemberBOOL:
		return typed.Value, nil
	case *types.AttributeValueMemberNULL:
		return nil, nil
	case *types.AttributeValueMemberL:
		return attributeValueListToInterface(typed.Value)
	case *types.AttributeValueMemberM:
		return attributeValueMapToInterface(typed.Value)
	case *types.AttributeValueMemberSS:
		return typed.Value, nil
	case *types.AttributeValueMemberNS:
		return attributeValueNumberSetToFloat64(typed.Value)
	case *types.AttributeValueMemberBS:
		return typed.Value, nil
	case *types.AttributeValueMemberB:
		return typed.Value, nil
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
	out := make([]interface{}, len(values))
	for i, item := range values {
		converted, err := attributeValueToInterface(item)
		if err != nil {
			return nil, err
		}
		out[i] = converted
	}
	return out, nil
}

func attributeValueMapToInterface(values map[string]types.AttributeValue) (map[string]interface{}, error) {
	out := make(map[string]interface{}, len(values))
	for key, item := range values {
		converted, err := attributeValueToInterface(item)
		if err != nil {
			return nil, err
		}
		out[key] = converted
	}
	return out, nil
}

func attributeValueNumberSetToFloat64(values []string) ([]float64, error) {
	out := make([]float64, len(values))
	for i, numStr := range values {
		f, err := strconv.ParseFloat(numStr, 64)
		if err != nil {
			return nil, err
		}
		out[i] = f
	}
	return out, nil
}

func (qe *queryExecutor) unmarshalItemToStruct(item map[string]types.AttributeValue, destValue reflect.Value) error {
	if qe.metadata == nil {
		return fmt.Errorf("model metadata is required for unmarshal")
	}

	for attrName, attrValue := range item {
		fieldMeta, exists := qe.metadata.FieldsByDBName[attrName]
		if !exists || fieldMeta == nil {
			continue
		}

		structField := destValue.FieldByIndex(fieldMeta.IndexPath)
		if !structField.CanSet() {
			continue
		}

		if err := qe.db.converter.FromAttributeValue(attrValue, structField.Addr().Interface()); err != nil {
			return fmt.Errorf("failed to unmarshal field %s: %w", fieldMeta.Name, err)
		}
	}

	return nil
}

func (qe *queryExecutor) unmarshalItems(items []map[string]types.AttributeValue, dest any) error {
	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr || destValue.IsNil() || destValue.Elem().Kind() != reflect.Slice {
		return fmt.Errorf("destination must be a pointer to slice")
	}

	destSlice := destValue.Elem()
	elemType := destSlice.Type().Elem()
	newSlice := reflect.MakeSlice(destSlice.Type(), len(items), len(items))

	for i, item := range items {
		var elem reflect.Value
		if elemType.Kind() == reflect.Ptr {
			elem = reflect.New(elemType.Elem())
		} else {
			elem = reflect.New(elemType)
		}

		if err := qe.unmarshalItem(item, elem.Interface()); err != nil {
			return fmt.Errorf("failed to unmarshal item %d: %w", i, err)
		}

		if elemType.Kind() == reflect.Ptr {
			newSlice.Index(i).Set(elem)
		} else {
			newSlice.Index(i).Set(elem.Elem())
		}
	}

	destSlice.Set(newSlice)
	return nil
}

func (qe *queryExecutor) writeItemsToDest(items []map[string]types.AttributeValue, dest any) error {
	for _, item := range items {
		if err := qe.decryptItem(item); err != nil {
			return err
		}
	}

	if rawDest, ok := dest.(*[]map[string]types.AttributeValue); ok && rawDest != nil {
		*rawDest = append((*rawDest)[:0], items...)
		return nil
	}

	return qe.unmarshalItems(items, dest)
}

type countPageFunc func(context.Context) (int32, int32, error)
type itemPageFunc func(context.Context) ([]map[string]types.AttributeValue, error)

type pagePaginator[Output any] interface {
	HasMorePages() bool
	NextPage(context.Context, ...func(*dynamodb.Options)) (*Output, error)
}

type readPagerSpec struct {
	buildCountPager func(*dynamodb.Client, *core.CompiledQuery) (func() bool, countPageFunc)
	buildItemPager  func(*dynamodb.Client, *core.CompiledQuery) (func() bool, itemPageFunc)
	nilErr          string
	operation       string
}

func newReadPagerSpec[Input any, Output any, P pagePaginator[Output]](
	nilErr string,
	operation string,
	buildInput func(*core.CompiledQuery) *Input,
	configureCountInput func(*Input),
	newPaginator func(*dynamodb.Client, *Input) P,
	extractCounts func(*Output) (int32, int32),
	extractItems func(*Output) []map[string]types.AttributeValue,
) readPagerSpec {
	return readPagerSpec{
		nilErr:    nilErr,
		operation: operation,
		buildCountPager: func(client *dynamodb.Client, input *core.CompiledQuery) (func() bool, countPageFunc) {
			countInput := buildInput(input)
			configureCountInput(countInput)

			paginator := newPaginator(client, countInput)
			return paginator.HasMorePages, func(ctx context.Context) (int32, int32, error) {
				page, pageErr := paginator.NextPage(ctx)
				if pageErr != nil {
					return 0, 0, fmt.Errorf("failed to count items: %w", pageErr)
				}

				count, scannedCount := extractCounts(page)
				return count, scannedCount, nil
			}
		},
		buildItemPager: func(client *dynamodb.Client, input *core.CompiledQuery) (func() bool, itemPageFunc) {
			itemInput := buildInput(input)

			paginator := newPaginator(client, itemInput)
			return paginator.HasMorePages, func(ctx context.Context) ([]map[string]types.AttributeValue, error) {
				page, pageErr := paginator.NextPage(ctx)
				if pageErr != nil {
					return nil, fmt.Errorf("failed to %s items: %w", operation, pageErr)
				}
				return extractItems(page), nil
			}
		},
	}
}

func (qe *queryExecutor) executeReadSpec(input *core.CompiledQuery, dest any, spec readPagerSpec) error {
	return qe.executeRead(
		input,
		dest,
		spec.nilErr,
		spec.operation,
		func(client *dynamodb.Client) (func() bool, countPageFunc) {
			return spec.buildCountPager(client, input)
		},
		func(client *dynamodb.Client) (func() bool, itemPageFunc) {
			return spec.buildItemPager(client, input)
		},
	)
}

type singlePageResult struct {
	lastEvaluatedKey map[string]types.AttributeValue
	items            []map[string]types.AttributeValue
	count            int32
	scannedCount     int32
}

type singlePageSpec struct {
	execute   func(context.Context, *dynamodb.Client, *core.CompiledQuery) (singlePageResult, error)
	nilErr    string
	operation string
}

func (qe *queryExecutor) executeReadWithPaginationSpec(input *core.CompiledQuery, dest any, spec singlePageSpec) (singlePageResult, error) {
	return qe.executeReadWithPagination(
		input,
		dest,
		spec.nilErr,
		spec.operation,
		func(client *dynamodb.Client, ctx context.Context) (singlePageResult, error) {
			return spec.execute(ctx, client, input)
		},
	)
}

func executeReadWithPaginationConverted[T any](
	qe *queryExecutor,
	input *core.CompiledQuery,
	dest any,
	spec singlePageSpec,
	convert func(singlePageResult) T,
) (*T, error) {
	result, err := qe.executeReadWithPaginationSpec(input, dest, spec)
	if err != nil {
		return nil, err
	}

	converted := convert(result)
	return &converted, nil
}

func configureQueryCountInput(queryInput *dynamodb.QueryInput) {
	queryInput.Select = types.SelectCount
	queryInput.Limit = nil
}

func configureScanCountInput(scanInput *dynamodb.ScanInput) {
	scanInput.Select = types.SelectCount
	scanInput.Limit = nil
}

func newQueryPaginator(client *dynamodb.Client, queryInput *dynamodb.QueryInput) *dynamodb.QueryPaginator {
	return dynamodb.NewQueryPaginator(client, queryInput)
}

func newScanPaginator(client *dynamodb.Client, scanInput *dynamodb.ScanInput) *dynamodb.ScanPaginator {
	return dynamodb.NewScanPaginator(client, scanInput)
}

func queryCountsFromOutput(out *dynamodb.QueryOutput) (int32, int32) {
	return out.Count, out.ScannedCount
}

func scanCountsFromOutput(out *dynamodb.ScanOutput) (int32, int32) {
	return out.Count, out.ScannedCount
}

func queryItemsFromOutput(out *dynamodb.QueryOutput) []map[string]types.AttributeValue {
	return out.Items
}

func scanItemsFromOutput(out *dynamodb.ScanOutput) []map[string]types.AttributeValue {
	return out.Items
}

func newSinglePageResult(
	items []map[string]types.AttributeValue,
	count int32,
	scannedCount int32,
	lastEvaluatedKey map[string]types.AttributeValue,
) singlePageResult {
	return singlePageResult{
		items:            items,
		count:            count,
		scannedCount:     scannedCount,
		lastEvaluatedKey: lastEvaluatedKey,
	}
}

func executeQuerySinglePage(ctx context.Context, client *dynamodb.Client, input *core.CompiledQuery) (singlePageResult, error) {
	out, err := client.Query(ctx, buildDynamoQueryInput(input))
	if err != nil {
		return singlePageResult{}, fmt.Errorf("failed to execute query: %w", err)
	}
	return newSinglePageResult(out.Items, out.Count, out.ScannedCount, out.LastEvaluatedKey), nil
}

func executeScanSinglePage(ctx context.Context, client *dynamodb.Client, input *core.CompiledQuery) (singlePageResult, error) {
	out, err := client.Scan(ctx, buildDynamoScanInput(input))
	if err != nil {
		return singlePageResult{}, fmt.Errorf("failed to execute scan: %w", err)
	}
	return newSinglePageResult(out.Items, out.Count, out.ScannedCount, out.LastEvaluatedKey), nil
}

var queryReadPagerSpec = newReadPagerSpec(
	"compiled query cannot be nil",
	"query",
	buildDynamoQueryInput,
	configureQueryCountInput,
	newQueryPaginator,
	queryCountsFromOutput,
	queryItemsFromOutput,
)

var scanReadPagerSpec = newReadPagerSpec(
	"compiled scan cannot be nil",
	"scan",
	buildDynamoScanInput,
	configureScanCountInput,
	newScanPaginator,
	scanCountsFromOutput,
	scanItemsFromOutput,
)

var querySinglePageSpec = singlePageSpec{
	nilErr:    "compiled query cannot be nil",
	operation: "query",
	execute:   executeQuerySinglePage,
}

var scanSinglePageSpec = singlePageSpec{
	nilErr:    "compiled scan cannot be nil",
	operation: "scan",
	execute:   executeScanSinglePage,
}

func (qe *queryExecutor) readClient(input *core.CompiledQuery, nilErr string, operation string) (*dynamodb.Client, error) {
	if input == nil {
		return nil, errors.New(nilErr)
	}
	if err := qe.checkLambdaTimeout(); err != nil {
		return nil, err
	}
	if err := qe.failClosedIfEncrypted(); err != nil {
		return nil, err
	}

	client, err := qe.session().Client()
	if err != nil {
		return nil, fmt.Errorf("failed to get client for %s: %w", operation, err)
	}
	return client, nil
}

func (qe *queryExecutor) executeRead(
	input *core.CompiledQuery,
	dest any,
	nilErr string,
	operation string,
	buildCountPager func(*dynamodb.Client) (func() bool, countPageFunc),
	buildItemPager func(*dynamodb.Client) (func() bool, itemPageFunc),
) error {
	client, err := qe.readClient(input, nilErr, operation)
	if err != nil {
		return err
	}

	if isCountSelect(input.Select) {
		hasMorePages, nextPage := buildCountPager(client)
		totalCount, scannedCount, countErr := collectPaginatedCounts(qe.ctxOrBackground(), hasMorePages, nextPage)
		if countErr != nil {
			return countErr
		}
		return writeCountResult(dest, totalCount, scannedCount)
	}

	hasMorePages, nextPage := buildItemPager(client)
	limit, hasLimit := compiledQueryLimit(input)
	items, itemsErr := collectPaginatedItems(qe.ctxOrBackground(), hasMorePages, nextPage, limit, hasLimit, true)
	if itemsErr != nil {
		return itemsErr
	}

	return qe.writeItemsToDest(items, dest)
}

func (qe *queryExecutor) executeReadWithPagination(
	input *core.CompiledQuery,
	dest any,
	nilErr string,
	operation string,
	execute func(*dynamodb.Client, context.Context) (singlePageResult, error),
) (singlePageResult, error) {
	client, err := qe.readClient(input, nilErr, operation)
	if err != nil {
		return singlePageResult{}, err
	}

	result, execErr := execute(client, qe.ctxOrBackground())
	if execErr != nil {
		return singlePageResult{}, execErr
	}

	if err := qe.writeItemsToDest(result.items, dest); err != nil {
		return singlePageResult{}, err
	}

	return result, nil
}

func (qe *queryExecutor) ExecuteQuery(input *core.CompiledQuery, dest any) error {
	return qe.executeReadSpec(input, dest, queryReadPagerSpec)
}

func (qe *queryExecutor) ExecuteScan(input *core.CompiledQuery, dest any) error {
	return qe.executeReadSpec(input, dest, scanReadPagerSpec)
}

func (qe *queryExecutor) ExecuteQueryWithPagination(input *core.CompiledQuery, dest any) (*query.QueryResult, error) {
	return executeReadWithPaginationConverted(
		qe,
		input,
		dest,
		querySinglePageSpec,
		func(result singlePageResult) query.QueryResult {
			return query.QueryResult{
				Items:            result.items,
				Count:            int64(result.count),
				ScannedCount:     int64(result.scannedCount),
				LastEvaluatedKey: result.lastEvaluatedKey,
			}
		},
	)
}

func (qe *queryExecutor) ExecuteScanWithPagination(input *core.CompiledQuery, dest any) (*query.ScanResult, error) {
	return executeReadWithPaginationConverted(
		qe,
		input,
		dest,
		scanSinglePageSpec,
		func(result singlePageResult) query.ScanResult {
			return query.ScanResult{
				Items:            result.items,
				Count:            int64(result.count),
				ScannedCount:     int64(result.scannedCount),
				LastEvaluatedKey: result.lastEvaluatedKey,
			}
		},
	)
}

func (qe *queryExecutor) ExecuteGetItem(input *core.CompiledQuery, key map[string]types.AttributeValue, dest any) error {
	if input == nil {
		return fmt.Errorf("compiled query cannot be nil")
	}
	if len(key) == 0 {
		return fmt.Errorf("key cannot be empty")
	}
	if err := qe.checkLambdaTimeout(); err != nil {
		return err
	}
	if err := qe.failClosedIfEncrypted(); err != nil {
		return err
	}

	client, err := qe.session().Client()
	if err != nil {
		return fmt.Errorf("failed to get client for get item: %w", err)
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

	out, err := client.GetItem(qe.ctxOrBackground(), getInput)
	if err != nil {
		return fmt.Errorf("failed to get item: %w", err)
	}
	if out.Item == nil {
		return customerrors.ErrItemNotFound
	}

	if err := qe.decryptItem(out.Item); err != nil {
		return err
	}

	if rawDest, ok := dest.(*map[string]types.AttributeValue); ok && rawDest != nil {
		*rawDest = out.Item
		return nil
	}

	return qe.unmarshalItem(out.Item, dest)
}

func (qe *queryExecutor) ExecutePutItem(input *core.CompiledQuery, item map[string]types.AttributeValue) error {
	if input == nil {
		return fmt.Errorf("compiled query cannot be nil")
	}
	if len(item) == 0 {
		return fmt.Errorf("item cannot be empty")
	}
	if err := qe.checkLambdaTimeout(); err != nil {
		return err
	}
	if err := qe.failClosedIfEncrypted(); err != nil {
		return err
	}

	if err := qe.encryptItem(item); err != nil {
		return err
	}

	client, err := qe.session().Client()
	if err != nil {
		return fmt.Errorf("failed to get client for put item: %w", err)
	}

	putInput := &dynamodb.PutItemInput{
		TableName: aws.String(input.TableName),
		Item:      item,
	}

	if input.ConditionExpression != "" {
		putInput.ConditionExpression = aws.String(input.ConditionExpression)
	}
	if len(input.ExpressionAttributeNames) > 0 {
		putInput.ExpressionAttributeNames = input.ExpressionAttributeNames
	}
	if len(input.ExpressionAttributeValues) > 0 {
		putInput.ExpressionAttributeValues = input.ExpressionAttributeValues
	}

	_, err = client.PutItem(qe.ctxOrBackground(), putInput)
	if err != nil {
		if isConditionalCheckFailedException(err) {
			return customerrors.ErrConditionFailed
		}
		return fmt.Errorf("failed to put item: %w", err)
	}

	return nil
}

func (qe *queryExecutor) buildUpdateItemInput(input *core.CompiledQuery, key map[string]types.AttributeValue) (*dynamodb.UpdateItemInput, error) {
	exprAttrValues := input.ExpressionAttributeValues

	if qe.metadata != nil && encryption.MetadataHasEncryptedFields(qe.metadata) {
		svc, err := qe.encryptionService()
		if err != nil {
			return nil, err
		}
		if err := encryption.EncryptUpdateExpressionValues(
			qe.ctxOrBackground(),
			svc,
			qe.metadata,
			input.UpdateExpression,
			input.ExpressionAttributeNames,
			exprAttrValues,
		); err != nil {
			return nil, err
		}
	}

	updateInput := &dynamodb.UpdateItemInput{
		TableName:        aws.String(input.TableName),
		Key:              key,
		UpdateExpression: aws.String(input.UpdateExpression),
	}

	if input.ConditionExpression != "" {
		updateInput.ConditionExpression = aws.String(input.ConditionExpression)
	}
	if input.ReturnValues != "" {
		updateInput.ReturnValues = types.ReturnValue(input.ReturnValues)
	}
	if len(input.ExpressionAttributeNames) > 0 {
		updateInput.ExpressionAttributeNames = input.ExpressionAttributeNames
	}
	if len(exprAttrValues) > 0 {
		updateInput.ExpressionAttributeValues = exprAttrValues
	}

	return updateInput, nil
}

func (qe *queryExecutor) ExecuteUpdateItem(input *core.CompiledQuery, key map[string]types.AttributeValue) error {
	if input == nil {
		return fmt.Errorf("compiled query cannot be nil")
	}
	if len(key) == 0 {
		return fmt.Errorf("key cannot be empty")
	}
	if err := qe.checkLambdaTimeout(); err != nil {
		return err
	}
	if err := qe.failClosedIfEncrypted(); err != nil {
		return err
	}

	client, err := qe.session().Client()
	if err != nil {
		return fmt.Errorf("failed to get client for update item: %w", err)
	}

	updateInput, err := qe.buildUpdateItemInput(input, key)
	if err != nil {
		return err
	}

	_, err = client.UpdateItem(qe.ctxOrBackground(), updateInput)
	if err != nil {
		if isConditionalCheckFailedException(err) {
			return customerrors.ErrConditionFailed
		}
		return fmt.Errorf("failed to update item: %w", err)
	}

	return nil
}

func (qe *queryExecutor) ExecuteUpdateItemWithResult(input *core.CompiledQuery, key map[string]types.AttributeValue) (*core.UpdateResult, error) {
	if input == nil {
		return nil, fmt.Errorf("compiled query cannot be nil")
	}
	if len(key) == 0 {
		return nil, fmt.Errorf("key cannot be empty")
	}
	if err := qe.checkLambdaTimeout(); err != nil {
		return nil, err
	}
	if err := qe.failClosedIfEncrypted(); err != nil {
		return nil, err
	}

	client, err := qe.session().Client()
	if err != nil {
		return nil, fmt.Errorf("failed to get client for update item: %w", err)
	}

	updateInput, err := qe.buildUpdateItemInput(input, key)
	if err != nil {
		return nil, err
	}

	output, err := client.UpdateItem(qe.ctxOrBackground(), updateInput)
	if err != nil {
		if isConditionalCheckFailedException(err) {
			return nil, customerrors.ErrConditionFailed
		}
		return nil, fmt.Errorf("failed to update item: %w", err)
	}

	if err := qe.decryptItem(output.Attributes); err != nil {
		return nil, err
	}

	return &core.UpdateResult{
		Attributes: output.Attributes,
	}, nil
}

func (qe *queryExecutor) ExecuteDeleteItem(input *core.CompiledQuery, key map[string]types.AttributeValue) error {
	if input == nil {
		return fmt.Errorf("compiled query cannot be nil")
	}
	if len(key) == 0 {
		return fmt.Errorf("key cannot be empty")
	}
	if err := qe.checkLambdaTimeout(); err != nil {
		return err
	}
	if err := qe.failClosedIfEncrypted(); err != nil {
		return err
	}

	client, err := qe.session().Client()
	if err != nil {
		return fmt.Errorf("failed to get client for delete item: %w", err)
	}

	deleteInput := &dynamodb.DeleteItemInput{
		TableName: aws.String(input.TableName),
		Key:       key,
	}

	if input.ConditionExpression != "" {
		deleteInput.ConditionExpression = aws.String(input.ConditionExpression)
	}
	if len(input.ExpressionAttributeNames) > 0 {
		deleteInput.ExpressionAttributeNames = input.ExpressionAttributeNames
	}
	if len(input.ExpressionAttributeValues) > 0 {
		deleteInput.ExpressionAttributeValues = input.ExpressionAttributeValues
	}

	_, err = client.DeleteItem(qe.ctxOrBackground(), deleteInput)
	if err != nil {
		if isConditionalCheckFailedException(err) {
			return customerrors.ErrConditionFailed
		}
		return fmt.Errorf("failed to delete item: %w", err)
	}

	return nil
}

func (qe *queryExecutor) ExecuteBatchGet(input *query.CompiledBatchGet, opts *core.BatchGetOptions) ([]map[string]types.AttributeValue, error) {
	if input == nil {
		return nil, fmt.Errorf("compiled batch get cannot be nil")
	}
	if len(input.Keys) == 0 {
		return nil, nil
	}
	if err := qe.checkLambdaTimeout(); err != nil {
		return nil, err
	}
	if err := qe.failClosedIfEncrypted(); err != nil {
		return nil, err
	}

	client, err := qe.session().Client()
	if err != nil {
		return nil, fmt.Errorf("failed to get client for batch get: %w", err)
	}

	normalizedOpts := normalizeBatchGetOptions(opts)

	requestItems := map[string]types.KeysAndAttributes{
		input.TableName: buildKeysAndAttributes(input),
	}

	return qe.executeBatchGetWithRetry(client, requestItems, input.TableName, normalizedOpts)
}

func normalizeBatchGetOptions(opts *core.BatchGetOptions) *core.BatchGetOptions {
	if opts == nil {
		return core.DefaultBatchGetOptions()
	}
	return opts.Clone()
}

func (qe *queryExecutor) executeBatchGetWithRetry(
	client *dynamodb.Client,
	requestItems map[string]types.KeysAndAttributes,
	tableName string,
	opts *core.BatchGetOptions,
) ([]map[string]types.AttributeValue, error) {
	var collected []map[string]types.AttributeValue
	retryAttempt := 0

	for len(requestItems) > 0 {
		output, err := client.BatchGetItem(qe.ctxOrBackground(), &dynamodb.BatchGetItemInput{
			RequestItems: requestItems,
		})
		if err != nil {
			return collected, fmt.Errorf("failed to batch get items: %w", err)
		}

		for _, item := range output.Responses[tableName] {
			if err := qe.decryptItem(item); err != nil {
				return collected, err
			}
			collected = append(collected, item)
		}

		requestItems = output.UnprocessedKeys
		remaining := countUnprocessedKeys(requestItems)
		if remaining == 0 {
			break
		}

		if opts.RetryPolicy == nil || retryAttempt >= opts.RetryPolicy.MaxRetries {
			return collected, fmt.Errorf("batch get exhausted retries with %d unprocessed keys", remaining)
		}

		delay := calculateBatchRetryDelay(opts.RetryPolicy, retryAttempt)
		retryAttempt++
		time.Sleep(delay)
	}

	return collected, nil
}

func (qe *queryExecutor) ExecuteBatchWrite(input *query.CompiledBatchWrite) error {
	if input == nil {
		return fmt.Errorf("compiled batch write cannot be nil")
	}
	if len(input.Items) == 0 {
		return nil
	}
	if err := qe.checkLambdaTimeout(); err != nil {
		return err
	}
	if err := qe.failClosedIfEncrypted(); err != nil {
		return err
	}

	for i := 0; i < len(input.Items); i += 25 {
		end := i + 25
		if end > len(input.Items) {
			end = len(input.Items)
		}

		writeRequests := make([]types.WriteRequest, 0, end-i)
		for _, item := range input.Items[i:end] {
			writeRequests = append(writeRequests, types.WriteRequest{
				PutRequest: &types.PutRequest{Item: item},
			})
		}

		for {
			result, err := qe.ExecuteBatchWriteItem(input.TableName, writeRequests)
			if err != nil {
				return err
			}
			if result == nil || len(result.UnprocessedItems) == 0 {
				break
			}

			var unprocessed []types.WriteRequest
			for _, reqs := range result.UnprocessedItems {
				unprocessed = append(unprocessed, reqs...)
			}
			if len(unprocessed) == 0 {
				break
			}
			writeRequests = unprocessed
		}
	}

	return nil
}

func (qe *queryExecutor) ExecuteBatchWriteItem(tableName string, writeRequests []types.WriteRequest) (*core.BatchWriteResult, error) {
	if err := qe.checkLambdaTimeout(); err != nil {
		return nil, err
	}
	if len(writeRequests) == 0 {
		return &core.BatchWriteResult{}, nil
	}
	if len(writeRequests) > 25 {
		return nil, fmt.Errorf("batch write supports maximum 25 items per request, got %d", len(writeRequests))
	}
	if err := qe.failClosedIfEncrypted(); err != nil {
		return nil, err
	}

	if qe.metadata != nil && encryption.MetadataHasEncryptedFields(qe.metadata) {
		for i := range writeRequests {
			put := writeRequests[i].PutRequest
			if put == nil || len(put.Item) == 0 {
				continue
			}
			if err := qe.encryptItem(put.Item); err != nil {
				return nil, err
			}
		}
	}

	client, err := qe.session().Client()
	if err != nil {
		return nil, fmt.Errorf("failed to get client for batch write: %w", err)
	}

	output, err := client.BatchWriteItem(qe.ctxOrBackground(), &dynamodb.BatchWriteItemInput{
		RequestItems: map[string][]types.WriteRequest{
			tableName: writeRequests,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("batch write failed: %w", err)
	}

	return &core.BatchWriteResult{
		UnprocessedItems: output.UnprocessedItems,
		ConsumedCapacity: output.ConsumedCapacity,
	}, nil
}

func buildDynamoQueryInput(input *core.CompiledQuery) *dynamodb.QueryInput {
	out := &dynamodb.QueryInput{
		TableName: aws.String(input.TableName),
	}

	if input.IndexName != "" {
		out.IndexName = aws.String(input.IndexName)
	}
	if input.KeyConditionExpression != "" {
		out.KeyConditionExpression = aws.String(input.KeyConditionExpression)
	}
	if input.FilterExpression != "" {
		out.FilterExpression = aws.String(input.FilterExpression)
	}
	if input.ProjectionExpression != "" {
		out.ProjectionExpression = aws.String(input.ProjectionExpression)
	}
	if len(input.ExpressionAttributeNames) > 0 {
		out.ExpressionAttributeNames = input.ExpressionAttributeNames
	}
	if len(input.ExpressionAttributeValues) > 0 {
		out.ExpressionAttributeValues = input.ExpressionAttributeValues
	}
	if input.Limit != nil {
		out.Limit = input.Limit
	}
	if len(input.ExclusiveStartKey) > 0 {
		out.ExclusiveStartKey = input.ExclusiveStartKey
	}
	if input.ScanIndexForward != nil {
		out.ScanIndexForward = input.ScanIndexForward
	}
	if input.ConsistentRead != nil {
		out.ConsistentRead = input.ConsistentRead
	}

	return out
}

func buildDynamoScanInput(input *core.CompiledQuery) *dynamodb.ScanInput {
	out := &dynamodb.ScanInput{
		TableName: aws.String(input.TableName),
	}

	if input.IndexName != "" {
		out.IndexName = aws.String(input.IndexName)
	}
	if input.FilterExpression != "" {
		out.FilterExpression = aws.String(input.FilterExpression)
	}
	if input.ProjectionExpression != "" {
		out.ProjectionExpression = aws.String(input.ProjectionExpression)
	}
	if len(input.ExpressionAttributeNames) > 0 {
		out.ExpressionAttributeNames = input.ExpressionAttributeNames
	}
	if len(input.ExpressionAttributeValues) > 0 {
		out.ExpressionAttributeValues = input.ExpressionAttributeValues
	}
	if input.Limit != nil {
		out.Limit = input.Limit
	}
	if len(input.ExclusiveStartKey) > 0 {
		out.ExclusiveStartKey = input.ExclusiveStartKey
	}
	if input.ConsistentRead != nil {
		out.ConsistentRead = input.ConsistentRead
	}
	if input.Segment != nil {
		out.Segment = input.Segment
	}
	if input.TotalSegments != nil {
		out.TotalSegments = input.TotalSegments
	}

	return out
}

func compiledQueryLimit(input *core.CompiledQuery) (int, bool) {
	if input == nil || input.Limit == nil {
		return 0, false
	}
	if *input.Limit <= 0 {
		return 0, true
	}
	return int(*input.Limit), true
}

func collectPaginatedCounts(
	ctx context.Context,
	hasMorePages func() bool,
	nextPage func(context.Context) (int32, int32, error),
) (int64, int64, error) {
	var totalCount int64
	var scannedCount int64
	for hasMorePages() {
		count, scanned, err := nextPage(ctx)
		if err != nil {
			return 0, 0, err
		}
		totalCount += int64(count)
		scannedCount += int64(scanned)
	}
	return totalCount, scannedCount, nil
}

func collectPaginatedItems(
	ctx context.Context,
	hasMorePages func() bool,
	nextPage func(context.Context) ([]map[string]types.AttributeValue, error),
	limit int,
	hasLimit bool,
	trim bool,
) ([]map[string]types.AttributeValue, error) {
	var items []map[string]types.AttributeValue
	for hasMorePages() {
		pageItems, err := nextPage(ctx)
		if err != nil {
			return nil, err
		}

		items = append(items, pageItems...)
		if hasLimit && len(items) >= limit {
			if trim {
				return items[:limit], nil
			}
			break
		}
	}
	return items, nil
}

func isCountSelect(selectValue string) bool {
	return selectValue == "COUNT"
}

func writeCountResult(dest any, count int64, scannedCount int64) error {
	if dest == nil {
		return fmt.Errorf("destination must be a pointer")
	}

	value := reflect.ValueOf(dest)
	if value.Kind() != reflect.Ptr || value.IsNil() {
		return fmt.Errorf("destination must be a pointer")
	}

	elem := value.Elem()
	switch elem.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		elem.SetInt(count)
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if count < 0 {
			return fmt.Errorf("count is negative")
		}
		elem.SetUint(uint64(count))
		return nil
	case reflect.Struct:
		if field := elem.FieldByName("Count"); field.IsValid() && field.CanSet() {
			setIntLike(field, count)
		}
		if field := elem.FieldByName("ScannedCount"); field.IsValid() && field.CanSet() {
			setIntLike(field, scannedCount)
		}
		return nil
	default:
		return fmt.Errorf("destination must be a pointer to an integer or struct")
	}
}

func setIntLike(field reflect.Value, value int64) {
	switch field.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		field.SetInt(value)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if value >= 0 {
			field.SetUint(uint64(value))
		}
	}
}

func isConditionalCheckFailedException(err error) bool {
	var ccfe *types.ConditionalCheckFailedException
	return errors.As(err, &ccfe)
}

func buildKeysAndAttributes(input *query.CompiledBatchGet) types.KeysAndAttributes {
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
	total := 0
	for _, entry := range unprocessed {
		total += len(entry.Keys)
	}
	return total
}

func cryptoFloat64() (float64, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0, err
	}

	u := binary.BigEndian.Uint64(b[:]) >> 11
	return float64(u) / (1 << 53), nil
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
