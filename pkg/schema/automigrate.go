package schema

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/theory-cloud/tabletheory/internal/numutil"
	"github.com/theory-cloud/tabletheory/pkg/model"
)

// AutoMigrateOptions holds configuration for AutoMigrate operations
type AutoMigrateOptions struct {
	TargetModel any
	Transform   interface{}
	Context     context.Context
	BackupTable string
	BatchSize   int
	DataCopy    bool
}

// AutoMigrateOption is a function that configures AutoMigrateOptions
type AutoMigrateOption func(*AutoMigrateOptions)

// WithBackupTable sets a backup table name
func WithBackupTable(tableName string) AutoMigrateOption {
	return func(opts *AutoMigrateOptions) {
		opts.BackupTable = tableName
	}
}

// WithDataCopy enables data copying
func WithDataCopy(enable bool) AutoMigrateOption {
	return func(opts *AutoMigrateOptions) {
		opts.DataCopy = enable
	}
}

// WithTargetModel sets a different target model for migration
func WithTargetModel(model any) AutoMigrateOption {
	return func(opts *AutoMigrateOptions) {
		opts.TargetModel = model
	}
}

// WithTransform sets a transformation function for data migration
func WithTransform(transform interface{}) AutoMigrateOption {
	return func(opts *AutoMigrateOptions) {
		opts.Transform = transform
	}
}

// WithBatchSize sets the batch size for data copy operations
func WithBatchSize(size int) AutoMigrateOption {
	return func(opts *AutoMigrateOptions) {
		opts.BatchSize = size
	}
}

// WithContext sets the context for the operation
func WithContext(ctx context.Context) AutoMigrateOption {
	return func(opts *AutoMigrateOptions) {
		opts.Context = ctx
	}
}

// AutoMigrateWithOptions performs an enhanced auto-migration with data copy support
func (m *Manager) AutoMigrateWithOptions(sourceModel any, options ...AutoMigrateOption) error {
	opts := newAutoMigrateOptions(options)

	sourceMetadata, targetModel, targetMetadata, err := m.resolveAutoMigrateModels(sourceModel, opts.TargetModel)
	if err != nil {
		return err
	}

	if err := m.createBackupIfRequested(opts.Context, sourceMetadata.TableName, opts.BackupTable); err != nil {
		return err
	}

	if err := m.ensureTargetTable(targetModel, targetMetadata.TableName); err != nil {
		return err
	}

	return m.copyDataIfRequested(opts, sourceMetadata, targetMetadata)
}

func newAutoMigrateOptions(options []AutoMigrateOption) *AutoMigrateOptions {
	opts := &AutoMigrateOptions{
		BatchSize: 25,
		Context:   context.Background(),
	}
	for _, opt := range options {
		opt(opts)
	}
	return opts
}

func (m *Manager) resolveAutoMigrateModels(sourceModel any, targetOverride any) (*model.Metadata, any, *model.Metadata, error) {
	if err := m.registry.Register(sourceModel); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to register source model: %w", err)
	}

	sourceMetadata, err := m.registry.GetMetadata(sourceModel)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get source metadata: %w", err)
	}

	targetModel := sourceModel
	if targetOverride != nil {
		targetModel = targetOverride
		if err = m.registry.Register(targetModel); err != nil {
			return nil, nil, nil, fmt.Errorf("failed to register target model: %w", err)
		}
	}

	targetMetadata, err := m.registry.GetMetadata(targetModel)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get target metadata: %w", err)
	}

	return sourceMetadata, targetModel, targetMetadata, nil
}

func (m *Manager) createBackupIfRequested(ctx context.Context, sourceTable string, backupTable string) error {
	if backupTable == "" {
		return nil
	}
	if err := m.createBackup(ctx, sourceTable, backupTable); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}
	return nil
}

func (m *Manager) ensureTargetTable(targetModel any, tableName string) error {
	exists, err := m.TableExists(tableName)
	if err != nil {
		return fmt.Errorf("failed to check target table existence: %w", err)
	}

	if !exists {
		if err := m.CreateTable(targetModel); err != nil {
			return fmt.Errorf("failed to create target table: %w", err)
		}
	}

	return nil
}

func (m *Manager) copyDataIfRequested(opts *AutoMigrateOptions, sourceMetadata, targetMetadata *model.Metadata) error {
	if !opts.DataCopy || sourceMetadata.TableName == targetMetadata.TableName {
		return nil
	}

	var transformFunc TransformFunc
	if opts.Transform != nil {
		var err error
		transformFunc, err = CreateModelTransform(opts.Transform, sourceMetadata, targetMetadata)
		if err != nil {
			return fmt.Errorf("invalid transform function: %w", err)
		}
	}

	if err := m.copyData(opts, sourceMetadata, targetMetadata, transformFunc); err != nil {
		return fmt.Errorf("failed to copy data: %w", err)
	}

	return nil
}

