package query

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/theory-cloud/tabletheory/v3/internal/expr"
	"github.com/theory-cloud/tabletheory/v3/internal/fieldcodec"
	"github.com/theory-cloud/tabletheory/v3/internal/reflectutil"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	theorydbErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/marshal"
	"github.com/theory-cloud/tabletheory/v3/pkg/model"
)

func (q *Query) First(dest any) error {
	if err := q.checkBuilderError(); err != nil {
		return err
	}
	if q.retryConfig != nil {
		return q.firstWithRetry(dest)
	}
	return q.firstInternal(dest)
}

// FirstOrNil executes the query and reports whether an item was found.
// ErrItemNotFound is translated into (false, nil); all other errors are
// returned unchanged.
func (q *Query) FirstOrNil(dest any) (bool, error) {
	err := q.First(dest)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, theorydbErrors.ErrItemNotFound) {
		return false, nil
	}
	return false, err
}

// FirstOrNil executes any TableTheory query and reports whether an item was
// found. Queries that provide their own FirstOrNil implementation use it;
// otherwise this helper falls back to First while only suppressing
// ErrItemNotFound.
func FirstOrNil(q core.Query, dest any) (bool, error) {
	if q == nil {
		return false, fmt.Errorf("query cannot be nil")
	}
	if optional, ok := q.(interface {
		FirstOrNil(any) (bool, error)
	}); ok {
		return optional.FirstOrNil(dest)
	}
	err := q.First(dest)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, theorydbErrors.ErrItemNotFound) {
		return false, nil
	}
	return false, err
}

// All executes the query and returns all results
func (q *Query) All(dest any) error {
	if err := q.checkBuilderError(); err != nil {
		return err
	}
	if q.retryConfig != nil {
		return q.allWithRetry(dest)
	}
	return q.allInternal(dest)
}

// Count returns the count of matching items
func (q *Query) Count() (int64, error) {
	if err := q.checkBuilderError(); err != nil {
		return 0, err
	}
	// Projection and limit are item-materialization controls; they do not
	// change the matching item count. Compile a projection-free clone so
	// Select=COUNT cannot leave unused projection expression names behind.
	clone := *q
	clone.projection = nil
	compiled, err := clone.Compile()
	if err != nil {
		return 0, err
	}

	compiled.Select = "COUNT"
	compiled.Limit = nil

	var result struct {
		Count        int64
		ScannedCount int64
	}

	if compiled.Operation == operationQuery {
		err = q.executor.ExecuteQuery(compiled, &result)
	} else {
		err = q.executor.ExecuteScan(compiled, &result)
	}

	return result.Count, err
}

func (q *Query) firstInternal(dest any) error {
	if err := q.checkBuilderError(); err != nil {
		return err
	}

	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr || destValue.IsNil() {
		return fmt.Errorf("destination must be a pointer")
	}
	if destValue.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("destination must be a pointer to a struct")
	}

	clone := *q
	clone.limit = 1

	if getExecutor, ok := clone.executor.(GetItemExecutor); ok {
		getCompiled, key, ok, err := clone.compileGetItem()
		if err != nil {
			return err
		}
		if ok {
			return getExecutor.ExecuteGetItem(getCompiled, key, dest)
		}
	}

	results := reflect.New(reflect.SliceOf(destValue.Elem().Type()))
	if err := clone.allInternal(results.Interface()); err != nil {
		return err
	}

	resultsValue := results.Elem()
	if resultsValue.Len() == 0 {
		return theorydbErrors.ErrItemNotFound
	}

	destValue.Elem().Set(resultsValue.Index(0))
	return nil
}

func (q *Query) firstWithRetry(dest any) error {
	if err := q.checkBuilderError(); err != nil {
		return err
	}

	delay := q.retryConfig.InitialDelay
	maxDelay := 5 * time.Second

	for attempt := 0; attempt <= q.retryConfig.MaxRetries; attempt++ {
		err := q.firstInternal(dest)
		if err == nil {
			return nil
		}

		if !errors.Is(err, theorydbErrors.ErrItemNotFound) {
			return err
		}

		if attempt >= q.retryConfig.MaxRetries {
			return err
		}

		if delay > 0 {
			time.Sleep(delay)
			delay *= 2
			if delay > maxDelay {
				delay = maxDelay
			}
		}
	}

	return theorydbErrors.ErrItemNotFound
}

