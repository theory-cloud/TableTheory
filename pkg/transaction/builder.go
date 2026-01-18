package transaction

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"

	"github.com/theory-cloud/tabletheory/internal/encryption"
	"github.com/theory-cloud/tabletheory/internal/expr"
	"github.com/theory-cloud/tabletheory/pkg/core"
	customerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/model"
	"github.com/theory-cloud/tabletheory/pkg/query"
	"github.com/theory-cloud/tabletheory/pkg/session"
	pkgTypes "github.com/theory-cloud/tabletheory/pkg/types"
)

const (
	maxTransactOperations = 25
)

var retrySchedule = []time.Duration{
	100 * time.Millisecond,
	200 * time.Millisecond,
	400 * time.Millisecond,
	800 * time.Millisecond,
}

type dynamoTransactAPI interface {
	TransactWriteItems(ctx context.Context, params *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error)
}

// Builder implements the core.TransactionBuilder interface.
type Builder struct {
	client     dynamoTransactAPI
	ctx        context.Context
	err        error
	session    *session.Session
	registry   *model.Registry
	converter  *pkgTypes.Converter
	operations []transactOperation
}

type operationType int

const (
	opPut operationType = iota
	opCreate
	opUpdate
	opUpdateWithBuilder
	opDelete
	opConditionCheck
)

type transactOperation struct {
	model      any
	metadata   *model.Metadata
	updateFn   func(core.UpdateBuilder) error
	fields     []string
	conditions []core.TransactCondition
	typ        operationType
}

type rawCondition struct {
	values     map[string]types.AttributeValue
	expression string
}

// NewBuilder creates a new transaction builder backed by the provided session, registry, and converter.
func NewBuilder(sess *session.Session, registry *model.Registry, converter *pkgTypes.Converter) *Builder {
	return &Builder{
		session:   sess,
		registry:  registry,
		converter: converter,
		ctx:       context.Background(),
	}
}

// Put schedules a put (upsert) operation.
func (b *Builder) Put(model any, conditions ...core.TransactCondition) core.TransactionBuilder {
	b.addOperation(opPut, model, nil, nil, conditions)
	return b
}

// Create schedules a conditional put guarded by attribute_not_exists on the primary key.
func (b *Builder) Create(model any, conditions ...core.TransactCondition) core.TransactionBuilder {
	cond := append([]core.TransactCondition{
		{Kind: core.TransactConditionKindPrimaryKeyNotExists},
	}, conditions...)
	b.addOperation(opCreate, model, nil, nil, cond)
	return b
}

// Update schedules a partial update using the provided fields.
func (b *Builder) Update(model any, fields []string, conditions ...core.TransactCondition) core.TransactionBuilder {
	if len(fields) == 0 {
		b.recordError(errors.New("transaction update requires at least one field"))
		return b
	}
	fieldCopy := append([]string(nil), fields...)
	b.addOperation(opUpdate, model, fieldCopy, nil, conditions)
	return b
}

// UpdateWithBuilder schedules a complex update using the UpdateBuilder DSL.
func (b *Builder) UpdateWithBuilder(model any, updateFn func(core.UpdateBuilder) error, conditions ...core.TransactCondition) core.TransactionBuilder {
	if updateFn == nil {
		b.recordError(errors.New("update builder function cannot be nil"))
		return b
	}
	b.addOperation(opUpdateWithBuilder, model, nil, updateFn, conditions)
	return b
}

// Delete schedules a delete operation.
func (b *Builder) Delete(model any, conditions ...core.TransactCondition) core.TransactionBuilder {
	b.addOperation(opDelete, model, nil, nil, conditions)
	return b
}

// ConditionCheck schedules a condition check.
func (b *Builder) ConditionCheck(model any, conditions ...core.TransactCondition) core.TransactionBuilder {
	if len(conditions) == 0 {
		b.recordError(errors.New("condition check requires at least one condition"))
		return b
	}
	b.addOperation(opConditionCheck, model, nil, nil, conditions)
	return b
}

// WithContext sets the execution context for the transaction.
func (b *Builder) WithContext(ctx context.Context) core.TransactionBuilder {
	if ctx == nil {
		b.ctx = context.Background()
	} else {
		b.ctx = ctx
	}
	return b
}

// Execute commits the transaction using the builder's configured context.
func (b *Builder) Execute() error {
	return b.ExecuteWithContext(b.ctx)
}

