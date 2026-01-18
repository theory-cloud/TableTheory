package consistency

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/theory-cloud/tabletheory/pkg/core"
)

// ReadAfterWriteHelper provides patterns for handling read-after-write consistency
type ReadAfterWriteHelper struct {
	db core.DB
}

// NewReadAfterWriteHelper creates a new helper instance
func NewReadAfterWriteHelper(db core.DB) *ReadAfterWriteHelper {
	return &ReadAfterWriteHelper{db: db}
}

// WriteOptions configures write behavior
type WriteOptions struct {
	// WaitForGSIPropagation adds a delay after write to allow GSI propagation
	WaitForGSIPropagation time.Duration

	// VerifyWrite performs a strongly consistent read after write to verify
	VerifyWrite bool
}

// CreateWithConsistency creates an item and handles consistency
func (h *ReadAfterWriteHelper) CreateWithConsistency(model any, opts *WriteOptions) error {
	// Create the item
	if err := h.db.Model(model).Create(); err != nil {
		return err
	}

	// Handle post-write consistency options
	if opts != nil {
		if opts.VerifyWrite {
			// Perform a strongly consistent read to verify the write
			dest := reflect.New(reflect.TypeOf(model).Elem()).Interface()
			if err := h.db.Model(model).ConsistentRead().First(dest); err != nil {
				return fmt.Errorf("failed to verify write: %w", err)
			}
		}

		if opts.WaitForGSIPropagation > 0 {
			// Wait for GSI propagation
			time.Sleep(opts.WaitForGSIPropagation)
		}
	}

	return nil
}

// UpdateWithConsistency updates an item and handles consistency
func (h *ReadAfterWriteHelper) UpdateWithConsistency(model any, fields []string, opts *WriteOptions) error {
	// Update the item
	if err := h.db.Model(model).Update(fields...); err != nil {
		return err
	}

	// Handle post-write consistency options
	if opts != nil {
		if opts.VerifyWrite {
			// Perform a strongly consistent read to verify the update
			dest := reflect.New(reflect.TypeOf(model).Elem()).Interface()
			if err := h.db.Model(model).ConsistentRead().First(dest); err != nil {
				return fmt.Errorf("failed to verify update: %w", err)
			}
			// Copy the verified data back
			reflect.ValueOf(model).Elem().Set(reflect.ValueOf(dest).Elem())
		}

		if opts.WaitForGSIPropagation > 0 {
			// Wait for GSI propagation
			time.Sleep(opts.WaitForGSIPropagation)
		}
	}

	return nil
}

// QueryAfterWriteOptions configures read-after-write query behavior
type QueryAfterWriteOptions struct {
	RetryConfig  *RetryConfig
	VerifyFunc   func(result any) bool
	UseMainTable bool
}

// QueryAfterWrite performs a query with read-after-write consistency handling
func (h *ReadAfterWriteHelper) QueryAfterWrite(model any, opts *QueryAfterWriteOptions) *ConsistentQueryBuilder {
	return &ConsistentQueryBuilder{
		db:    h.db,
		model: model,
		opts:  opts,
	}
}

// ConsistentQueryBuilder builds queries with consistency options
type ConsistentQueryBuilder struct {
	db    core.DB
	model any
	opts  *QueryAfterWriteOptions
	query core.Query
}

// Where adds a condition to the query
func (b *ConsistentQueryBuilder) Where(field string, op string, value any) *ConsistentQueryBuilder {
	if b.query == nil {
		b.query = b.db.Model(b.model)
	}
	b.query = b.query.Where(field, op, value)
	return b
}

// Index specifies the index to use
func (b *ConsistentQueryBuilder) Index(indexName string) *ConsistentQueryBuilder {
	if b.query == nil {
		b.query = b.db.Model(b.model)
	}

	// If UseMainTable is true, ignore the index specification
	if b.opts != nil && b.opts.UseMainTable {
		return b
	}

	b.query = b.query.Index(indexName)
	return b
}

// First executes the query and returns the first result with consistency handling
func (b *ConsistentQueryBuilder) First(dest any) error {
	if b.query == nil {
		b.query = b.db.Model(b.model)
	}

	// If using main table, use consistent read
	if b.opts != nil && b.opts.UseMainTable {
		return b.query.ConsistentRead().First(dest)
	}

	// If retry config is specified, use it
	if b.opts != nil && b.opts.RetryConfig != nil {
		config := b.opts.RetryConfig

		// If custom verification function is provided
		if b.opts.VerifyFunc != nil {
			return RetryWithVerification(
				context.Background(),
				b.query,
				dest,
				b.opts.VerifyFunc,
				config,
			)
		}

		// Otherwise use standard retry
		return b.query.WithRetry(config.MaxRetries, config.InitialDelay).First(dest)
	}

	// No special handling, just execute the query
	return b.query.First(dest)
}

// All executes the query and returns all results with consistency handling
func (b *ConsistentQueryBuilder) All(dest any) error {
	if b.query == nil {
		b.query = b.db.Model(b.model)
	}

	// If using main table, use consistent read
	if b.opts != nil && b.opts.UseMainTable {
		return b.query.ConsistentRead().All(dest)
	}

	// If retry config is specified, use it
	if b.opts != nil && b.opts.RetryConfig != nil {
		config := b.opts.RetryConfig
		return b.query.WithRetry(config.MaxRetries, config.InitialDelay).All(dest)
	}

	// No special handling, just execute the query
	return b.query.All(dest)
}

// WriteAndReadPattern demonstrates a complete write-then-read pattern
type WriteAndReadPattern struct {
	db core.DB
}

// NewWriteAndReadPattern creates a new pattern helper
func NewWriteAndReadPattern(db core.DB) *WriteAndReadPattern {
	return &WriteAndReadPattern{db: db}
}

// CreateAndQueryGSI creates an item and queries it via GSI with retry
func (p *WriteAndReadPattern) CreateAndQueryGSI(item any, gsiName string, gsiKey string, gsiValue any, dest any) error {
	// Create the item
	if err := p.db.Model(item).Create(); err != nil {
		return fmt.Errorf("failed to create item: %w", err)
	}

	// Query with retry for GSI eventual consistency
	retryConfig := &RetryConfig{
		MaxRetries:   5,
		InitialDelay: 100 * time.Millisecond,
	}

	err := p.db.Model(dest).
		Index(gsiName).
		Where(gsiKey, "=", gsiValue).
		WithRetry(retryConfig.MaxRetries, retryConfig.InitialDelay).
		First(dest)

	if err != nil {
		// Fallback to main table query if GSI is still not consistent
		return p.db.Model(dest).
			ConsistentRead().
			Where("PK", "=", reflect.ValueOf(item).Elem().FieldByName("PK").Interface()).
			First(dest)
	}

	return nil
}

// UpdateAndVerify updates an item and verifies the update with strongly consistent read
func (p *WriteAndReadPattern) UpdateAndVerify(item any, fields []string) error {
	// Update the item
	if err := p.db.Model(item).Update(fields...); err != nil {
		return fmt.Errorf("failed to update item: %w", err)
	}

	// Verify with strongly consistent read
	dest := reflect.New(reflect.TypeOf(item).Elem()).Interface()
	if err := p.db.Model(item).ConsistentRead().First(dest); err != nil {
		return fmt.Errorf("failed to verify update: %w", err)
	}

	// Copy the verified data back to the original item
	reflect.ValueOf(item).Elem().Set(reflect.ValueOf(dest).Elem())

	return nil
}