func (q *Query) allInternal(dest any) error {
	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr || destValue.IsNil() || destValue.Elem().Kind() != reflect.Slice {
		return fmt.Errorf("destination must be a pointer to slice")
	}

	compiled, err := q.Compile()
	if err != nil {
		return err
	}

	if compiled.Operation == operationQuery {
		return q.executor.ExecuteQuery(compiled, dest)
	}
	return q.executor.ExecuteScan(compiled, dest)
}

func (q *Query) allWithRetry(dest any) error {
	if err := q.checkBuilderError(); err != nil {
		return err
	}

	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr || destValue.IsNil() || destValue.Elem().Kind() != reflect.Slice {
		return fmt.Errorf("destination must be a pointer to slice")
	}

	delay := q.retryConfig.InitialDelay
	maxDelay := 5 * time.Second
	var lastErr error

	for attempt := 0; attempt <= q.retryConfig.MaxRetries; attempt++ {
		destValue.Elem().Set(reflect.MakeSlice(destValue.Elem().Type(), 0, 0))

		err := q.allInternal(dest)
		lastErr = err
		switch {
		case err != nil:
			if attempt >= q.retryConfig.MaxRetries {
				return err
			}
		case destValue.Elem().Len() > 0:
			return nil
		case attempt >= q.retryConfig.MaxRetries:
			return nil
		}

		if delay > 0 {
			time.Sleep(delay)
			delay *= 2
			if delay > maxDelay {
				delay = maxDelay
			}
		}
	}

	return lastErr
}

// Update updates specified fields on an item
func (q *Query) Update(fields ...string) error {
	if err := q.checkBuilderError(); err != nil {
		return err
	}

	key, keyErr := q.buildPrimaryKeyMap("update")
	if keyErr != nil {
		return keyErr
	}

	modelValue, err := q.updateModelValue()
	if err != nil {
		return err
	}
	if policyErr := q.rejectWriteOnceMutation("update"); policyErr != nil {
		return policyErr
	}

	policyFields, err := q.updatePolicyFields(modelValue, fields)
	if err != nil {
		return err
	}
	if policyErr := q.rejectProtectedFieldMutation(policyFields, nil); policyErr != nil {
		return policyErr
	}

	builder := q.newBuilder()

	if buildErr := q.buildUpdateExpression(builder, modelValue, fields); buildErr != nil {
		return buildErr
	}

	conditionExpr, names, values, err := q.buildConditionExpression(builder, true, true, false)
	if err != nil {
		return err
	}

	components := builder.Build()
	if components.UpdateExpression == "" {
		return fmt.Errorf("no non-key fields to update")
	}

	compiled := &core.CompiledQuery{
		Operation:                 "UpdateItem",
		TableName:                 q.metadata.TableName(),
		UpdateExpression:          components.UpdateExpression,
		ConditionExpression:       conditionExpr,
		ExpressionAttributeNames:  names,
		ExpressionAttributeValues: values,
	}

	if updateExecutor, ok := q.executor.(UpdateItemExecutor); ok {
		return updateExecutor.ExecuteUpdateItem(compiled, key)
	}

	return fmt.Errorf("executor does not support UpdateItem operation")
}

func (q *Query) updateModelValue() (reflect.Value, error) {
	modelValue := reflect.ValueOf(q.model)
	if modelValue.Kind() == reflect.Ptr {
		if modelValue.IsNil() {
			return reflect.Value{}, fmt.Errorf("model cannot be nil")
		}
		modelValue = modelValue.Elem()
	}
	if modelValue.Kind() != reflect.Struct {
		return reflect.Value{}, fmt.Errorf("model must be a struct or pointer to struct")
	}
	return modelValue, nil
}

func (q *Query) buildUpdateExpression(builder *expr.Builder, modelValue reflect.Value, fields []string) error {
	if q.rawMetadata != nil {
		return q.buildUpdateExpressionFromMetadata(builder, modelValue, fields)
	}
	return q.buildUpdateExpressionFromTags(builder, modelValue, fields)
}

