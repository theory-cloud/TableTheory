package schema

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v6/pkg/model"
	"github.com/theory-cloud/tabletheory/v6/pkg/session"
	"github.com/theory-cloud/tabletheory/v6/tests"
)

type ttlCreateTableModel struct {
	ID        string `theorydb:"pk"`
	ExpiresAt int64  `theorydb:"ttl"`
}

func (ttlCreateTableModel) TableName() string { return "tabletheory_local_t15_ttl_create" }

func newTTLIntegrationManager(t *testing.T) (*Manager, *dynamodb.Client) {
	t.Helper()

	tests.RequireDynamoDBLocal(t)
	cfg := tests.GetTestConfig()

	sess, err := session.NewSession(&session.Config{
		Region:   cfg.Region,
		Endpoint: cfg.Endpoint,
		CredentialsProvider: aws.CredentialsProviderFunc(
			func(context.Context) (aws.Credentials, error) {
				return aws.Credentials{AccessKeyID: "dummy", SecretAccessKey: "dummy"}, nil
			}),
	})
	require.NoError(t, err)

	client, err := sess.Client()
	require.NoError(t, err)

	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&ttlCreateTableModel{}))

	return NewManager(sess, registry), client
}

func TestCreateTable_ExistingTableWithTTLEnabled_IsIdempotent(t *testing.T) {
	mgr, client := newTTLIntegrationManager(t)
	ctx := context.Background()

	deleteTableIfExists(t, mgr, "tabletheory_local_t15_ttl_create")
	t.Cleanup(func() {
		if err := mgr.DeleteTable("tabletheory_local_t15_ttl_create"); err != nil {
			t.Logf("cleanup: delete table: %v", err)
		}
	})

	require.NoError(t, mgr.CreateTable(&ttlCreateTableModel{}))
	require.NoError(t, mgr.CreateTable(&ttlCreateTableModel{}))

	ttl, err := client.DescribeTimeToLive(ctx, &dynamodb.DescribeTimeToLiveInput{
		TableName: aws.String("tabletheory_local_t15_ttl_create"),
	})
	require.NoError(t, err)
	require.NotNil(t, ttl.TimeToLiveDescription)
	require.Equal(t, types.TimeToLiveStatusEnabled, ttl.TimeToLiveDescription.TimeToLiveStatus)
	require.Equal(t, "expiresAt", aws.ToString(ttl.TimeToLiveDescription.AttributeName))
}

func TestCreateTable_ExistingTableWithTTLOnDifferentAttribute_ReturnsClearError(t *testing.T) {
	mgr, client := newTTLIntegrationManager(t)
	ctx := context.Background()

	deleteTableIfExists(t, mgr, "tabletheory_local_t15_ttl_create")
	t.Cleanup(func() {
		if err := mgr.DeleteTable("tabletheory_local_t15_ttl_create"); err != nil {
			t.Logf("cleanup: delete table: %v", err)
		}
	})

	_, err := client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName:   aws.String("tabletheory_local_t15_ttl_create"),
		BillingMode: types.BillingModePayPerRequest,
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("ID"), KeyType: types.KeyTypeHash},
		},
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("ID"), AttributeType: types.ScalarAttributeTypeS},
		},
	})
	require.NoError(t, err)

	waiter := dynamodb.NewTableExistsWaiter(client)
	require.NoError(t, waiter.Wait(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String("tabletheory_local_t15_ttl_create"),
	}, 30*time.Second))

	_, err = client.UpdateTimeToLive(ctx, &dynamodb.UpdateTimeToLiveInput{
		TableName: aws.String("tabletheory_local_t15_ttl_create"),
		TimeToLiveSpecification: &types.TimeToLiveSpecification{
			AttributeName: aws.String("LegacyTtl"),
			Enabled:       aws.Bool(true),
		},
	})
	require.NoError(t, err)

	err = mgr.CreateTable(&ttlCreateTableModel{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "expiresAt")
	require.Contains(t, err.Error(), "LegacyTtl")
	require.Contains(t, err.Error(), "different attribute")
}