// createBackup creates a point-in-time backup of the table
func (m *Manager) createBackup(ctx context.Context, sourceTable, backupName string) error {
	// Check if source table exists
	exists, err := m.TableExists(sourceTable)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("source table %s does not exist", sourceTable)
	}

	// Create backup using DynamoDB's backup feature
	backupRequest := &dynamodb.CreateBackupInput{
		TableName:  &sourceTable,
		BackupName: &backupName,
	}

	client, err := m.session.Client()
	if err != nil {
		return fmt.Errorf("failed to get client for backup creation: %w", err)
	}

	_, err = client.CreateBackup(ctx, backupRequest)
	if err != nil {
		// If backup fails, try table copy instead
		return m.copyTable(ctx, sourceTable, backupName)
	}

	return nil
}

// copyTable creates a copy of a table
func (m *Manager) copyTable(ctx context.Context, sourceTable, targetTable string) error {
	// Get source table description
	client, err := m.session.Client()
	if err != nil {
		return fmt.Errorf("failed to get client for table description: %w", err)
	}

	// Check if target table already exists and delete it
	exists, err := m.TableExists(targetTable)
	if err != nil {
		return fmt.Errorf("failed to check if backup table exists: %w", err)
	}
	if exists {
		// Delete existing backup table
		_, err = client.DeleteTable(ctx, &dynamodb.DeleteTableInput{
			TableName: &targetTable,
		})
		if err != nil {
			return fmt.Errorf("failed to delete existing backup table: %w", err)
		}

		// Wait for table to be deleted
		waiter := dynamodb.NewTableNotExistsWaiter(client)
		if waitErr := waiter.Wait(ctx, &dynamodb.DescribeTableInput{
			TableName: &targetTable,
		}, 2*time.Minute); waitErr != nil {
			return fmt.Errorf("timeout waiting for table deletion: %w", waitErr)
		}
	}

	desc, err := client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: &sourceTable,
	})
	if err != nil {
		return fmt.Errorf("failed to describe source table: %w", err)
	}

	// Create target table with same schema
	createInput := &dynamodb.CreateTableInput{
		TableName:            &targetTable,
		KeySchema:            desc.Table.KeySchema,
		AttributeDefinitions: desc.Table.AttributeDefinitions,
		BillingMode:          desc.Table.BillingModeSummary.BillingMode,
	}

	// Convert GlobalSecondaryIndexDescriptions to GlobalSecondaryIndexes
	if len(desc.Table.GlobalSecondaryIndexes) > 0 {
		gsis := make([]types.GlobalSecondaryIndex, len(desc.Table.GlobalSecondaryIndexes))
		for i, gsi := range desc.Table.GlobalSecondaryIndexes {
			gsis[i] = types.GlobalSecondaryIndex{
				IndexName:  gsi.IndexName,
				KeySchema:  gsi.KeySchema,
				Projection: gsi.Projection,
			}
			if gsi.ProvisionedThroughput != nil {
				gsis[i].ProvisionedThroughput = &types.ProvisionedThroughput{
					ReadCapacityUnits:  gsi.ProvisionedThroughput.ReadCapacityUnits,
					WriteCapacityUnits: gsi.ProvisionedThroughput.WriteCapacityUnits,
				}
			}
		}
		createInput.GlobalSecondaryIndexes = gsis
	}

	// Convert LocalSecondaryIndexDescriptions to LocalSecondaryIndexes
	if len(desc.Table.LocalSecondaryIndexes) > 0 {
		lsis := make([]types.LocalSecondaryIndex, len(desc.Table.LocalSecondaryIndexes))
		for i, lsi := range desc.Table.LocalSecondaryIndexes {
			lsis[i] = types.LocalSecondaryIndex{
				IndexName:  lsi.IndexName,
				KeySchema:  lsi.KeySchema,
				Projection: lsi.Projection,
			}
		}
		createInput.LocalSecondaryIndexes = lsis
	}

	if desc.Table.ProvisionedThroughput != nil {
		createInput.ProvisionedThroughput = &types.ProvisionedThroughput{
			ReadCapacityUnits:  desc.Table.ProvisionedThroughput.ReadCapacityUnits,
			WriteCapacityUnits: desc.Table.ProvisionedThroughput.WriteCapacityUnits,
		}
	}

	_, err = client.CreateTable(ctx, createInput)
	if err != nil {
		return fmt.Errorf("failed to create target table: %w", err)
	}

	// Wait for table to be active
	waiter := dynamodb.NewTableExistsWaiter(client)
	if err := waiter.Wait(ctx, &dynamodb.DescribeTableInput{
		TableName: &targetTable,
	}, 5*time.Minute); err != nil {
		return fmt.Errorf("timeout waiting for table creation: %w", err)
	}

	// Copy data
	return m.copyTableData(ctx, sourceTable, targetTable, 25)
}

