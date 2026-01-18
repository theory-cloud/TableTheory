package theorydb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/model"
	"github.com/theory-cloud/tabletheory/pkg/schema"
	"github.com/theory-cloud/tabletheory/pkg/session"
	"github.com/theory-cloud/tabletheory/pkg/transaction"
)

// errorQuery is a query that always returns an error.
type errorQuery struct {
	err error
}

func (e *errorQuery) Where(_ string, _ string, _ any) core.Query  { return e }
func (e *errorQuery) Index(_ string) core.Query                   { return e }
func (e *errorQuery) Filter(_ string, _ string, _ any) core.Query { return e }
func (e *errorQuery) OrFilter(_ string, _ string, _ any) core.Query {
	return e
}
func (e *errorQuery) FilterGroup(_ func(q core.Query)) core.Query { return e }
func (e *errorQuery) OrFilterGroup(_ func(core.Query)) core.Query {
	return e
}
func (e *errorQuery) IfNotExists() core.Query { return e }
func (e *errorQuery) IfExists() core.Query    { return e }
func (e *errorQuery) WithCondition(_ string, _ string, _ any) core.Query {
	return e
}
func (e *errorQuery) WithConditionExpression(_ string, _ map[string]any) core.Query {
	return e
}
func (e *errorQuery) OrderBy(_ string, _ string) core.Query       { return e }
func (e *errorQuery) Limit(_ int) core.Query                      { return e }
func (e *errorQuery) Offset(_ int) core.Query                     { return e }
func (e *errorQuery) Select(_ ...string) core.Query               { return e }
func (e *errorQuery) ConsistentRead() core.Query                  { return e }
func (e *errorQuery) WithRetry(_ int, _ time.Duration) core.Query { return e }
func (e *errorQuery) First(_ any) error                           { return e.err }
func (e *errorQuery) All(_ any) error                             { return e.err }
func (e *errorQuery) Count() (int64, error)                       { return 0, e.err }
func (e *errorQuery) Create() error                               { return e.err }
func (e *errorQuery) CreateOrUpdate() error                       { return e.err }
func (e *errorQuery) Update(_ ...string) error                    { return e.err }
func (e *errorQuery) Delete() error                               { return e.err }
func (e *errorQuery) Scan(_ any) error                            { return e.err }
func (e *errorQuery) BatchGet(_ []any, _ any) error               { return e.err }
func (e *errorQuery) BatchGetWithOptions(_ []any, _ any, _ *core.BatchGetOptions) error {
	return e.err
}
func (e *errorQuery) BatchGetBuilder() core.BatchGetBuilder { return &errorBatchGetBuilder{err: e.err} }
func (e *errorQuery) BatchCreate(_ any) error               { return e.err }
func (e *errorQuery) BatchDelete(_ []any) error             { return e.err }
func (e *errorQuery) BatchWrite(_ []any, _ []any) error     { return e.err }
func (e *errorQuery) BatchUpdateWithOptions(_ []any, _ []string, _ ...any) error {
	return e.err
}
func (e *errorQuery) WithContext(_ context.Context) core.Query          { return e }
func (e *errorQuery) AllPaginated(_ any) (*core.PaginatedResult, error) { return nil, e.err }
func (e *errorQuery) UpdateBuilder() core.UpdateBuilder                 { return &errorUpdateBuilder{err: e.err} }
func (e *errorQuery) ParallelScan(_ int32, _ int32) core.Query          { return e }
func (e *errorQuery) ScanAllSegments(_ any, _ int32) error              { return e.err }
func (e *errorQuery) Cursor(_ string) core.Query                        { return e }
func (e *errorQuery) SetCursor(_ string) error                          { return e.err }

type errorBatchGetBuilder struct {
	err error
}

func (b *errorBatchGetBuilder) Keys(_ []any) core.BatchGetBuilder                  { return b }
func (b *errorBatchGetBuilder) ChunkSize(_ int) core.BatchGetBuilder               { return b }
func (b *errorBatchGetBuilder) ConsistentRead() core.BatchGetBuilder               { return b }
func (b *errorBatchGetBuilder) Parallel(_ int) core.BatchGetBuilder                { return b }
func (b *errorBatchGetBuilder) WithRetry(_ *core.RetryPolicy) core.BatchGetBuilder { return b }
func (b *errorBatchGetBuilder) Select(_ ...string) core.BatchGetBuilder            { return b }
func (b *errorBatchGetBuilder) OnProgress(_ core.BatchProgressCallback) core.BatchGetBuilder {
	return b
}
func (b *errorBatchGetBuilder) OnError(_ core.BatchChunkErrorHandler) core.BatchGetBuilder {
	return b
}
func (b *errorBatchGetBuilder) Execute(_ any) error { return b.err }

type errorUpdateBuilder struct {
	err error
}