// ExecuteWithContext commits the transaction with an explicit context override.
func (b *Builder) ExecuteWithContext(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	if b.err != nil {
		return b.err
	}
	if len(b.operations) == 0 {
		return errors.New("transaction has no operations")
	}

	items, err := b.materializeOperations()
	if err != nil {
		return err
	}

	input := &dynamodb.TransactWriteItemsInput{
		TransactItems:      items,
		ClientRequestToken: aws.String(uuid.NewString()),
	}

	if err := b.executeWithRetry(ctx, input); err != nil {
		return err
	}

	// Allow builder reuse after successful execution
	b.operations = nil
	return nil
}

func (b *Builder) addOperation(opType operationType, model any, fields []string, updateFn func(core.UpdateBuilder) error, conditions []core.TransactCondition) {
	if b.err != nil {
		return
	}
	if model == nil {
		b.recordError(errors.New("model cannot be nil"))
		return
	}
	if len(b.operations) >= maxTransactOperations {
		b.recordError(fmt.Errorf("dynamodb transactions support up to %d operations", maxTransactOperations))
		return
	}

	if err := b.registry.Register(model); err != nil {
		b.recordError(err)
		return
	}

	metadata, err := b.registry.GetMetadata(model)
	if err != nil {
		b.recordError(err)
		return
	}

	b.operations = append(b.operations, transactOperation{
		typ:        opType,
		model:      model,
		metadata:   metadata,
		fields:     fields,
		updateFn:   updateFn,
		conditions: cloneTransactConditions(conditions),
	})
}

func (b *Builder) recordError(err error) {
	if err != nil && b.err == nil {
		b.err = err
	}
}