// copyData copies data from source to target table with optional transformation
func (m *Manager) copyData(opts *AutoMigrateOptions, sourceMetadata, targetMetadata *model.Metadata, transformFunc TransformFunc) error {
	ctx := opts.Context

	// Get client once for the entire operation
	client, err := m.session.Client()
	if err != nil {
		return fmt.Errorf("failed to get client for data copy: %w", err)
	}

	// Scan source table
	var lastEvaluatedKey map[string]types.AttributeValue
	for {
		scanInput := &dynamodb.ScanInput{
			TableName: &sourceMetadata.TableName,
			Limit:     int32Ptr(numutil.ClampIntToInt32(opts.BatchSize)),
		}
		if lastEvaluatedKey != nil {
			scanInput.ExclusiveStartKey = lastEvaluatedKey
		}

		result, err := client.Scan(ctx, scanInput)
		if err != nil {
			return fmt.Errorf("failed to scan source table: %w", err)
		}

		// Process items
		if len(result.Items) > 0 {
			if err := m.processItems(ctx, client, result.Items, targetMetadata.TableName, transformFunc, sourceMetadata, targetMetadata); err != nil {
				return fmt.Errorf("failed to process items: %w", err)
			}
		}

		// Check if more items
		lastEvaluatedKey = result.LastEvaluatedKey
		if lastEvaluatedKey == nil {
			break
		}
	}

	return nil
}

// processItems processes and writes items to the target table
func (m *Manager) processItems(ctx context.Context, client *dynamodb.Client, items []map[string]types.AttributeValue,
	targetTable string,
	transformFunc TransformFunc,
	sourceMetadata, targetMetadata *model.Metadata,
) error {
	writeRequests, err := buildPutWriteRequestsWithTransform(items, transformFunc, sourceMetadata, targetMetadata)
	if err != nil {
		return err
	}

	const (
		maxBatchSize = 25
		maxRetries   = 5
	)

	return writeRequestsBatched(ctx, client, targetTable, writeRequests, maxBatchSize, maxRetries)
}

// copyTableData copies all data from source to target table
func (m *Manager) copyTableData(ctx context.Context, sourceTable, targetTable string, batchSize int) error {
	client, err := m.session.Client()
	if err != nil {
		return fmt.Errorf("failed to get client for table data copy: %w", err)
	}

	var lastEvaluatedKey map[string]types.AttributeValue

	maxBatchSize := resolveDataCopyBatchSize(batchSize)

	for {
		result, err := scanTablePage(ctx, client, sourceTable, batchSize, lastEvaluatedKey)
		if err != nil {
			return err
		}

		if err := writeItemsToTable(ctx, client, targetTable, result.Items, maxBatchSize); err != nil {
			return err
		}

		// Check if more items
		lastEvaluatedKey = result.LastEvaluatedKey
		if lastEvaluatedKey == nil {
			break
		}
	}

	return nil
}

func resolveDataCopyBatchSize(batchSize int) int {
	const maxLocalBatchSize = 10
	if batchSize <= 0 {
		return maxLocalBatchSize
	}
	if batchSize < maxLocalBatchSize {
		return batchSize
	}
	return maxLocalBatchSize
}

func scanTablePage(
	ctx context.Context,
	client *dynamodb.Client,
	sourceTable string,
	batchSize int,
	lastEvaluatedKey map[string]types.AttributeValue,
) (*dynamodb.ScanOutput, error) {
	scanInput := &dynamodb.ScanInput{
		TableName: &sourceTable,
		Limit:     int32Ptr(numutil.ClampIntToInt32(batchSize)),
	}
	if lastEvaluatedKey != nil {
		scanInput.ExclusiveStartKey = lastEvaluatedKey
	}

	result, err := client.Scan(ctx, scanInput)
	if err != nil {
		return nil, fmt.Errorf("failed to scan source table: %w", err)
	}

	return result, nil
}

