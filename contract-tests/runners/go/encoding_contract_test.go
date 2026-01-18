package contracttests

import (
	"context"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory"
	"github.com/theory-cloud/tabletheory/pkg/session"
)

type emptySetContractModel struct {
	PK   string   `theorydb:"pk"`
	SK   string   `theorydb:"sk"`
	Tags []string `theorydb:"set,attr:tags"`
}

func (emptySetContractModel) TableName() string { return "sets_contract" }

func TestEncoding_EmptySetEncodesNULL(t *testing.T) {
	t.Helper()

	skip := os.Getenv("SKIP_INTEGRATION")
	endpoint := os.Getenv("DYNAMODB_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:8000"
	}
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = os.Getenv("AWS_DEFAULT_REGION")
	}
	if region == "" {
		region = "us-east-1"
	}

	ctx := context.Background()

	ddb, err := dynamodbLocalClient(ctx, region, endpoint)
	require.NoError(t, err)

	if err := pingDynamoDB(ctx, ddb); err != nil {
		if skip == "1" || skip == "true" {
			t.Skipf("DynamoDB Local not reachable (SKIP_INTEGRATION set): %v", err)
		}
		require.NoError(t, err)
	}

	_, err = recreatePKSKTable(ctx, ddb, "sets_contract", "PK", "SK")
	require.NoError(t, err)
	defer func() {
		_, _ = ddb.DeleteTable(ctx, &dynamodb.DeleteTableInput{TableName: aws.String("sets_contract")})
	}()

	db, err := theorydb.New(session.Config{
		Region:   region,
		Endpoint: endpoint,
		AWSConfigOptions: []func(*config.LoadOptions) error{
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")),
			config.WithRegion(region),
		},
	})
	require.NoError(t, err)

	err = db.WithContext(ctx).Model(&emptySetContractModel{
		PK:   "A",
		SK:   "B",
		Tags: []string{},
	}).CreateOrUpdate()
	require.NoError(t, err)

	raw, err := ddb.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String("sets_contract"),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: "A"},
			"SK": &types.AttributeValueMemberS{Value: "B"},
		},
		ConsistentRead: aws.Bool(true),
	})
	require.NoError(t, err)
	require.NotNil(t, raw.Item)

	tags, ok := raw.Item["tags"].(*types.AttributeValueMemberNULL)
	require.True(t, ok, "expected tags NULL, got %T", raw.Item["tags"])
	require.True(t, tags.Value)
}