func (b *Builder) materializeOperations() ([]types.TransactWriteItem, error) {
	items := make([]types.TransactWriteItem, 0, len(b.operations))

	for idx, op := range b.operations {
		item, err := b.buildWriteItem(idx, op)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, nil
}

func (b *Builder) buildWriteItem(index int, op transactOperation) (types.TransactWriteItem, error) {
	switch op.typ {
	case opPut, opCreate:
		put, err := b.buildPut(op)
		if err != nil {
			return types.TransactWriteItem{}, err
		}
		return types.TransactWriteItem{Put: put}, nil
	case opUpdate:
		update, err := b.buildFieldUpdate(op)
		if err != nil {
			return types.TransactWriteItem{}, err
		}
		return types.TransactWriteItem{Update: update}, nil
	case opUpdateWithBuilder:
		update, err := b.buildBuilderUpdate(op, index)
		if err != nil {
			return types.TransactWriteItem{}, err
		}
		return types.TransactWriteItem{Update: update}, nil
	case opDelete:
		del, err := b.buildDelete(op)
		if err != nil {
			return types.TransactWriteItem{}, err
		}
		return types.TransactWriteItem{Delete: del}, nil
	case opConditionCheck:
		check, err := b.buildConditionCheck(op)
		if err != nil {
			return types.TransactWriteItem{}, err
		}
		return types.TransactWriteItem{ConditionCheck: check}, nil
	default:
		return types.TransactWriteItem{}, fmt.Errorf("unsupported transaction operation: %d", op.typ)
	}
}

func (b *Builder) buildPut(op transactOperation) (*types.Put, error) {
	tx := &Transaction{
		session:   b.session,
		registry:  b.registry,
		converter: b.converter,
	}

	item, err := tx.marshalItem(op.model, op.metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal item for put: %w", err)
	}

	builder := expr.NewBuilderWithConverter(b.converter)

	rawConds, err := b.applyConditionsToBuilder(op.metadata, builder, op.conditions)
	if err != nil {
		return nil, err
	}

	components := builder.Build()
	exprStr, names, values, err := b.mergeRawConditions(components.ConditionExpression, components.ExpressionAttributeNames, components.ExpressionAttributeValues, rawConds)
	if err != nil {
		return nil, err
	}

	put := &types.Put{
		TableName: aws.String(op.metadata.TableName),
		Item:      item,
	}

	if exprStr != "" {
		put.ConditionExpression = aws.String(exprStr)
	}
	if len(names) > 0 {
		put.ExpressionAttributeNames = names
	}
	if len(values) > 0 {
		put.ExpressionAttributeValues = values
	}

	return put, nil
}

func (b *Builder) buildFieldUpdate(op transactOperation) (*types.Update, error) {
	tx := &Transaction{
		session:   b.session,
		registry:  b.registry,
		converter: b.converter,
	}

	key, err := tx.extractPrimaryKey(op.model, op.metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to extract primary key: %w", err)
	}

	value := reflect.ValueOf(op.model)
	if value.Kind() == reflect.Ptr {
		value = value.Elem()
	}

	builder := expr.NewBuilderWithConverter(b.converter)
	for _, field := range op.fields {
		fieldMeta := op.metadata.Fields[field]
		if fieldMeta == nil {
			return nil, fmt.Errorf("unknown field %s for update", field)
		}
		fieldValue := value.Field(fieldMeta.Index)
		if !fieldValue.IsValid() {
			return nil, fmt.Errorf("field %s is invalid", field)
		}
		if err := builder.AddUpdateSet(fieldMeta.DBName, fieldValue.Interface()); err != nil {
			return nil, fmt.Errorf("failed to build update for %s: %w", field, err)
		}
	}

	rawConds, err := b.applyConditionsToBuilder(op.metadata, builder, op.conditions)
	if err != nil {
		return nil, err
	}

	components := builder.Build()
	if components.UpdateExpression == "" {
		return nil, errors.New("update expression cannot be empty")
	}

	conditionExpr, names, values, err := b.mergeRawConditions(components.ConditionExpression, components.ExpressionAttributeNames, components.ExpressionAttributeValues, rawConds)
	if err != nil {
		return nil, err
	}

	if encryption.MetadataHasEncryptedFields(op.metadata) {
		if err := encryption.FailClosedIfEncryptedWithoutKMSKeyARN(b.session, op.metadata); err != nil {
			return nil, err
		}
		cfg := b.session.Config()
		keyARN := ""
		var rng io.Reader
		if cfg != nil {
			keyARN = cfg.KMSKeyARN
			rng = cfg.EncryptionRand
		}
		var svc *encryption.Service
		if cfg != nil && cfg.KMSClient != nil {
			svc = encryption.NewServiceWithRand(keyARN, cfg.KMSClient, rng)
		} else {
			svc = encryption.NewServiceFromAWSConfigWithRand(keyARN, b.session.AWSConfig(), rng)
		}
		ctx := b.ctx
		if ctx == nil {
			ctx = context.Background()
		}
		if err := encryption.EncryptUpdateExpressionValues(ctx, svc, op.metadata, components.UpdateExpression, names, values); err != nil {
			return nil, err
		}
	}

	update := &types.Update{
		TableName:        aws.String(op.metadata.TableName),
		Key:              key,
		UpdateExpression: aws.String(components.UpdateExpression),
	}

	if conditionExpr != "" {
		update.ConditionExpression = aws.String(conditionExpr)
	}
	if len(names) > 0 {
		update.ExpressionAttributeNames = names
	}
	if len(values) > 0 {
		update.ExpressionAttributeValues = values
	}

	return update, nil
}

func (b *Builder) buildBuilderUpdate(op transactOperation, index int) (*types.Update, error) {
	capture := &capturingUpdateExecutor{}
	q := query.New(op.model, adaptMetadata(op.metadata), capture)

	if err := b.populateKeyConditions(q, op.metadata, op.model); err != nil {
		return nil, err
	}

	builderConds, rawConds, err := b.partitionBuilderConditions(op.metadata, op.conditions)
	if err != nil {
		return nil, err
	}

	ubInterface := query.NewUpdateBuilder(q)
	ubImpl, ok := ubInterface.(*query.UpdateBuilder)
	if !ok {
		return nil, fmt.Errorf("unsupported update builder type %T", ubInterface)
	}

	if err := b.applyConditionsToUpdateBuilder(ubImpl, op.metadata, builderConds); err != nil {
		return nil, err
	}

	if err := op.updateFn(ubImpl); err != nil {
		return nil, err
	}

	if !capture.executed {
		if err := ubImpl.Execute(); err != nil {
			return nil, err
		}
	}

	if capture.compiled == nil {
		return nil, fmt.Errorf("update builder for operation %d did not produce an expression", index)
	}

	update := &types.Update{
		TableName: aws.String(op.metadata.TableName),
		Key:       capture.key,
	}

	if capture.compiled.UpdateExpression == "" {
		return nil, errors.New("update builder produced empty update expression")
	}
	update.UpdateExpression = aws.String(capture.compiled.UpdateExpression)

	if capture.compiled.ConditionExpression != "" {
		update.ConditionExpression = aws.String(capture.compiled.ConditionExpression)
	}

	if len(capture.compiled.ExpressionAttributeNames) > 0 {
		update.ExpressionAttributeNames = capture.compiled.ExpressionAttributeNames
	}

	if len(capture.compiled.ExpressionAttributeValues) > 0 {
		update.ExpressionAttributeValues = capture.compiled.ExpressionAttributeValues
	}

	if len(rawConds) > 0 {
		conditionExpr := ""
		if update.ConditionExpression != nil {
			conditionExpr = aws.ToString(update.ConditionExpression)
		}

		names := update.ExpressionAttributeNames
		values := update.ExpressionAttributeValues

		mergedExpr, mergedNames, mergedValues, err := b.mergeRawConditions(conditionExpr, names, values, rawConds)
		if err != nil {
			return nil, err
		}

		if mergedExpr != "" {
			update.ConditionExpression = aws.String(mergedExpr)
		}
		if len(mergedNames) > 0 {
			update.ExpressionAttributeNames = mergedNames
		}
		if len(mergedValues) > 0 {
			update.ExpressionAttributeValues = mergedValues
		}
	}

	if encryption.MetadataHasEncryptedFields(op.metadata) && update.UpdateExpression != nil && len(update.ExpressionAttributeValues) > 0 {
		if err := encryption.FailClosedIfEncryptedWithoutKMSKeyARN(b.session, op.metadata); err != nil {
			return nil, err
		}
		cfg := b.session.Config()
		keyARN := ""
		var rng io.Reader
		if cfg != nil {
			keyARN = cfg.KMSKeyARN
			rng = cfg.EncryptionRand
		}
		var svc *encryption.Service
		if cfg != nil && cfg.KMSClient != nil {
			svc = encryption.NewServiceWithRand(keyARN, cfg.KMSClient, rng)
		} else {
			svc = encryption.NewServiceFromAWSConfigWithRand(keyARN, b.session.AWSConfig(), rng)
		}
		ctx := b.ctx
		if ctx == nil {
			ctx = context.Background()
		}
		if err := encryption.EncryptUpdateExpressionValues(ctx, svc, op.metadata, aws.ToString(update.UpdateExpression), update.ExpressionAttributeNames, update.ExpressionAttributeValues); err != nil {
			return nil, err
		}
	}

	return update, nil
}

func (b *Builder) buildDelete(op transactOperation) (*types.Delete, error) {
	tx := &Transaction{
		session:   b.session,
		registry:  b.registry,
		converter: b.converter,
	}

	key, err := tx.extractPrimaryKey(op.model, op.metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to extract primary key: %w", err)
	}

	builder := expr.NewBuilderWithConverter(b.converter)
	rawConds, err := b.applyConditionsToBuilder(op.metadata, builder, op.conditions)
	if err != nil {
		return nil, err
	}

	components := builder.Build()
	conditionExpr, names, values, err := b.mergeRawConditions(components.ConditionExpression, components.ExpressionAttributeNames, components.ExpressionAttributeValues, rawConds)
	if err != nil {
		return nil, err
	}

	deleteItem := &types.Delete{
		TableName: aws.String(op.metadata.TableName),
		Key:       key,
	}

	if conditionExpr != "" {
		deleteItem.ConditionExpression = aws.String(conditionExpr)
	}
	if len(names) > 0 {
		deleteItem.ExpressionAttributeNames = names
	}
	if len(values) > 0 {
		deleteItem.ExpressionAttributeValues = values
	}

	return deleteItem, nil
}

func (b *Builder) buildConditionCheck(op transactOperation) (*types.ConditionCheck, error) {
	tx := &Transaction{
		session:   b.session,
		registry:  b.registry,
		converter: b.converter,
	}

	key, err := tx.extractPrimaryKey(op.model, op.metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to extract primary key: %w", err)
	}

	builder := expr.NewBuilderWithConverter(b.converter)
	rawConds, err := b.applyConditionsToBuilder(op.metadata, builder, op.conditions)
	if err != nil {
		return nil, err
	}

	components := builder.Build()
	if components.ConditionExpression == "" && len(rawConds) == 0 {
		return nil, errors.New("condition check requires at least one condition")
	}

	conditionExpr, names, values, err := b.mergeRawConditions(components.ConditionExpression, components.ExpressionAttributeNames, components.ExpressionAttributeValues, rawConds)
	if err != nil {
		return nil, err
	}

	check := &types.ConditionCheck{
		TableName: aws.String(op.metadata.TableName),
		Key:       key,
	}

	if conditionExpr != "" {
		check.ConditionExpression = aws.String(conditionExpr)
	}
	if len(names) > 0 {
		check.ExpressionAttributeNames = names
	}
	if len(values) > 0 {
		check.ExpressionAttributeValues = values
	}

	return check, nil
}

func (b *Builder) populateKeyConditions(q *query.Query, metadata *model.Metadata, model any) error {
	value := reflect.ValueOf(model)
	if value.Kind() == reflect.Ptr {
		value = value.Elem()
	}

	if metadata.PrimaryKey == nil || metadata.PrimaryKey.PartitionKey == nil {
		return errors.New("model is missing primary key metadata")
	}

	pkMeta := metadata.PrimaryKey.PartitionKey
	pkValue := value.Field(pkMeta.Index)
	if !pkValue.IsValid() || pkValue.IsZero() {
		return fmt.Errorf("partition key %s is required", pkMeta.Name)
	}

	q.Where(pkMeta.Name, "=", pkValue.Interface())

	if metadata.PrimaryKey.SortKey != nil {
		skMeta := metadata.PrimaryKey.SortKey
		skValue := value.Field(skMeta.Index)
		if !skValue.IsValid() || skValue.IsZero() {
			return fmt.Errorf("sort key %s is required", skMeta.Name)
		}
		q.Where(skMeta.Name, "=", skValue.Interface())
	}

	return nil
}

func (b *Builder) applyConditionsToBuilder(metadata *model.Metadata, builder *expr.Builder, conditions []core.TransactCondition) ([]rawCondition, error) {
	raw := make([]rawCondition, 0)
	for _, cond := range conditions {
		kind := cond.Kind
		if kind == "" {
			kind = core.TransactConditionKindField
		}

		switch kind {
		case core.TransactConditionKindField:
			attrName, err := b.resolveAttributeName(metadata, cond.Field)
			if err != nil {
				return nil, err
			}
			if cond.Operator == "" {
				return nil, fmt.Errorf("operator required for condition on %s", cond.Field)
			}
			if err := builder.AddConditionExpression(attrName, cond.Operator, cond.Value); err != nil {
				return nil, err
			}
		case core.TransactConditionKindPrimaryKeyExists:
			if err := b.addPrimaryKeyCondition(metadata, builder, "attribute_exists"); err != nil {
				return nil, err
			}
		case core.TransactConditionKindPrimaryKeyNotExists:
			if err := b.addPrimaryKeyCondition(metadata, builder, "attribute_not_exists"); err != nil {
				return nil, err
			}
		case core.TransactConditionKindVersionEquals:
			fieldName, err := b.versionAttributeName(metadata)
			if err != nil {
				return nil, err
			}
			if err := builder.AddConditionExpression(fieldName, "=", cond.Value); err != nil {
				return nil, err
			}
		case core.TransactConditionKindExpression:
			rc, err := b.buildRawCondition(cond)
			if err != nil {
				return nil, err
			}
			raw = append(raw, rc)
		default:
			return nil, fmt.Errorf("unsupported transaction condition type %s", cond.Kind)
		}
	}
	return raw, nil
}

func (b *Builder) resolveAttributeName(metadata *model.Metadata, field string) (string, error) {
	if metadata == nil {
		return field, nil
	}
	if field == "" {
		return "", errors.New("condition field cannot be empty")
	}

	if meta, ok := metadata.Fields[field]; ok && meta != nil && meta.DBName != "" {
		return meta.DBName, nil
	}
	if meta, ok := metadata.FieldsByDBName[field]; ok && meta != nil {
		return meta.DBName, nil
	}
	return field, nil
}

func (b *Builder) versionAttributeName(metadata *model.Metadata) (string, error) {
	if metadata.VersionField != nil && metadata.VersionField.DBName != "" {
		return metadata.VersionField.DBName, nil
	}
	if metadata.VersionField != nil {
		return metadata.VersionField.Name, nil
	}

	if meta, ok := metadata.Fields["Version"]; ok && meta.DBName != "" {
		return meta.DBName, nil
	}

	return "", errors.New("model does not define a version field")
}

func (b *Builder) addPrimaryKeyCondition(metadata *model.Metadata, builder *expr.Builder, operator string) error {
	if metadata.PrimaryKey == nil || metadata.PrimaryKey.PartitionKey == nil {
		return errors.New("model is missing primary key metadata")
	}

	if err := builder.AddConditionExpression(metadata.PrimaryKey.PartitionKey.DBName, operator, nil); err != nil {
		return err
	}

	if metadata.PrimaryKey.SortKey != nil {
		if err := builder.AddConditionExpression(metadata.PrimaryKey.SortKey.DBName, operator, nil); err != nil {
			return err
		}
	}
	return nil
}

func (b *Builder) buildRawCondition(cond core.TransactCondition) (rawCondition, error) {
	expression := strings.TrimSpace(cond.Expression)
	if expression == "" {
		return rawCondition{}, errors.New("condition expression cannot be empty")
	}

	values := make(map[string]types.AttributeValue, len(cond.Values))
	for key, val := range cond.Values {
		av, err := b.converter.ToAttributeValue(val)
		if err != nil {
			return rawCondition{}, fmt.Errorf("failed to convert condition value for %s: %w", key, err)
		}
		values[key] = av
	}

	return rawCondition{
		expression: expression,
		values:     values,
	}, nil
}

func (b *Builder) mergeRawConditions(expression string, names map[string]string, values map[string]types.AttributeValue, raw []rawCondition) (string, map[string]string, map[string]types.AttributeValue, error) {
	if names == nil {
		names = make(map[string]string)
	}
	if values == nil {
		values = make(map[string]types.AttributeValue)
	}

	resultExpr := expression
	for _, rc := range raw {
		if resultExpr == "" {
			resultExpr = rc.expression
		} else {
			resultExpr = fmt.Sprintf("(%s) AND (%s)", resultExpr, rc.expression)
		}

		for k, v := range rc.values {
			if _, exists := values[k]; exists {
				return "", nil, nil, fmt.Errorf("duplicate condition value placeholder: %s", k)
			}
			values[k] = v
		}
	}

	return resultExpr, names, values, nil
}

func (b *Builder) executeWithRetry(ctx context.Context, input *dynamodb.TransactWriteItemsInput) error {
	var attempt int

	for {
		if b.client == nil {
			if b.session == nil {
				return errors.New("dynamodb session is not configured")
			}
			client, err := b.session.Client()
			if err != nil {
				return err
			}
			b.client = client
		}

		_, err := b.client.TransactWriteItems(ctx, input)
		if err == nil {
			return nil
		}

		retryable, translated := b.translateError(err)
		if !retryable || attempt >= len(retrySchedule) {
			return translated
		}

		sleep := retrySchedule[attempt]
		attempt++

		timer := time.NewTimer(sleep)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
			// retry
		}
	}
}