func (e *errorUpdateBuilder) Set(_ string, _ any) core.UpdateBuilder { return e }
func (e *errorUpdateBuilder) SetIfNotExists(_ string, _ any, _ any) core.UpdateBuilder {
	return e
}
func (e *errorUpdateBuilder) Add(_ string, _ any) core.UpdateBuilder           { return e }
func (e *errorUpdateBuilder) Increment(_ string) core.UpdateBuilder            { return e }
func (e *errorUpdateBuilder) Decrement(_ string) core.UpdateBuilder            { return e }
func (e *errorUpdateBuilder) Remove(_ string) core.UpdateBuilder               { return e }
func (e *errorUpdateBuilder) Delete(_ string, _ any) core.UpdateBuilder        { return e }
func (e *errorUpdateBuilder) AppendToList(_ string, _ any) core.UpdateBuilder  { return e }
func (e *errorUpdateBuilder) PrependToList(_ string, _ any) core.UpdateBuilder { return e }
func (e *errorUpdateBuilder) RemoveFromListAt(_ string, _ int) core.UpdateBuilder {
	return e
}
func (e *errorUpdateBuilder) SetListElement(_ string, _ int, _ any) core.UpdateBuilder {
	return e
}
func (e *errorUpdateBuilder) Condition(_ string, _ string, _ any) core.UpdateBuilder {
	return e
}
func (e *errorUpdateBuilder) OrCondition(_ string, _ string, _ any) core.UpdateBuilder {
	return e
}
func (e *errorUpdateBuilder) ConditionExists(_ string) core.UpdateBuilder    { return e }
func (e *errorUpdateBuilder) ConditionNotExists(_ string) core.UpdateBuilder { return e }
func (e *errorUpdateBuilder) ConditionVersion(_ int64) core.UpdateBuilder    { return e }
func (e *errorUpdateBuilder) ReturnValues(_ string) core.UpdateBuilder       { return e }
func (e *errorUpdateBuilder) Execute() error                                 { return e.err }
func (e *errorUpdateBuilder) ExecuteWithResult(_ any) error                  { return e.err }

// Re-export types for convenience.
type (
	Config            = session.Config
	AutoMigrateOption = schema.AutoMigrateOption
	BatchGetOptions   = core.BatchGetOptions
	KeyPair           = core.KeyPair
)

// Re-export AutoMigrate options for convenience.
var (
	WithBackupTable = schema.WithBackupTable
	WithDataCopy    = schema.WithDataCopy
	WithTargetModel = schema.WithTargetModel
	WithTransform   = schema.WithTransform
	WithBatchSize   = schema.WithBatchSize
)

// NewKeyPair constructs a composite key helper for BatchGet operations.
func NewKeyPair(partitionKey any, sortKey ...any) core.KeyPair {
	return core.NewKeyPair(partitionKey, sortKey...)
}

// DefaultBatchGetOptions returns the library defaults for BatchGet operations.
func DefaultBatchGetOptions() *core.BatchGetOptions {
	return core.DefaultBatchGetOptions()
}

// TransactionFunc executes a function within a database transaction.
func (db *DB) TransactionFunc(fn func(tx any) error) error {
	tx := transaction.NewTransaction(db.session, db.registry, db.converter)
	tx = tx.WithContext(db.ctx)

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return errors.Join(err, fmt.Errorf("rollback failed: %w", rbErr))
		}
		return err
	}

	return tx.Commit()
}

type metadataAdapter struct {
	metadata *model.Metadata
}

func (ma *metadataAdapter) TableName() string {
	if ma == nil || ma.metadata == nil {
		return ""
	}
	return ma.metadata.TableName
}

func (ma *metadataAdapter) RawMetadata() *model.Metadata {
	if ma == nil {
		return nil
	}
	return ma.metadata
}

func (ma *metadataAdapter) PrimaryKey() core.KeySchema {
	if ma == nil || ma.metadata == nil || ma.metadata.PrimaryKey == nil {
		return core.KeySchema{}
	}

	schema := core.KeySchema{}
	if ma.metadata.PrimaryKey.PartitionKey != nil {
		schema.PartitionKey = ma.metadata.PrimaryKey.PartitionKey.Name
	}
	if ma.metadata.PrimaryKey.SortKey != nil {
		schema.SortKey = ma.metadata.PrimaryKey.SortKey.Name
	}
	return schema
}

func (ma *metadataAdapter) Indexes() []core.IndexSchema {
	if ma == nil || ma.metadata == nil {
		return nil
	}

	indexes := make([]core.IndexSchema, len(ma.metadata.Indexes))
	for i, idx := range ma.metadata.Indexes {
		schema := core.IndexSchema{
			Name:            idx.Name,
			Type:            string(idx.Type),
			ProjectionType:  idx.ProjectionType,
			ProjectedFields: idx.ProjectedFields,
		}
		if idx.PartitionKey != nil {
			schema.PartitionKey = idx.PartitionKey.Name
		}
		if idx.SortKey != nil {
			schema.SortKey = idx.SortKey.Name
		}
		indexes[i] = schema
	}
	return indexes
}

func (ma *metadataAdapter) AttributeMetadata(field string) *core.AttributeMetadata {
	if ma == nil || ma.metadata == nil {
		return nil
	}

	fieldMeta, ok := ma.metadata.Fields[field]
	if !ok {
		fieldMeta, ok = ma.metadata.FieldsByDBName[field]
		if !ok {
			return nil
		}
	}

	return &core.AttributeMetadata{
		Name:         fieldMeta.Name,
		Type:         fieldMeta.Type.String(),
		DynamoDBName: fieldMeta.DBName,
		Tags:         fieldMeta.Tags,
	}
}

func (ma *metadataAdapter) VersionFieldName() string {
	if ma == nil || ma.metadata == nil {
		return ""
	}
	if ma.metadata.VersionField != nil {
		if ma.metadata.VersionField.DBName != "" {
			return ma.metadata.VersionField.DBName
		}
		return ma.metadata.VersionField.Name
	}
	return ""
}