func (q *Query) buildUpdateExpressionFromMetadata(builder *expr.Builder, modelValue reflect.Value, fields []string) error {
	fieldsToUpdate := fields
	if len(fieldsToUpdate) == 0 {
		fieldsToUpdate = q.metadataFieldsToUpdate(modelValue)
	}

	for _, fieldName := range fieldsToUpdate {
		fieldMeta, err := q.updateFieldMetadata(fieldName)
		if err != nil {
			return err
		}

		switch {
		case fieldMeta.IsPK || fieldMeta.IsSK:
			return fmt.Errorf("field '%s' is part of the primary key and cannot be updated", fieldName)
		case fieldMeta.IsCreatedAt:
			continue
		case fieldMeta.IsUpdatedAt, fieldMeta.IsVersion:
			continue // handled below
		}

		fieldValue := modelValue.FieldByIndex(fieldMeta.IndexPath)
		if fieldMeta.OmitEmpty && reflectutil.IsEmpty(fieldValue) {
			if err := builder.AddUpdateRemove(fieldMeta.DBName); err != nil {
				return fmt.Errorf("failed to build removal for %s: %w", fieldName, err)
			}
			continue
		}
		valueToSet := fieldValue.Interface()
		if fieldcodec.HasJSONTag(fieldMeta.Tags) {
			normalized, err := fieldcodec.NormalizeJSONReflectValue(fieldMeta.Type, fieldValue)
			if err != nil {
				return fmt.Errorf("failed to normalize json field %s: %w", fieldName, err)
			}
			valueToSet = normalized
		}
		if err := builder.AddUpdateSet(fieldMeta.DBName, valueToSet); err != nil {
			return fmt.Errorf("failed to build update for %s: %w", fieldName, err)
		}
	}

	return q.appendUpdatedAtAndVersionUpdates(builder, modelValue)
}

func (q *Query) metadataFieldsToUpdate(modelValue reflect.Value) []string {
	fieldsToUpdate := make([]string, 0, len(q.rawMetadata.Fields))
	for fieldName, fieldMeta := range q.rawMetadata.Fields {
		if fieldMeta == nil || fieldMeta.IsPK || fieldMeta.IsSK || fieldMeta.IsCreatedAt || fieldMeta.IsUpdatedAt || fieldMeta.IsVersion {
			continue
		}
		fieldValue := modelValue.FieldByIndex(fieldMeta.IndexPath)
		if fieldMeta.OmitEmpty && reflectutil.IsSparseUpdateEmpty(fieldValue) {
			continue
		}
		fieldsToUpdate = append(fieldsToUpdate, fieldName)
	}
	return fieldsToUpdate
}

func (q *Query) updateFieldMetadata(fieldName string) (*model.FieldMetadata, error) {
	fieldMeta, ok := q.rawMetadata.Fields[fieldName]
	if !ok {
		fieldMeta, ok = q.rawMetadata.FieldsByDBName[fieldName]
	}
	if !ok || fieldMeta == nil {
		return nil, fmt.Errorf("field '%s' not found in model metadata (use Go field name or DB attribute name)", fieldName)
	}
	return fieldMeta, nil
}

func (q *Query) appendUpdatedAtAndVersionUpdates(builder *expr.Builder, modelValue reflect.Value) error {
	if q.rawMetadata.UpdatedAtField != nil {
		if err := builder.AddUpdateSet(q.rawMetadata.UpdatedAtField.DBName, time.Now()); err != nil {
			return fmt.Errorf("failed to build updated_at update: %w", err)
		}
	}

	if q.rawMetadata.VersionField != nil {
		current, err := reflectutil.VersionNumber(modelValue.FieldByIndex(q.rawMetadata.VersionField.IndexPath))
		if err != nil {
			return fmt.Errorf("failed to read current version: %w", err)
		}
		if err := builder.AddConditionExpression(q.rawMetadata.VersionField.DBName, "=", current); err != nil {
			return fmt.Errorf("failed to add version condition: %w", err)
		}
		if err := builder.AddUpdateAdd(q.rawMetadata.VersionField.DBName, int64(1)); err != nil {
			return fmt.Errorf("failed to build version increment: %w", err)
		}
	}

	return nil
}