func (b *Builder) translateError(err error) (bool, error) {
	var canceled *types.TransactionCanceledException
	if errors.As(err, &canceled) {
		retryable := true
		for _, reason := range canceled.CancellationReasons {
			if reason.Code == nil {
				continue
			}
			if !isRetryableReason(*reason.Code) {
				retryable = false
				break
			}
		}
		return retryable, b.buildTransactionError(canceled, err)
	}

	return false, err
}

func (b *Builder) buildTransactionError(exc *types.TransactionCanceledException, original error) error {
	if len(exc.CancellationReasons) == 0 {
		return fmt.Errorf("transaction canceled: %w", original)
	}

	for idx, reason := range exc.CancellationReasons {
		if reason.Code == nil {
			continue
		}

		opName := "unknown"
		if idx < len(b.operations) {
			opName = b.operations[idx].typ.String()
		}
		modelName := "unknown"
		if idx < len(b.operations) && b.operations[idx].model != nil {
			modelName = reflect.TypeOf(b.operations[idx].model).String()
		}

		baseErr := customerrors.ErrTransactionFailed
		if *reason.Code == "ConditionalCheckFailed" {
			baseErr = customerrors.ErrConditionFailed
		}

		return &customerrors.TransactionError{
			OperationIndex: idx,
			Operation:      opName,
			Model:          modelName,
			Reason:         aws.ToString(reason.Message),
			Err:            baseErr,
		}
	}

	return fmt.Errorf("transaction canceled: %w", original)
}

