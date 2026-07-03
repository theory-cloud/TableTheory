package query

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/stretchr/testify/require"
)

type covCountPage struct {
	count   int32
	scanned int32
}

type covCountPaginator struct {
	pages []covCountPage
	idx   int
	fail  bool
}

func (p *covCountPaginator) HasMorePages() bool {
	return p.idx < len(p.pages) || (p.fail && p.idx == 0)
}

func (p *covCountPaginator) NextPage(context.Context, ...func(*dynamodb.Options)) (*covCountPage, error) {
	if p.fail {
		p.idx++
		return nil, errCovCountBoom
	}
	page := p.pages[p.idx]
	p.idx++
	return &page, nil
}

var errCovCountBoom = errors.New("boom")

func TestCountDynamoPagesAndWriteCountResult_COV5(t *testing.T) {
	paginator := &covCountPaginator{
		pages: []covCountPage{{count: 2, scanned: 5}, {count: 3, scanned: 8}},
	}
	count, scanned, err := countDynamoPages(
		context.Background(),
		"fixture",
		paginator,
		func(page *covCountPage) (int32, int32) { return page.count, page.scanned },
	)
	require.NoError(t, err)
	require.Equal(t, int64(5), count)
	require.Equal(t, int64(13), scanned)

	var uintCount uint64
	require.NoError(t, writeCountResult(&uintCount, count, scanned))
	require.Equal(t, uint64(5), uintCount)

	var structCount struct {
		Count        uint64
		ScannedCount int64
	}
	require.NoError(t, writeCountResult(&structCount, count, scanned))
	require.Equal(t, uint64(5), structCount.Count)
	require.Equal(t, int64(13), structCount.ScannedCount)

	require.Error(t, writeCountResult(nil, count, scanned))
	require.Error(t, writeCountResult(uintCount, count, scanned))
	require.Error(t, writeCountResult(&uintCount, -1, scanned))

	_, _, err = countDynamoPages(
		context.Background(),
		"fixture",
		&covCountPaginator{fail: true},
		func(page *covCountPage) (int32, int32) { return page.count, page.scanned },
	)
	require.ErrorIs(t, err, errCovCountBoom)
}
