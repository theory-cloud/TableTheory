package theorydb

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/session"
)

func TestQuery_Count_UsesScanPaginatorAndAccumulatesCounts_COV6(t *testing.T) {
	httpClient := newCapturingHTTPClient(nil)
	httpClient.SetResponseSequence("DynamoDB_20120810.Scan", []stubbedResponse{
		{body: `{"Items":[],"Count":1,"ScannedCount":1,"LastEvaluatedKey":{"id":{"S":"u1"}}}`},
		{body: `{"Items":[],"Count":2,"ScannedCount":2}`},
	})

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	count, err := db.Model(&cov4RootItem{}).
		Where("Name", "=", "alice").
		Where("unknown_attr", "=", "x").
		Index("byName").
		Count()
	require.NoError(t, err)
	require.Equal(t, int64(3), count)

	req := findRequestByTarget(httpClient.Requests(), "DynamoDB_20120810.Scan")
	require.NotNil(t, req)
	require.Equal(t, "byName", req.Payload["IndexName"])
}
