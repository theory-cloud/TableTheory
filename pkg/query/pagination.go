package query

import (
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/theory-cloud/tabletheory/pkg/core"
)

// AllPaginated executes the query and returns paginated results
func (q *Query) AllPaginated(dest any) (*core.PaginatedResult, error) {
	if err := q.checkBuilderError(); err != nil {
		return nil, err
	}
	// Set a reasonable limit if not specified
	if q.limit == 0 {
		q.limit = 100
	}

	compiled, err := q.Compile()
	if err != nil {
		return nil, err
	}

	// Execute the query
	var result any
	if compiled.Operation == operationQuery {
		result, err = q.executePaginatedQuery(compiled, dest)
	} else {
		result, err = q.executePaginatedScan(compiled, dest)
	}

	if err != nil {
		return nil, err
	}

	// Extract pagination info
	queryResult, ok := result.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unexpected pagination result type: %T", result)
	}

	// Build the paginated result
	paginatedResult := &core.PaginatedResult{
		Items:        dest,
		NextCursor:   q.encodeCursor(queryResult["LastEvaluatedKey"]),
		Count:        0,
		ScannedCount: 0,
	}

	// Safely extract counts
	if count, ok := queryResult["Count"].(int64); ok {
		paginatedResult.Count = int(count)
	} else if count, ok := queryResult["Count"].(int); ok {
		paginatedResult.Count = count
	}

	if scannedCount, ok := queryResult["ScannedCount"].(int64); ok {
		paginatedResult.ScannedCount = int(scannedCount)
	} else if scannedCount, ok := queryResult["ScannedCount"].(int); ok {
		paginatedResult.ScannedCount = scannedCount
	}

	// Set HasMore based on cursor
	paginatedResult.HasMore = paginatedResult.NextCursor != ""

	// Extract LastEvaluatedKey
	if lastKey, ok := queryResult["LastEvaluatedKey"].(map[string]types.AttributeValue); ok {
		paginatedResult.LastEvaluatedKey = lastKey
	}

	return paginatedResult, nil
}

// SetCursor sets the pagination cursor for the query
func (q *Query) SetCursor(cursor string) error {
	if cursor == "" {
		return nil
	}

	// Decode the cursor to ExclusiveStartKey
	startKey, err := q.decodeCursor(cursor)
	if err != nil {
		return fmt.Errorf("invalid cursor: %w", err)
	}

	q.exclusive = startKey
	return nil
}

// Cursor is a fluent method to set the pagination cursor
func (q *Query) Cursor(cursor string) core.Query {
	if err := q.SetCursor(cursor); err != nil {
		q.recordBuilderError(err)
	}
	return q
}

func paginationInfoMap(count int64, scannedCount int64, lastEvaluatedKey map[string]types.AttributeValue) map[string]any {
	return map[string]any{
		"Count":            count,
		"ScannedCount":     scannedCount,
		"LastEvaluatedKey": lastEvaluatedKey,
	}
}

func emptyPaginationInfoMap() map[string]any {
	return map[string]any{
		"Count":            0,
		"ScannedCount":     0,
		"LastEvaluatedKey": nil,
	}
}

func (q *Query) executeWithOptionalPagination(
	compiled *core.CompiledQuery,
	dest any,
	execPaginated func(PaginatedQueryExecutor, *core.CompiledQuery, any) (int64, int64, map[string]types.AttributeValue, error),
	exec func(*core.CompiledQuery, any) error,
) (any, error) {
	// Check if executor supports pagination
	if paginatedExecutor, ok := q.executor.(PaginatedQueryExecutor); ok {
		count, scannedCount, lastEvaluatedKey, err := execPaginated(paginatedExecutor, compiled, dest)
		if err != nil {
			return nil, err
		}
		return paginationInfoMap(count, scannedCount, lastEvaluatedKey), nil
	}

	// Fall back to regular execution without pagination info
	if err := exec(compiled, dest); err != nil {
		return nil, err
	}

	// Return mock result for backward compatibility
	return emptyPaginationInfoMap(), nil
}

// executePaginatedQuery executes a query with pagination support
func (q *Query) executePaginatedQuery(compiled *core.CompiledQuery, dest any) (any, error) {
	return q.executeWithOptionalPagination(
		compiled,
		dest,
		func(exec PaginatedQueryExecutor, compiled *core.CompiledQuery, dest any) (int64, int64, map[string]types.AttributeValue, error) {
			result, err := exec.ExecuteQueryWithPagination(compiled, dest)
			if err != nil {
				return 0, 0, nil, err
			}
			return result.Count, result.ScannedCount, result.LastEvaluatedKey, nil
		},
		q.executor.ExecuteQuery,
	)
}

// executePaginatedScan executes a scan with pagination support
func (q *Query) executePaginatedScan(compiled *core.CompiledQuery, dest any) (any, error) {
	return q.executeWithOptionalPagination(
		compiled,
		dest,
		func(exec PaginatedQueryExecutor, compiled *core.CompiledQuery, dest any) (int64, int64, map[string]types.AttributeValue, error) {
			result, err := exec.ExecuteScanWithPagination(compiled, dest)
			if err != nil {
				return 0, 0, nil, err
			}
			return result.Count, result.ScannedCount, result.LastEvaluatedKey, nil
		},
		q.executor.ExecuteScan,
	)
}

// encodeCursor encodes the LastEvaluatedKey as a cursor string
func (q *Query) encodeCursor(lastKey any) string {
	if lastKey == nil {
		return ""
	}

	// Convert to map[string]types.AttributeValue if needed
	var avMap map[string]types.AttributeValue
	switch v := lastKey.(type) {
	case map[string]types.AttributeValue:
		avMap = v
	case map[string]any:
		// Handle the case where lastKey is map[string]any
		// This would come from the executor results
		if val, ok := v["LastEvaluatedKey"]; ok {
			if m, ok := val.(map[string]types.AttributeValue); ok {
				avMap = m
			}
		}
	default:
		return ""
	}

	if len(avMap) == 0 {
		return ""
	}

	// Use the new EncodeCursor function
	encoded, err := EncodeCursor(avMap, q.index, q.orderBy.Order)
	if err != nil {
		// Log error in production
		return ""
	}
	return encoded
}

// decodeCursor decodes a cursor string to ExclusiveStartKey
func (q *Query) decodeCursor(cursor string) (map[string]types.AttributeValue, error) {
	if cursor == "" {
		return nil, nil
	}

	// Use the new DecodeCursor function
	decodedCursor, err := DecodeCursor(cursor)
	if err != nil {
		return nil, err
	}

	if decodedCursor == nil {
		return nil, nil
	}

	// Convert back to AttributeValues
	return decodedCursor.ToAttributeValues()
}
