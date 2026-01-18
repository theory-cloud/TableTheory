// Package theorydb provides a type-safe ORM for Amazon DynamoDB in Go
package theorydb

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/marshal"
	"github.com/theory-cloud/tabletheory/pkg/model"
	queryPkg "github.com/theory-cloud/tabletheory/pkg/query"
	"github.com/theory-cloud/tabletheory/pkg/schema"
	"github.com/theory-cloud/tabletheory/pkg/session"
	"github.com/theory-cloud/tabletheory/pkg/transaction"
	pkgTypes "github.com/theory-cloud/tabletheory/pkg/types"
)

// DB is the main TableTheory database instance
type DB struct {
	lambdaDeadline      time.Time
	ctx                 context.Context
	session             *session.Session
	registry            *model.Registry
	converter           *pkgTypes.Converter
	marshaler           marshal.MarshalerInterface
	metadataCache       sync.Map
	lambdaTimeoutBuffer time.Duration
	mu                  sync.RWMutex
}

// UnmarshalItem unmarshals a DynamoDB AttributeValue map into a Go struct.
// This is the recommended way to unmarshal DynamoDB stream records or any
// DynamoDB items when using TableTheory.
//
// The function respects TableTheory struct tags (theorydb:"pk", theorydb:"attr:name", etc.)
// and handles all DynamoDB attribute types correctly.
//
// Example usage with DynamoDB Streams:
//
//	func processDynamoDBStream(record events.DynamoDBEventRecord) (*MyModel, error) {
//	    image := record.Change.NewImage
//	    if image == nil {
//	        return nil, nil
//	    }
//
//	    var model MyModel
//	    if err := theorydb.UnmarshalItem(image, &model); err != nil {
//	        return nil, fmt.Errorf("failed to unmarshal: %w", err)
//	    }
//
//	    return &model, nil
//	}
func UnmarshalItem(item map[string]types.AttributeValue, dest interface{}) error {
	// Use the internal unmarshalItem function from the query executor
	return queryPkg.UnmarshalItem(item, dest)
}

// UnmarshalItems unmarshals a slice of DynamoDB AttributeValue maps into a slice of Go structs.
// This is useful for batch operations or when processing multiple items from a query result.
func UnmarshalItems(items []map[string]types.AttributeValue, dest interface{}) error {
	// Use the internal unmarshalItems function from the query executor
	return queryPkg.UnmarshalItems(items, dest)
}

// UnmarshalStreamImage unmarshals a DynamoDB stream image (from Lambda events) into a Go struct.
// This function handles the conversion from Lambda's events.DynamoDBAttributeValue to the standard types.AttributeValue
// and then unmarshals into your TableTheory model.
//
// Example usage:
//
//	func handleStream(record events.DynamoDBEventRecord) error {
//	    var order Order
//	    if err := theorydb.UnmarshalStreamImage(record.Change.NewImage, &order); err != nil {
//	        return err
//	    }
//	    // Process order...
//	}
func UnmarshalStreamImage(streamImage map[string]events.DynamoDBAttributeValue, dest interface{}) error {
	// Convert Lambda event AttributeValues to SDK v2 AttributeValues
	item := make(map[string]types.AttributeValue, len(streamImage))
	for k, v := range streamImage {
		item[k] = convertLambdaAttributeValue(v)
	}

	return UnmarshalItem(item, dest)
}