func (q *Query) buildUpdateExpressionFromTags(builder *expr.Builder, modelValue reflect.Value, fields []string) error {
	if len(fields) > 0 {
		return q.buildUpdateExpressionFromNamedFields(builder, modelValue, fields)
	}

	if q.usesFlatAnonymousEmbedEncoding() {
		return q.buildUpdateExpressionFromTaggedVisibleFields(builder, modelValue, q.metadata.PrimaryKey())
	}

	// Legacy tag-driven update helpers intentionally preserve anonymous embedded
	// struct container writes (for example `BaseObject = {...}`) rather than
	// flattening promoted fields. Focused regressions fence that compatibility
	// shape so embedded-base models do not silently disappear while the public
	// helper surface remains write-compatible.
	primaryKey := q.metadata.PrimaryKey()
	modelType := modelValue.Type()
	for i := 0; i < modelType.NumField(); i++ {
		field := modelType.Field(i)
		if !field.IsExported() {
			continue
		}

		tag := field.Tag.Get("theorydb")
		if shouldSkipUpdateField(field, tag, primaryKey) {
			continue
		}

		fieldValue := modelValue.Field(i)
		if fieldcodec.HasModifier(tag, "omitempty") && reflectutil.IsSparseUpdateEmpty(fieldValue) {
			continue
		}

		attrName := q.resolveAttributeName(field.Name)
		valueToSet := fieldValue.Interface()
		if fieldcodec.HasJSONModifier(tag) {
			normalized, err := fieldcodec.NormalizeJSONReflectValue(field.Type, fieldValue)
			if err != nil {
				return fmt.Errorf("failed to normalize json field %s: %w", field.Name, err)
			}
			valueToSet = normalized
		}
		if err := builder.AddUpdateSet(attrName, valueToSet); err != nil {
			return fmt.Errorf("failed to build update for %s: %w", field.Name, err)
		}
	}
	return nil
}

func (q *Query) buildUpdateExpressionFromNamedFields(builder *expr.Builder, modelValue reflect.Value, fields []string) error {
	for _, field := range fields {
		fieldValue, fieldStruct, ok := q.findVisibleFieldByNames(modelValue, field)
		if !ok {
			return fmt.Errorf("field %s not found in model", field)
		}
		tag := fieldStruct.Tag.Get("theorydb")
		attrName := q.resolveMatchedFieldAttributeName(fieldStruct)
		if fieldcodec.HasModifier(tag, "omitempty") && reflectutil.IsEmpty(fieldValue) {
			if err := builder.AddUpdateRemove(attrName); err != nil {
				return fmt.Errorf("failed to build removal for %s: %w", field, err)
			}
			continue
		}
		valueToSet := fieldValue.Interface()
		if fieldcodec.HasJSONModifier(tag) {
			normalized, err := fieldcodec.NormalizeJSONReflectValue(fieldStruct.Type, fieldValue)
			if err != nil {
				return fmt.Errorf("failed to normalize json field %s: %w", field, err)
			}
			valueToSet = normalized
		}
		if err := builder.AddUpdateSet(attrName, valueToSet); err != nil {
			return fmt.Errorf("failed to build update for %s: %w", field, err)
		}
	}
	return nil
}

func extractKeyValues(primaryKey core.KeySchema, conditions []Condition) (map[string]any, error) {
	keyValues := make(map[string]any)
	for _, cond := range conditions {
		if cond.Field != primaryKey.PartitionKey &&
			(primaryKey.SortKey == "" || cond.Field != primaryKey.SortKey) {
			continue
		}
		if cond.Operator != "=" {
			return nil, fmt.Errorf("key condition must use '=' operator")
		}
		keyValues[cond.Field] = cond.Value
	}
	return keyValues, nil
}

func validateKeyValues(primaryKey core.KeySchema, keyValues map[string]any, operation string) error {
	if _, ok := keyValues[primaryKey.PartitionKey]; !ok {
		return fmt.Errorf("partition key %s is required for %s", primaryKey.PartitionKey, operation)
	}
	if primaryKey.SortKey == "" {
		return nil
	}
	if _, ok := keyValues[primaryKey.SortKey]; !ok {
		return fmt.Errorf("sort key %s is required for %s", primaryKey.SortKey, operation)
	}
	return nil
}