func isRetryableReason(code string) bool {
	switch code {
	case "TransactionConflict", "ProvisionedThroughputExceeded", "ThrottlingError", "InternalServerError":
		return true
	default:
		return false
	}
}

func (t operationType) String() string {
	switch t {
	case opPut:
		return "Put"
	case opCreate:
		return "Create"
	case opUpdate:
		return "Update"
	case opUpdateWithBuilder:
		return "UpdateWithBuilder"
	case opDelete:
		return "Delete"
	case opConditionCheck:
		return "ConditionCheck"
	default:
		return "Unknown"
	}
}

func (b *Builder) partitionBuilderConditions(metadata *model.Metadata, conditions []core.TransactCondition) ([]core.TransactCondition, []rawCondition, error) {
	if len(conditions) == 0 {
		return nil, nil, nil
	}

	builderConds := make([]core.TransactCondition, 0, len(conditions))
	rawConds := make([]rawCondition, 0)

	for _, cond := range conditions {
		kind := cond.Kind
		if kind == "" {
			kind = core.TransactConditionKindField
		}

		if kind == core.TransactConditionKindExpression {
			rc, err := b.buildRawCondition(cond)
			if err != nil {
				return nil, nil, err
			}
			rawConds = append(rawConds, rc)
			continue
		}

		builderConds = append(builderConds, cond)
	}

	return builderConds, rawConds, nil
}