// convertLambdaAttributeValue converts a Lambda event AttributeValue to SDK v2 AttributeValue
func convertLambdaAttributeValue(attr events.DynamoDBAttributeValue) types.AttributeValue {
	switch attr.DataType() {
	case events.DataTypeString:
		return &types.AttributeValueMemberS{Value: attr.String()}
	case events.DataTypeNumber:
		return &types.AttributeValueMemberN{Value: attr.Number()}
	case events.DataTypeBinary:
		return &types.AttributeValueMemberB{Value: attr.Binary()}
	case events.DataTypeBoolean:
		return &types.AttributeValueMemberBOOL{Value: attr.Boolean()}
	case events.DataTypeNull:
		return &types.AttributeValueMemberNULL{Value: true}
	case events.DataTypeList:
		list := make([]types.AttributeValue, 0, len(attr.List()))
		for _, item := range attr.List() {
			list = append(list, convertLambdaAttributeValue(item))
		}
		return &types.AttributeValueMemberL{Value: list}
	case events.DataTypeMap:
		m := make(map[string]types.AttributeValue)
		for k, v := range attr.Map() {
			m[k] = convertLambdaAttributeValue(v)
		}
		return &types.AttributeValueMemberM{Value: m}
	case events.DataTypeStringSet:
		return &types.AttributeValueMemberSS{Value: attr.StringSet()}
	case events.DataTypeNumberSet:
		return &types.AttributeValueMemberNS{Value: attr.NumberSet()}
	case events.DataTypeBinarySet:
		return &types.AttributeValueMemberBS{Value: attr.BinarySet()}
	default:
		// This shouldn't happen, but return NULL if unknown type
		return &types.AttributeValueMemberNULL{Value: true}
	}
}

// New creates a new TableTheory instance with the given configuration
func New(config session.Config) (core.ExtendedDB, error) {
	sess, err := session.NewSession(&config)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	converter := pkgTypes.NewConverter()
	marshalerFactory := marshal.NewMarshalerFactory(marshal.DefaultConfig()).WithConverter(converter)
	if config.Now != nil {
		marshalerFactory = marshalerFactory.WithNowFunc(config.Now)
	}
	marshalerInstance, err := marshalerFactory.NewMarshaler()
	if err != nil {
		return nil, fmt.Errorf("failed to create marshaler: %w", err)
	}

	return &DB{
		session:   sess,
		registry:  model.NewRegistry(),
		converter: converter,
		marshaler: marshalerInstance,
		ctx:       context.Background(),
	}, nil
}

// NewBasic creates a new TableTheory instance that returns the basic DB interface
// Use this when you only need core functionality and want easier mocking
func NewBasic(config session.Config) (core.DB, error) {
	return New(config)
}

