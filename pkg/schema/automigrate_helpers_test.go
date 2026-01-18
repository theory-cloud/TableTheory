package schema

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"
)

func TestNewAutoMigrateOptions_DefaultsAndOverrides(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		opts := newAutoMigrateOptions(nil)
		require.Equal(t, 25, opts.BatchSize)
		require.NotNil(t, opts.Context)
		require.False(t, opts.DataCopy)
		require.Empty(t, opts.BackupTable)
		require.Nil(t, opts.TargetModel)
		require.Nil(t, opts.Transform)
	})

	t.Run("overrides", func(t *testing.T) {
		type ctxKey struct{}
		ctx := context.WithValue(context.Background(), ctxKey{}, "v")
		transform := func(source map[string]types.AttributeValue) (map[string]types.AttributeValue, error) {
			return source, nil
		}

		opts := newAutoMigrateOptions([]AutoMigrateOption{
			WithBatchSize(7),
			WithContext(ctx),
			WithBackupTable("backup"),
			WithDataCopy(true),
			WithTargetModel("target"),
			WithTransform(transform),
		})

		require.Equal(t, 7, opts.BatchSize)
		require.Equal(t, ctx, opts.Context)
		require.Equal(t, "backup", opts.BackupTable)
		require.True(t, opts.DataCopy)
		require.Equal(t, "target", opts.TargetModel)
		require.NotNil(t, opts.Transform)
	})
}

func TestResolveDataCopyBatchSize(t *testing.T) {
	require.Equal(t, 10, resolveDataCopyBatchSize(0))
	require.Equal(t, 10, resolveDataCopyBatchSize(-1))
	require.Equal(t, 5, resolveDataCopyBatchSize(5))
	require.Equal(t, 10, resolveDataCopyBatchSize(10))
	require.Equal(t, 10, resolveDataCopyBatchSize(25))
}

func TestUnprocessedRequestsForTable(t *testing.T) {
	require.Nil(t, unprocessedRequestsForTable(nil, "tbl"))

	require.Nil(t, unprocessedRequestsForTable(map[string][]types.WriteRequest{}, "tbl"))

	require.Nil(t, unprocessedRequestsForTable(map[string][]types.WriteRequest{
		"other": {{PutRequest: &types.PutRequest{}}},
	}, "tbl"))

	require.Nil(t, unprocessedRequestsForTable(map[string][]types.WriteRequest{
		"tbl": {},
	}, "tbl"))

	reqs := []types.WriteRequest{{PutRequest: &types.PutRequest{}}}
	got := unprocessedRequestsForTable(map[string][]types.WriteRequest{
		"tbl": reqs,
	}, "tbl")
	require.Equal(t, reqs, got)
}

func TestSleepWithBackoff_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := sleepWithBackoff(ctx, 1)
	require.ErrorIs(t, err, context.Canceled)
}