func (b *Builder) applyConditionsToUpdateBuilder(ub *query.UpdateBuilder, metadata *model.Metadata, conditions []core.TransactCondition) error {
	for _, cond := range conditions {
		kind := cond.Kind
		if kind == "" {
			kind = core.TransactConditionKindField
		}

		switch kind {
		case core.TransactConditionKindField:
			fieldName, err := b.resolveFieldName(metadata, cond.Field)
			if err != nil {
				return err
			}
			if cond.Operator == "" {
				return fmt.Errorf("operator required for condition on %s", fieldName)
			}
			ub.Condition(fieldName, cond.Operator, cond.Value)
		case core.TransactConditionKindPrimaryKeyExists:
			if err := b.addBuilderPrimaryKeyCondition(ub, metadata, "attribute_exists"); err != nil {
				return err
			}
		case core.TransactConditionKindPrimaryKeyNotExists:
			if err := b.addBuilderPrimaryKeyCondition(ub, metadata, "attribute_not_exists"); err != nil {
				return err
			}
		case core.TransactConditionKindVersionEquals:
			fieldName, err := b.resolveVersionFieldName(metadata)
			if err != nil {
				return err
			}
			ub.Condition(fieldName, "=", cond.Value)
		default:
			return fmt.Errorf("unsupported condition type %s for update builder", kind)
		}
	}
	return nil
}