func shouldSkipUpdateField(field reflect.StructField, tag string, primaryKey core.KeySchema) bool {
	if tag == "-" {
		return true
	}
	if field.Name == primaryKey.PartitionKey || field.Name == primaryKey.SortKey {
		return true
	}
	if fieldcodec.HasKeyRoleModifier(tag, "pk") || fieldcodec.HasKeyRoleModifier(tag, "sk") {
		return true
	}
	return fieldcodec.HasModifier(tag, "created_at")
}

// Delete deletes an item
func (q *Query) Delete() error {
	if err := q.checkBuilderError(); err != nil {
		return err
	}
	if err := q.rejectWriteOnceMutation("delete"); err != nil {
		return err
	}

	key, keyErr := q.buildPrimaryKeyMap("delete")
	if keyErr != nil {
		return keyErr
	}

	builder := q.newBuilder()
	if q.rawMetadata != nil && q.rawMetadata.VersionField != nil && q.model != nil {
		modelValue := reflect.ValueOf(q.model)
		if modelValue.Kind() == reflect.Ptr && !modelValue.IsNil() {
			modelValue = modelValue.Elem()
		}

		if modelValue.Kind() == reflect.Struct {
			versionValue := modelValue.FieldByIndex(q.rawMetadata.VersionField.IndexPath)
			if !versionValue.IsZero() {
				current, err := reflectutil.VersionNumber(versionValue)
				if err != nil {
					return fmt.Errorf("failed to read current version: %w", err)
				}
				if err := builder.AddConditionExpression(q.rawMetadata.VersionField.DBName, "=", current); err != nil {
					return fmt.Errorf("failed to add version condition: %w", err)
				}
			}
		}
	}

	conditionExpr, condNames, condValues, err := q.buildConditionExpression(builder, true, true, false)
	if err != nil {
		return err
	}

	compiled := &core.CompiledQuery{
		Operation:                 "DeleteItem",
		TableName:                 q.metadata.TableName(),
		ConditionExpression:       conditionExpr,
		ExpressionAttributeNames:  condNames,
		ExpressionAttributeValues: condValues,
	}

	if deleteExecutor, ok := q.executor.(DeleteItemExecutor); ok {
		return deleteExecutor.ExecuteDeleteItem(compiled, key)
	}

	return fmt.Errorf("executor does not support DeleteItem operation")
}

// Scan performs a table scan
func (q *Query) Scan(dest any) error {
	if err := q.checkBuilderError(); err != nil {
		return err
	}
	compiled, err := q.compileScan()
	if err != nil {
		return err
	}

	return q.executor.ExecuteScan(compiled, dest)
}

// ParallelScan performs a parallel table scan with the specified segment
func (q *Query) ParallelScan(segment int32, totalSegments int32) core.Query {
	q.segment = &segment
	q.totalSegments = &totalSegments
	return q
}