// RegisterTypeConverter registers a custom converter for a specific Go type. This allows
// callers to control how values are marshaled to and unmarshaled from DynamoDB without
// forking the internal marshaler. Registering a converter clears any cached marshalers
// so subsequent operations use the new logic.
func (db *DB) RegisterTypeConverter(typ reflect.Type, converter pkgTypes.CustomConverter) error {
	if typ == nil {
		return fmt.Errorf("converter type cannot be nil")
	}
	if converter == nil {
		return fmt.Errorf("converter implementation cannot be nil")
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	db.converter.RegisterConverter(typ, converter)
	if db.marshaler != nil {
		type cacheClearer interface {
			ClearCache()
		}
		if clearer, ok := db.marshaler.(cacheClearer); ok && clearer != nil {
			clearer.ClearCache()
		}
	}
	return nil
}

// Model returns a new query builder for the given model
func (db *DB) Model(model any) core.Query {
	// Ensure model is registered
	if err := db.registry.Register(model); err != nil {
		// Log the error for debugging
		if db.ctx != nil {
			// Include context info if available
			return &errorQuery{err: fmt.Errorf("failed to register model %T: %w", model, err)}
		}
		// Return a query that will error on execution
		return &errorQuery{err: fmt.Errorf("failed to register model %T: %w", model, err)}
	}

	// Fast-path metadata lookup - cache for later use
	typ := reflect.TypeOf(model)
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	// Check cache first
	if _, ok := db.metadataCache.Load(typ); !ok {
		// Get from registry and cache
		meta, err := db.registry.GetMetadata(model)
		if err != nil {
			return &errorQuery{err: fmt.Errorf("failed to get metadata for model %T: %w", model, err)}
		}
		db.metadataCache.Store(typ, meta)
	}

	// Use the context from the DB if query doesn't have one
	ctx := db.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	meta, err := db.registry.GetMetadata(model)
	if err != nil {
		return &errorQuery{err: fmt.Errorf("failed to get metadata for model %T: %w", model, err)}
	}

	adapter := &metadataAdapter{metadata: meta}
	executor := &queryExecutor{
		db:       db,
		metadata: meta,
		ctx:      ctx,
	}

	q := queryPkg.New(model, adapter, executor).
		WithConverter(db.converter).
		WithMarshaler(db.marshaler)
	q.WithContext(ctx)
	return q
}

// Transaction executes a function within a database transaction
func (db *DB) Transaction(fn func(tx *core.Tx) error) error {
	// For now, we'll use a simple wrapper that doesn't support full transaction features
	// Users should use TransactionFunc for full transaction support
	tx := &core.Tx{}
	// Set the db field to avoid nil pointer panic
	tx.SetDB(db)
	return fn(tx)
}

// Transact returns a fluent transaction builder for composing TransactWriteItems requests.
func (db *DB) Transact() core.TransactionBuilder {
	builder := transaction.NewBuilder(db.session, db.registry, db.converter)
	if db.ctx != nil {
		builder.WithContext(db.ctx)
	}
	return builder
}

// TransactWrite executes the supplied function with a transaction builder and automatically commits it.
func (db *DB) TransactWrite(ctx context.Context, fn func(core.TransactionBuilder) error) error {
	if fn == nil {
		return fmt.Errorf("transaction function cannot be nil")
	}

	builder := db.Transact()
	if ctx != nil {
		builder = builder.WithContext(ctx)
	} else if db.ctx != nil {
		builder = builder.WithContext(db.ctx)
	}

	if err := fn(builder); err != nil {
		return err
	}

	return builder.Execute()
}

// AutoMigrate creates or updates tables based on the given models
func (db *DB) AutoMigrate(models ...any) error {
	manager := schema.NewManager(db.session, db.registry)

	for _, model := range models {
		if err := db.registry.Register(model); err != nil {
			return fmt.Errorf("failed to register model %T: %w", model, err)
		}

		// Check if table exists, create if not
		metadata, err := db.registry.GetMetadata(model)
		if err != nil {
			return err
		}

		exists, err := manager.TableExists(metadata.TableName)
		if err != nil {
			return fmt.Errorf("failed to check table existence: %w", err)
		}

		if !exists {
			if err := manager.CreateTable(model); err != nil {
				return fmt.Errorf("failed to create table for %T: %w", model, err)
			}
		}
	}

	return nil
}

// AutoMigrateWithOptions performs enhanced auto-migration with data copy support
func (db *DB) AutoMigrateWithOptions(model any, opts ...any) error {
	// Convert opts to the expected type
	var options []schema.AutoMigrateOption
	for _, opt := range opts {
		if option, ok := opt.(schema.AutoMigrateOption); ok {
			options = append(options, option)
		} else {
			return fmt.Errorf("invalid option type: expected schema.AutoMigrateOption, got %T", opt)
		}
	}

	manager := schema.NewManager(db.session, db.registry)
	return manager.AutoMigrateWithOptions(model, options...)
}

// CreateTable creates a DynamoDB table for the given model
func (db *DB) CreateTable(model any, opts ...any) error {
	// Register model first
	if err := db.registry.Register(model); err != nil {
		return fmt.Errorf("failed to register model %T: %w", model, err)
	}

	// Convert opts to the expected type
	var options []schema.TableOption
	for _, opt := range opts {
		if option, ok := opt.(schema.TableOption); ok {
			options = append(options, option)
		} else {
			return fmt.Errorf("invalid option type: expected schema.TableOption, got %T", opt)
		}
	}

	manager := schema.NewManager(db.session, db.registry)
	return manager.CreateTable(model, options...)
}

// EnsureTable checks if a table exists for the model and creates it if not
func (db *DB) EnsureTable(model any) error {
	// Register model first
	if err := db.registry.Register(model); err != nil {
		return fmt.Errorf("failed to register model %T: %w", model, err)
	}

	metadata, err := db.registry.GetMetadata(model)
	if err != nil {
		return err
	}

	manager := schema.NewManager(db.session, db.registry)
	exists, err := manager.TableExists(metadata.TableName)
	if err != nil {
		return fmt.Errorf("failed to check table existence: %w", err)
	}

	if !exists {
		return manager.CreateTable(model)
	}

	return nil
}

// DeleteTable deletes the DynamoDB table for the given model
func (db *DB) DeleteTable(model any) error {
	if tableName, ok := model.(string); ok {
		manager := schema.NewManager(db.session, db.registry)
		return manager.DeleteTable(tableName)
	}

	// Register model first
	if err := db.registry.Register(model); err != nil {
		return fmt.Errorf("failed to register model %T: %w", model, err)
	}

	metadata, err := db.registry.GetMetadata(model)
	if err != nil {
		return err
	}

	manager := schema.NewManager(db.session, db.registry)
	return manager.DeleteTable(metadata.TableName)
}

// DescribeTable returns the table description for the given model
func (db *DB) DescribeTable(model any) (any, error) {
	// Register model first
	if err := db.registry.Register(model); err != nil {
		return nil, fmt.Errorf("failed to register model %T: %w", model, err)
	}

	manager := schema.NewManager(db.session, db.registry)
	return manager.DescribeTable(model)
}

// Close closes the database connection
func (db *DB) Close() error {
	// AWS SDK v2 clients don't need explicit closing
	return nil
}

// Migrate runs all pending migrations
func (db *DB) Migrate() error {
	// TableTheory doesn't support traditional migrations
	// Use infrastructure as code tools like Terraform or CloudFormation instead
	return fmt.Errorf("TableTheory does not support migrations. Use infrastructure as code tools (Terraform, CloudFormation) or AutoMigrate for development")
}

// WithContext returns a new DB instance with the given context
func (db *DB) WithContext(ctx context.Context) core.DB {
	db.mu.RLock()
	defer db.mu.RUnlock()

	newDB := &DB{
		session:             db.session,
		registry:            db.registry,
		converter:           db.converter,
		marshaler:           db.marshaler,
		ctx:                 ctx,
		lambdaDeadline:      db.lambdaDeadline,
		lambdaTimeoutBuffer: db.lambdaTimeoutBuffer,
	}

	// Copy metadata cache
	db.metadataCache.Range(func(key, value any) bool {
		newDB.metadataCache.Store(key, value)
		return true
	})

	return newDB
}

// WithLambdaTimeout sets a deadline based on Lambda context
func (db *DB) WithLambdaTimeout(ctx context.Context) core.DB {
	deadline, ok := ctx.Deadline()
	if !ok {
		return db
	}

	// Leave a buffer for Lambda cleanup
	buffer := db.lambdaTimeoutBuffer
	if buffer == 0 {
		buffer = 500 * time.Millisecond // Default buffer
	}
	adjustedDeadline := deadline.Add(-buffer)

	db.mu.RLock()
	defer db.mu.RUnlock()

	newDB := &DB{
		session:             db.session,
		registry:            db.registry,
		converter:           db.converter,
		marshaler:           db.marshaler,
		ctx:                 ctx,
		lambdaDeadline:      adjustedDeadline,
		lambdaTimeoutBuffer: db.lambdaTimeoutBuffer,
	}

	// Copy metadata cache
	db.metadataCache.Range(func(key, value any) bool {
		newDB.metadataCache.Store(key, value)
		return true
	})

	return newDB
}

// WithLambdaTimeoutBuffer sets a custom timeout buffer for Lambda execution
func (db *DB) WithLambdaTimeoutBuffer(buffer time.Duration) core.DB {
	db.mu.RLock()
	defer db.mu.RUnlock()

	// Create new instance instead of modifying existing one to avoid race conditions
	newDB := &DB{
		session:             db.session,
		registry:            db.registry,
		converter:           db.converter,
		marshaler:           db.marshaler,
		ctx:                 db.ctx,
		lambdaDeadline:      db.lambdaDeadline,
		lambdaTimeoutBuffer: buffer, // Set the new buffer value
	}

	// Copy metadata cache
	db.metadataCache.Range(func(key, value any) bool {
		newDB.metadataCache.Store(key, value)
		return true
	})

	return newDB
}