func writeItemsToTable(
	ctx context.Context,
	client *dynamodb.Client,
	targetTable string,
	items []map[string]types.AttributeValue,
	maxBatchSize int,
) error {
	if len(items) == 0 {
		return nil
	}

	const maxRetries = 5
	writeRequests := buildPutWriteRequests(items)
	return writeRequestsBatched(ctx, client, targetTable, writeRequests, maxBatchSize, maxRetries)
}

func buildPutWriteRequests(items []map[string]types.AttributeValue) []types.WriteRequest {
	writeRequests := make([]types.WriteRequest, 0, len(items))
	for _, item := range items {
		writeRequests = append(writeRequests, types.WriteRequest{
			PutRequest: &types.PutRequest{
				Item: item,
			},
		})
	}
	return writeRequests
}

func buildPutWriteRequestsWithTransform(
	items []map[string]types.AttributeValue,
	transform TransformFunc,
	sourceMetadata, targetMetadata *model.Metadata,
) ([]types.WriteRequest, error) {
	if transform == nil {
		return buildPutWriteRequests(items), nil
	}

	writeRequests := make([]types.WriteRequest, 0, len(items))
	for _, item := range items {
		transformedItem, err := TransformWithValidation(item, transform, sourceMetadata, targetMetadata)
		if err != nil {
			return nil, fmt.Errorf("failed to transform item: %w", err)
		}

		writeRequests = append(writeRequests, types.WriteRequest{
			PutRequest: &types.PutRequest{
				Item: transformedItem,
			},
		})
	}

	return writeRequests, nil
}

func writeRequestsBatched(
	ctx context.Context,
	client *dynamodb.Client,
	tableName string,
	writeRequests []types.WriteRequest,
	maxBatchSize int,
	maxRetries int,
) error {
	if len(writeRequests) == 0 {
		return nil
	}
	if maxBatchSize <= 0 {
		maxBatchSize = len(writeRequests)
	}

	for i := 0; i < len(writeRequests); i += maxBatchSize {
		end := i + maxBatchSize
		if end > len(writeRequests) {
			end = len(writeRequests)
		}

		batch := writeRequests[i:end]
		remaining, err := batchWriteWithRetries(ctx, client, tableName, batch, maxRetries)
		if err != nil {
			return err
		}
		if len(remaining) == 0 {
			continue
		}

		if err := putWriteRequestsIndividually(ctx, client, tableName, remaining); err != nil {
			return err
		}
	}

	return nil
}

func batchWriteWithRetries(
	ctx context.Context,
	client *dynamodb.Client,
	tableName string,
	writeRequests []types.WriteRequest,
	maxRetries int,
) ([]types.WriteRequest, error) {
	if len(writeRequests) == 0 {
		return nil, nil
	}
	if maxRetries <= 0 {
		return writeRequests, nil
	}

	remainingRequests := writeRequests
	for attempt := 1; attempt <= maxRetries; attempt++ {
		batchInput := &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]types.WriteRequest{
				tableName: remainingRequests,
			},
		}

		result, err := client.BatchWriteItem(ctx, batchInput)
		if err != nil {
			return nil, fmt.Errorf("failed to write batch: %w", err)
		}

		remainingRequests = unprocessedRequestsForTable(result.UnprocessedItems, tableName)
		if len(remainingRequests) == 0 {
			return nil, nil
		}

		if attempt == maxRetries {
			break
		}

		if err := sleepWithBackoff(ctx, attempt); err != nil {
			return nil, err
		}
	}

	return remainingRequests, nil
}

func unprocessedRequestsForTable(unprocessedItems map[string][]types.WriteRequest, tableName string) []types.WriteRequest {
	if len(unprocessedItems) == 0 {
		return nil
	}
	unprocessed, exists := unprocessedItems[tableName]
	if !exists || len(unprocessed) == 0 {
		return nil
	}
	return unprocessed
}

func sleepWithBackoff(ctx context.Context, retryCount int) error {
	backoff := time.Duration(retryCount*retryCount) * 100 * time.Millisecond
	timer := time.NewTimer(backoff)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("context canceled during retry: %w", ctx.Err())
	}
}

func putWriteRequestsIndividually(
	ctx context.Context,
	client *dynamodb.Client,
	tableName string,
	remainingRequests []types.WriteRequest,
) error {
	for _, req := range remainingRequests {
		if req.PutRequest == nil {
			continue
		}

		putInput := &dynamodb.PutItemInput{
			TableName: &tableName,
			Item:      req.PutRequest.Item,
		}
		if _, err := client.PutItem(ctx, putInput); err != nil {
			return fmt.Errorf("failed to put individual item after batch failures: %w", err)
		}
	}

	return nil
}

// Helper function
func int32Ptr(i int32) *int32 {
	return &i
}
