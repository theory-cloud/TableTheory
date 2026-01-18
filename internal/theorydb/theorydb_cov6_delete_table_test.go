package theorydb

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/session"
)

func TestDB_DeleteTable_CoversModelPathAndRegisterError_COV6(t *testing.T) {
	httpClient := newCapturingHTTPClient(map[string]string{
		"DynamoDB_20120810.DeleteTable": `{}`,
	})
	httpClient.SetResponseSequence("DynamoDB_20120810.DescribeTable", []stubbedResponse{
		{
			status:  400,
			headers: map[string]string{"X-Amzn-ErrorType": "ResourceNotFoundException"},
			body:    `{"__type":"com.amazonaws.dynamodb.v20120810#ResourceNotFoundException","message":"not found"}`,
		},
	})

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	require.NoError(t, db.DeleteTable(&cov4RootItem{}))

	require.ErrorContains(t, db.DeleteTable(123), "failed to register model")
}
