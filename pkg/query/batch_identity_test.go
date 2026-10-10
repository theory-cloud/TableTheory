package query

import (
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v4/pkg/core"
)

// shrinkingBatchWriteExecutor models a legal DynamoDB BatchWriteItem partial
// success: on every call it accepts everything except the last request and
// returns that request in UnprocessedItems. The surviving subset therefore
// cannot shrink to empty within the retry budget, so the retry loop exhausts
// with the last request still unprocessed while earlier requests were accepted.
type shrinkingBatchWriteExecutor struct {
	calls    []int
	accepted int
}

func (e *shrinkingBatchWriteExecutor) ExecuteBatchWriteItem(_ string, writeRequests []types.WriteRequest) (*core.BatchWriteResult, error) {
	e.calls = append(e.calls, len(writeRequests))
	if len(writeRequests) == 0 {
		return &core.BatchWriteResult{UnprocessedItems: map[string][]types.WriteRequest{}}, nil
	}

	e.accepted += len(writeRequests) - 1
	last := writeRequests[len(writeRequests)-1]
	return &core.BatchWriteResult{
		UnprocessedItems: map[string][]types.WriteRequest{"tbl": {last}},
	}, nil
}

func (e *shrinkingBatchWriteExecutor) ExecuteQuery(*core.CompiledQuery, any) error { return nil }
func (e *shrinkingBatchWriteExecutor) ExecuteScan(*core.CompiledQuery, any) error  { return nil }

// TestBatchCreateWithResult_PartialSuccessReportsOnlySurvivors proves that when
// DynamoDB accepts part of a chunk and the remainder survives retry exhaustion,
// only the surviving request is reported failed; the accepted request is counted
// in Succeeded and the top-level error stays nil.
func TestBatchCreateWithResult_PartialSuccessReportsOnlySurvivors(t *testing.T) {
	exec := &shrinkingBatchWriteExecutor{}
	q := New(&cov6BatchCreateItem{}, cov6Metadata{table: "tbl"}, exec)

	result, err := q.BatchCreateWithResult([]cov6BatchCreateItem{{ID: "1"}, {ID: "2"}})
	require.NoError(t, err)
	require.Equal(t, []int{2, 1, 1, 1, 1}, exec.calls)
	require.Equal(t, 1, exec.accepted)

	require.Equal(t, 1, result.Failed, "only the surviving request failed")
	require.Len(t, result.Errors, 1)
	require.Equal(t, 1, result.Succeeded, "the accepted request succeeded")
	require.NotContains(t, result.Errors[0].Error(), "failed to marshal")
}

// TestBatchCreateWithResult_MarshalFailurePlusPartialWriteDoesNotDoubleCount
// proves that a marshal failure combined with a partial write followed by retry
// exhaustion counts each input at most once and never makes Succeeded negative.
func TestBatchCreateWithResult_MarshalFailurePlusPartialWriteDoesNotDoubleCount(t *testing.T) {
	exec := &shrinkingBatchWriteExecutor{}
	q := New(&cov6BatchCreateItem{}, cov6Metadata{table: "tbl"}, exec)

	// 123 fails to marshal; the two structs marshaled successfully, the first is
	// accepted, the second survives exhaustion.
	result, err := q.BatchCreateWithResult([]any{
		cov6BatchCreateItem{ID: "1"},
		123,
		cov6BatchCreateItem{ID: "3"},
	})
	require.NoError(t, err)

	require.Equal(t, 2, result.Failed, "one marshal failure + one surviving request")
	require.Len(t, result.Errors, 2)
	require.Equal(t, 1, result.Succeeded)
	require.GreaterOrEqual(t, result.Succeeded, 0)
	require.LessOrEqual(t, result.Failed+result.Succeeded, 3, "each input counted at most once")

	// Exactly one error is the retry-exhaustion failure; the other is the
	// marshal failure. No accepted item is represented in either.
	var writeFailures, marshalFailures int
	for _, err := range result.Errors {
		if strings.Contains(err.Error(), "failed to process") {
			writeFailures++
		} else {
			marshalFailures++
		}
	}
	require.Equal(t, 1, writeFailures)
	require.Equal(t, 1, marshalFailures)
}

// TestQuery_batchCreateWithOptionsInternal_PartialSuccessReportsOnlySurvivors
// proves the adjacent options/callback path attributes a terminal write failure
// to the surviving requests only, so an accepted item is never handed to the
// error handler.
func TestQuery_batchCreateWithOptionsInternal_PartialSuccessReportsOnlySurvivors(t *testing.T) {
	exec := &shrinkingBatchWriteExecutor{}
	q := New(&cov6BatchCreateItem{}, cov6Metadata{table: "tbl"}, exec)

	var subjects []any
	opts := DefaultBatchOptions()
	opts.ErrorHandler = func(item any, _ error) error {
		subjects = append(subjects, item)
		return nil
	}

	require.NoError(t, q.batchCreateWithOptionsInternal(
		[]cov6BatchCreateItem{{ID: "1"}, {ID: "2"}}, opts))

	require.Len(t, subjects, 1, "only the surviving request is reported")
	require.Equal(t, cov6BatchCreateItem{ID: "2"}, subjects[0])
}
