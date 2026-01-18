package query

import (
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/theory-cloud/tabletheory/pkg/core"
)

// CompiledBatchGet represents a compiled batch get operation
type CompiledBatchGet struct {
	ExpressionAttributeNames map[string]string
	TableName                string
	ProjectionExpression     string
	Keys                     []map[string]types.AttributeValue
	ConsistentRead           bool
}

// CompiledBatchWrite represents a compiled batch write operation
type CompiledBatchWrite struct {
	TableName string
	Items     []map[string]types.AttributeValue
}

// BatchExecutor extends QueryExecutor with batch operations
type BatchExecutor interface {
	QueryExecutor
	ExecuteBatchGet(input *CompiledBatchGet, opts *core.BatchGetOptions) ([]map[string]types.AttributeValue, error)
	ExecuteBatchWrite(input *CompiledBatchWrite) error
}

// QueryResult represents the result of a query operation
type QueryResult struct {
	LastEvaluatedKey map[string]types.AttributeValue
	Items            []map[string]types.AttributeValue
	Count            int64
	ScannedCount     int64
}

// ScanResult represents the result of a scan operation
type ScanResult struct {
	LastEvaluatedKey map[string]types.AttributeValue
	Items            []map[string]types.AttributeValue
	Count            int64
	ScannedCount     int64
}

// BatchGetResult represents the result of a batch get operation
type BatchGetResult struct {
	Responses       []map[string]types.AttributeValue
	UnprocessedKeys []map[string]types.AttributeValue
}

// PaginatedResult represents a paginated query result
type PaginatedResult struct {
	Items        any    `json:"items"`
	NextCursor   string `json:"nextCursor,omitempty"`
	Count        int    `json:"count"`
	HasMore      bool   `json:"hasMore"`
	ScannedCount int    `json:"scannedCount,omitempty"`
}

// CompiledScan represents a compiled scan operation
type CompiledScan struct {
	ExpressionAttributeNames  map[string]string
	ExpressionAttributeValues map[string]types.AttributeValue
	Limit                     *int32
	ExclusiveStartKey         map[string]types.AttributeValue
	Segment                   *int32
	TotalSegments             *int32
	TableName                 string
	FilterExpression          string
	ProjectionExpression      string
	ConsistentRead            bool
}
