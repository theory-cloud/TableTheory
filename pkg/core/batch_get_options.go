package core

// BatchProgressCallback is invoked after each chunk completes with the total number of items retrieved so far.
type BatchProgressCallback func(retrieved, total int)

// BatchChunkErrorHandler can intercept per-chunk failures. Return nil to swallow the error and continue.
type BatchChunkErrorHandler func(chunk []any, err error) error

// BatchGetOptions tune the behavior of BatchGet operations.
type BatchGetOptions struct {
	RetryPolicy      *RetryPolicy
	ProgressCallback BatchProgressCallback
	OnChunkError     BatchChunkErrorHandler
	ChunkSize        int
	MaxConcurrency   int
	ConsistentRead   bool
	Parallel         bool
}

// DefaultBatchGetOptions returns a sensible baseline configuration.
func DefaultBatchGetOptions() *BatchGetOptions {
	return &BatchGetOptions{
		ChunkSize:        100,
		ConsistentRead:   false,
		Parallel:         false,
		MaxConcurrency:   4,
		RetryPolicy:      DefaultRetryPolicy(),
		ProgressCallback: nil,
		OnChunkError:     nil,
	}
}

// Clone returns a shallow copy of the options to decouple caller modifications from shared defaults.
func (o *BatchGetOptions) Clone() *BatchGetOptions {
	if o == nil {
		return nil
	}

	clone := *o
	if o.RetryPolicy != nil {
		clone.RetryPolicy = o.RetryPolicy.Clone()
	}
	return &clone
}

// BatchGetBuilder exposes a fluent API for composing advanced BatchGet operations.
type BatchGetBuilder interface {
	Keys(keys []any) BatchGetBuilder
	ChunkSize(size int) BatchGetBuilder
	ConsistentRead() BatchGetBuilder
	Parallel(maxConcurrency int) BatchGetBuilder
	WithRetry(policy *RetryPolicy) BatchGetBuilder
	Select(fields ...string) BatchGetBuilder
	OnProgress(callback BatchProgressCallback) BatchGetBuilder
	OnError(handler BatchChunkErrorHandler) BatchGetBuilder
	Execute(dest any) error
}

// KeyPair lets callers supply composite keys without defining ad-hoc structs.
type KeyPair struct {
	PartitionKey any
	SortKey      any
}

// NewKeyPair constructs a KeyPair with the optional sort key.
func NewKeyPair(partitionKey any, sortKey ...any) KeyPair {
	var sk any
	if len(sortKey) > 0 {
		sk = sortKey[0]
	}
	return KeyPair{
		PartitionKey: partitionKey,
		SortKey:      sk,
	}
}
