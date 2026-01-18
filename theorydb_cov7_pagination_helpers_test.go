package theorydb

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"
)

func TestCollectPaginatedCounts_ErrorPropagation_COV7(t *testing.T) {
	calls := 0
	hasMorePages := func() bool {
		return calls < 1
	}
	nextPage := func(ctx context.Context) (int32, int32, error) {
		_ = ctx
		calls++
		return 0, 0, errors.New("boom")
	}

	_, _, err := collectPaginatedCounts(context.Background(), hasMorePages, nextPage)
	require.Error(t, err)
}

func TestCollectPaginatedItems_LimitTrimAndBreak_COV7(t *testing.T) {
	t.Run("trim_true", func(t *testing.T) {
		calls := 0
		hasMorePages := func() bool {
			return calls < 1
		}
		nextPage := func(ctx context.Context) ([]map[string]types.AttributeValue, error) {
			_ = ctx
			calls++
			return []map[string]types.AttributeValue{{}, {}, {}}, nil
		}

		items, err := collectPaginatedItems(context.Background(), hasMorePages, nextPage, 2, true, true)
		require.NoError(t, err)
		require.Len(t, items, 2)
	})

	t.Run("trim_false", func(t *testing.T) {
		calls := 0
		hasMorePages := func() bool {
			return calls < 1
		}
		nextPage := func(ctx context.Context) ([]map[string]types.AttributeValue, error) {
			_ = ctx
			calls++
			return []map[string]types.AttributeValue{{}, {}, {}}, nil
		}

		items, err := collectPaginatedItems(context.Background(), hasMorePages, nextPage, 2, true, false)
		require.NoError(t, err)
		require.Len(t, items, 3)
	})

	t.Run("nextPage_error_propagates", func(t *testing.T) {
		calls := 0
		hasMorePages := func() bool {
			return calls < 1
		}
		nextPage := func(ctx context.Context) ([]map[string]types.AttributeValue, error) {
			_ = ctx
			calls++
			return nil, errors.New("boom")
		}

		_, err := collectPaginatedItems(context.Background(), hasMorePages, nextPage, 0, false, false)
		require.Error(t, err)
	})
}