func (b *Builder) addBuilderPrimaryKeyCondition(ub *query.UpdateBuilder, metadata *model.Metadata, operator string) error {
	if metadata == nil || metadata.PrimaryKey == nil || metadata.PrimaryKey.PartitionKey == nil {
		return errors.New("model is missing primary key metadata")
	}

	ub.Condition(metadata.PrimaryKey.PartitionKey.Name, operator, nil)
	if metadata.PrimaryKey.SortKey != nil {
		ub.Condition(metadata.PrimaryKey.SortKey.Name, operator, nil)
	}
	return nil
}

func (b *Builder) resolveFieldName(metadata *model.Metadata, field string) (string, error) {
	if metadata == nil {
		return field, nil
	}
	if field == "" {
		return "", errors.New("condition field cannot be empty")
	}

	if meta := metadata.Fields[field]; meta != nil {
		return meta.Name, nil
	}
	if meta := metadata.FieldsByDBName[field]; meta != nil {
		return meta.Name, nil
	}
	return field, nil
}

func (b *Builder) resolveVersionFieldName(metadata *model.Metadata) (string, error) {
	if metadata == nil {
		return "", errors.New("model metadata is required for version condition")
	}
	if metadata.VersionField != nil {
		return metadata.VersionField.Name, nil
	}
	if field := metadata.Fields["Version"]; field != nil {
		return field.Name, nil
	}
	return "", errors.New("model does not define a version field")
}

type metadataAdapter struct {
	meta *model.Metadata
}