// ScanAllSegments performs a parallel scan across all segments and combines results
func (q *Query) ScanAllSegments(dest any, totalSegments int32) error {
	if err := q.checkBuilderError(); err != nil {
		return err
	}
	// Validate destination is a slice pointer
	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr || destValue.Elem().Kind() != reflect.Slice {
		return fmt.Errorf("destination must be a pointer to slice")
	}
	sliceType := destValue.Elem().Type()

	// Create a channel to collect results from each segment
	type segmentResult struct {
		err   error
		items []any
	}

	results := make(chan segmentResult, totalSegments)

	// Launch goroutines for each segment
	for i := int32(0); i < totalSegments; i++ {
		go func(segment int32) {
			// Create a new query for this segment
			segmentQuery := &Query{
				builderErr:     q.builderErr,
				model:          q.model,
				conditions:     q.conditions,
				filters:        q.filters,
				rawFilters:     q.rawFilters,
				index:          q.index,
				limit:          q.limit,
				offset:         q.offset,
				projection:     q.projection,
				orderBy:        q.orderBy,
				exclusive:      q.exclusive,
				consistentRead: q.consistentRead,
				ctx:            q.ctx,
				metadata:       q.metadata,
				rawMetadata:    q.rawMetadata,
				converter:      q.converter,
				marshaler:      q.marshaler,
				executor:       q.executor,
				builder:        q.builder,
				segment:        &segment,
				totalSegments:  &totalSegments,
			}

			// Create a slice to hold this segment's results
			elemType := sliceType.Elem()
			segmentDest := reflect.New(reflect.SliceOf(elemType))

			// Execute scan for this segment
			err := segmentQuery.Scan(segmentDest.Interface())
			if err != nil {
				results <- segmentResult{err: err}
				return
			}

			// Convert results to []any
			segmentSlice := segmentDest.Elem()
			items := make([]any, segmentSlice.Len())
			for j := 0; j < segmentSlice.Len(); j++ {
				items[j] = segmentSlice.Index(j).Interface()
			}

			results <- segmentResult{items: items}
		}(i)
	}

	// Collect results from all segments
	var allItems []any
	for i := int32(0); i < totalSegments; i++ {
		result := <-results
		if result.err != nil {
			return result.err
		}
		allItems = append(allItems, result.items...)
	}

	// Combine all results into the destination slice
	destSlice := destValue.Elem()
	newSlice := reflect.MakeSlice(destSlice.Type(), len(allItems), len(allItems))

	for i, item := range allItems {
		newSlice.Index(i).Set(reflect.ValueOf(item))
	}

	destSlice.Set(newSlice)
	return nil
}

// BatchCreate creates multiple items.
func (q *Query) BatchCreate(items any) error {
	return q.batchCreateWithOptionsInternal(items, nil)
}

// batchCreateWithOptionsInternal is the shared implementation behind
// BatchCreate and BatchCreateWithResult. With a nil opts it preserves the
// legacy fail-fast behavior: the first marshal or write error aborts the
// operation. With opts provided, item-level failures are delivered through
// opts.ErrorHandler (returning nil skips the item and keeps processing) and
// cumulative progress through opts.ProgressCallback.
//
// The execution path is strictly sequential: chunks are written one at a time
// on the caller's goroutine and executeBatchWriteWithRetries retries
// synchronously, so user callbacks are never invoked concurrently and need no
// extra synchronization.
func (q *Query) batchCreateWithOptionsInternal(items any, opts *BatchUpdateOptions) error {
	if err := q.checkBuilderError(); err != nil {
		return err
	}
	if err := q.rejectWriteOnceMutation("batch create"); err != nil {
		return err
	}
	if err := q.rejectProtectedOverwrite("batch create"); err != nil {
		return err
	}
	// Validate items is a slice
	itemsValue := reflect.ValueOf(items)
	if itemsValue.Kind() != reflect.Slice {
		return errors.New("items must be a slice")
	}

	if itemsValue.Len() == 0 {
		return nil
	}

	// Try to use the new BatchWriteItemExecutor first
	if _, ok := q.executor.(BatchWriteItemExecutor); ok {
		return q.batchCreateWithBatchWriteItemExecutor(itemsValue, opts)
	}

	// Fall back to old BatchExecutor for backward compatibility
	if executor, ok := q.executor.(BatchExecutor); ok {
		return q.batchCreateWithLegacyBatchExecutor(itemsValue, opts, executor)
	}

	return errors.New("executor does not support batch operations")
}

