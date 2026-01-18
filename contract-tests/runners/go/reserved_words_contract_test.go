package contracttests

import (
	"context"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory"
	"github.com/theory-cloud/tabletheory/pkg/session"
)

type reservedWordsContractModel struct {
	PK      string `theorydb:"pk"`
	SK      string `theorydb:"sk"`
	Name    string `theorydb:"attr:name"`
	Version int64  `theorydb:"version"`
}

func (reservedWordsContractModel) TableName() string { return "reserved_words_contract" }

func TestReservedWords_UpdateEscapesAttributeNames(t *testing.T) {
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

	_, err = recreatePKSKTable(ctx, ddb, "reserved_words_contract", "PK", "SK")
	require.NoError(t, err)
	defer func() {
		_, _ = ddb.DeleteTable(ctx, &dynamodb.DeleteTableInput{TableName: aws.String("reserved_words_contract")})
	}()

	db, err := tabletheory.New(session.Config{
		Region:   region,
		Endpoint: endpoint,
		AWSConfigOptions: []func(*config.LoadOptions) error{
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")),
			config.WithRegion(region),
		},
	})
	require.NoError(t, err)

	err = db.WithContext(ctx).Model(&reservedWordsContractModel{
		PK:   "A",
		SK:   "B",
		Name: "v0",
	}).CreateOrUpdate()
	require.NoError(t, err)

	err = db.WithContext(ctx).Model(&reservedWordsContractModel{
		PK:      "A",
		SK:      "B",
		Name:    "v1",
		Version: 0,
	}).Update("Name")
	require.NoError(t, err)

	var got reservedWordsContractModel
	err = db.WithContext(ctx).
		Model(&reservedWordsContractModel{}).
		Where("PK", "=", "A").
		Where("SK", "=", "B").
		First(&got)
	require.NoError(t, err)
	require.Equal(t, "v1", got.Name)
	require.Equal(t, int64(1), got.Version)
}