func adaptMetadata(meta *model.Metadata) core.ModelMetadata {
	if meta == nil {
		return nil
	}
	return &metadataAdapter{meta: meta}
}

func (m *metadataAdapter) TableName() string {
	if m.meta == nil {
		return ""
	}
	return m.meta.TableName
}

func (m *metadataAdapter) PrimaryKey() core.KeySchema {
	if m.meta == nil || m.meta.PrimaryKey == nil {
		return core.KeySchema{}
	}

	var schema core.KeySchema
	if m.meta.PrimaryKey.PartitionKey != nil {
		schema.PartitionKey = m.meta.PrimaryKey.PartitionKey.Name
	}
	if m.meta.PrimaryKey.SortKey != nil {
		schema.SortKey = m.meta.PrimaryKey.SortKey.Name
	}
	return schema
}

func (m *metadataAdapter) Indexes() []core.IndexSchema {
	if m.meta == nil || len(m.meta.Indexes) == 0 {
		return nil
	}

	indexes := make([]core.IndexSchema, 0, len(m.meta.Indexes))
	for _, idx := range m.meta.Indexes {
		schema := core.IndexSchema{
			Name:           idx.Name,
			Type:           string(idx.Type),
			ProjectionType: idx.ProjectionType,
			ProjectedFields: append(
				[]string(nil),
				idx.ProjectedFields...,
			),
		}
		if idx.PartitionKey != nil {
			schema.PartitionKey = idx.PartitionKey.Name
		}
		if idx.SortKey != nil {
			schema.SortKey = idx.SortKey.Name
		}
		indexes = append(indexes, schema)
	}
	return indexes
}

func (m *metadataAdapter) AttributeMetadata(field string) *core.AttributeMetadata {
	if m.meta == nil {
		return nil
	}

	if attr := m.meta.Fields[field]; attr != nil {
		return convertFieldMetadata(attr)
	}
	if attr := m.meta.FieldsByDBName[field]; attr != nil {
		return convertFieldMetadata(attr)
	}
	return nil
}

func (m *metadataAdapter) VersionFieldName() string {
	if m.meta == nil {
		return ""
	}
	if m.meta.VersionField != nil {
		if m.meta.VersionField.DBName != "" {
			return m.meta.VersionField.DBName
		}
		return m.meta.VersionField.Name
	}
	return ""
}

func convertFieldMetadata(field *model.FieldMetadata) *core.AttributeMetadata {
	if field == nil {
		return nil
	}

	var typeName string
	if field.Type != nil {
		typeName = field.Type.String()
	}

	meta := &core.AttributeMetadata{
		Name:         field.Name,
		Type:         typeName,
		DynamoDBName: field.DBName,
	}
	if len(field.Tags) > 0 {
		meta.Tags = make(map[string]string, len(field.Tags))
		for k, v := range field.Tags {
			meta.Tags[k] = v
		}
	}
	return meta
}

func cloneTransactConditions(conds []core.TransactCondition) []core.TransactCondition {
	if len(conds) == 0 {
		return nil
	}
	cloned := make([]core.TransactCondition, len(conds))
	for i, cond := range conds {
		copyCond := cond
		if cond.Values != nil {
			copyCond.Values = make(map[string]any, len(cond.Values))
			for k, v := range cond.Values {
				copyCond.Values[k] = v
			}
		}
		cloned[i] = copyCond
	}
	return cloned
}

type capturingUpdateExecutor struct {
	compiled     *core.CompiledQuery
	key          map[string]types.AttributeValue
	compilations int
	executed     bool
}

func (c *capturingUpdateExecutor) ExecuteQuery(*core.CompiledQuery, any) error {
	return errors.New("query execution not supported in transaction builder")
}

func (c *capturingUpdateExecutor) ExecuteScan(*core.CompiledQuery, any) error {
	return errors.New("scan execution not supported in transaction builder")
}

func (c *capturingUpdateExecutor) ExecuteUpdateItem(input *core.CompiledQuery, key map[string]types.AttributeValue) error {
	c.compilations++
	c.executed = true
	c.compiled = input
	c.key = key
	return nil
}

func (c *capturingUpdateExecutor) ExecuteUpdateItemWithResult(input *core.CompiledQuery, key map[string]types.AttributeValue) (*core.UpdateResult, error) {
	if err := c.ExecuteUpdateItem(input, key); err != nil {
		return nil, err
	}
	return &core.UpdateResult{}, nil
}