// batchCreateWithBatchWriteItemExecutor writes items in chunks of 25 through
// the BatchWriteItemExecutor path. Marshal failures are routed through the
// error handler (with nil opts the first one aborts); a failed chunk write
// reports only the successfully-marshaled items of the chunk as failed so
// Failed/Errors stay item-accurate (marshal-failed items are not reported a
// second time).
func (q *Query) batchCreateWithBatchWriteItemExecutor(itemsValue reflect.Value, opts *BatchUpdateOptions) error {
	tableName := q.metadata.TableName()
	const batchSize = 25
	totalItems := itemsValue.Len()
	processed := 0

	for i := 0; i < totalItems; i += batchSize {
		end := i + batchSize
		if end > totalItems {
			end = totalItems
		}

		// Marshal the chunk. With options, a marshal failure is delivered to
		// the error handler and the item skipped so the rest of the chunk
		// still gets written; without options the first failure aborts,
		// matching the legacy BatchCreate behavior.
		marshaledItems := make([]any, 0, end-i)
		writeRequests := make([]types.WriteRequest, 0, end-i)
		for j := i; j < end; j++ {
			item := itemsValue.Index(j).Interface()

			av, err := q.marshalItem(item)
			if err != nil {
				wrapped := fmt.Errorf("failed to marshal item %d: %w", j, err)
				if handlerErr := handleBatchUpdateError(opts, item, err, wrapped); handlerErr != nil {
					return handlerErr
				}
				continue
			}

			// Mirror the legacy path's `created` shape: only items that were
			// successfully marshaled can be part of the chunk write, so only
			// they are candidates for a chunk-write failure report.
			marshaledItems = append(marshaledItems, item)
			writeRequests = append(writeRequests, types.WriteRequest{
				PutRequest: &types.PutRequest{
					Item: av,
				},
			})
		}

		// A failed batch write leaves every successfully-marshaled item of the
		// chunk unwritten; report each one through the error handler. Marshal-
		// failed items were already reported above and are not reported again.
		// With nil opts the first failure still aborts.
		if err := q.executeBatchWriteWithRetries(tableName, writeRequests, nil); err != nil {
			for _, item := range marshaledItems {
				if handlerErr := handleBatchUpdateError(opts, item, err, err); handlerErr != nil {
					return handlerErr
				}
			}
		}

		processed += end - i
		if opts != nil && opts.ProgressCallback != nil {
			opts.ProgressCallback(processed, totalItems)
		}
	}

	return nil
}

// batchCreateWithLegacyBatchExecutor writes all items in a single request
// through the legacy BatchExecutor interface, applying the same options
// semantics as the BatchWriteItemExecutor path.
func (q *Query) batchCreateWithLegacyBatchExecutor(itemsValue reflect.Value, opts *BatchUpdateOptions, executor BatchExecutor) error {
	batchWrite := &CompiledBatchWrite{
		TableName: q.metadata.TableName(),
		Items:     make([]map[string]types.AttributeValue, 0, itemsValue.Len()),
	}
	created := make([]any, 0, itemsValue.Len())

	// Convert items to AttributeValues
	for i := 0; i < itemsValue.Len(); i++ {
		item := itemsValue.Index(i).Interface()

		av, err := q.marshalItem(item)
		if err != nil {
			wrapped := fmt.Errorf("failed to convert item %d: %w", i, err)
			if handlerErr := handleBatchUpdateError(opts, item, err, wrapped); handlerErr != nil {
				return handlerErr
			}
			continue
		}

		created = append(created, item)
		batchWrite.Items = append(batchWrite.Items, av)
	}

	if err := executor.ExecuteBatchWrite(batchWrite); err != nil {
		for _, item := range created {
			if handlerErr := handleBatchUpdateError(opts, item, err, err); handlerErr != nil {
				return handlerErr
			}
		}
		return nil
	}

	if opts != nil && opts.ProgressCallback != nil {
		opts.ProgressCallback(itemsValue.Len(), itemsValue.Len())
	}
	return nil
}

// WithConverter configures the query to use the provided converter for expression and key/value conversion.
//
// This is optional; when unset, the query falls back to the internal expression converter.
func (q *Query) WithConverter(converter AttributeValueConverter) *Query {
	q.converter = converter
	return q
}

// WithMarshaler configures the query to use the provided marshaler for PutItem-style operations.
//
// This is optional; when unset, the query falls back to reflection-based conversion.
func (q *Query) WithMarshaler(marshaler marshal.MarshalerInterface) *Query {
	q.marshaler = marshaler
	return q
}

func (q *Query) setExecutorContext(ctx context.Context) {
	if ctx == nil {
		return
	}
	if setter, ok := q.executor.(executorContextSetter); ok && setter != nil {
		setter.SetContext(ctx)
	}
}

// WithContext sets the context for the query
func (q *Query) WithContext(ctx context.Context) core.Query {
	if ctx == nil {
		ctx = context.Background()
	}
	q.ctx = ctx
	q.setExecutorContext(ctx)
	return q
}

// selectBestIndex analyzes conditions and selects the optimal index
