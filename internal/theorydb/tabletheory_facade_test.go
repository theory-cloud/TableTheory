package theorydb_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory"
	"github.com/theory-cloud/tabletheory/pkg/core"
)

type facadeModel struct {
	Name  string `dynamodb:"name"`
	Count int    `dynamodb:"count"`
}

func TestTableTheoryFacade_Wrappers(t *testing.T) {
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_ACCESS_KEY_ID", "dummy")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "dummy")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")

	// Unmarshal helpers
	var single facadeModel
	require.NoError(t, tabletheory.UnmarshalItem(map[string]types.AttributeValue{
		"name":  &types.AttributeValueMemberS{Value: "alice"},
		"count": &types.AttributeValueMemberN{Value: "42"},
	}, &single))
	require.Equal(t, "alice", single.Name)
	require.Equal(t, 42, single.Count)

	var many []facadeModel
	require.NoError(t, tabletheory.UnmarshalItems([]map[string]types.AttributeValue{
		{
			"name":  &types.AttributeValueMemberS{Value: "a"},
			"count": &types.AttributeValueMemberN{Value: "1"},
		},
		{
			"name":  &types.AttributeValueMemberS{Value: "b"},
			"count": &types.AttributeValueMemberN{Value: "2"},
		},
	}, &many))
	require.Len(t, many, 2)
	require.Equal(t, "a", many[0].Name)
	require.Equal(t, 1, many[0].Count)
	require.Equal(t, "b", many[1].Name)
	require.Equal(t, 2, many[1].Count)

	var fromStream facadeModel
	require.NoError(t, tabletheory.UnmarshalStreamImage(map[string]events.DynamoDBAttributeValue{
		"name":  events.NewStringAttribute("stream"),
		"count": events.NewNumberAttribute("7"),
	}, &fromStream))
	require.Equal(t, "stream", fromStream.Name)
	require.Equal(t, 7, fromStream.Count)

	// Constructor helpers
	key := tabletheory.NewKeyPair("pk", "sk")
	require.Equal(t, "pk", key.PartitionKey)
	require.Equal(t, "sk", key.SortKey)

	opts := tabletheory.DefaultBatchGetOptions()
	require.NotNil(t, opts)
	require.Equal(t, 100, opts.ChunkSize)

	// Transaction conditions
	cond := tabletheory.Condition("field", "=", 1)
	require.Equal(t, core.TransactConditionKindField, cond.Kind)
	require.Equal(t, "field", cond.Field)
	require.Equal(t, "=", cond.Operator)
	require.Equal(t, 1, cond.Value)

	values := map[string]any{":v": 1}
	condExpr := tabletheory.ConditionExpression("field = :v", values)
	require.Equal(t, core.TransactConditionKindExpression, condExpr.Kind)
	require.Equal(t, "field = :v", condExpr.Expression)
	require.Equal(t, map[string]any{":v": 1}, condExpr.Values)
	values[":v"] = 2
	require.Equal(t, map[string]any{":v": 1}, condExpr.Values)

	require.Equal(t, core.TransactConditionKindPrimaryKeyNotExists, tabletheory.IfNotExists().Kind)
	require.Equal(t, core.TransactConditionKindPrimaryKeyExists, tabletheory.IfExists().Kind)

	version := int64(42)
	require.Equal(t, core.TransactConditionKindVersionEquals, tabletheory.AtVersion(version).Kind)
	require.Equal(t, version, tabletheory.AtVersion(version).Value)
	require.Equal(t, tabletheory.AtVersion(version), tabletheory.ConditionVersion(version))

	// Lambda helpers (env-driven)
	require.False(t, tabletheory.IsLambdaEnvironment())
	t.Setenv("AWS_LAMBDA_FUNCTION_NAME", "facade-test")
	require.True(t, tabletheory.IsLambdaEnvironment())

	require.Equal(t, 0, tabletheory.GetLambdaMemoryMB())
	t.Setenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE", "256")
	require.Equal(t, 256, tabletheory.GetLambdaMemoryMB())

	require.False(t, tabletheory.EnableXRayTracing())
	t.Setenv("_X_AMZN_TRACE_ID", "trace")
	require.True(t, tabletheory.EnableXRayTracing())

	require.Equal(t, int64(-1), tabletheory.GetRemainingTimeMillis(context.Background()))
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	require.GreaterOrEqual(t, tabletheory.GetRemainingTimeMillis(ctx), int64(0))

	// Context helpers
	require.Equal(t, "", tabletheory.GetPartnerFromContext(context.Background()))
	require.Equal(t, "partner", tabletheory.GetPartnerFromContext(tabletheory.PartnerContext(context.Background(), "partner")))

	// Factory helpers (avoid hitting the network by supplying an explicit credentials provider)
	creds := aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider("dummy", "dummy", ""))
	db, err := tabletheory.New(tabletheory.Config{
		Region:              "us-east-1",
		CredentialsProvider: creds,
	})
	require.NoError(t, err)
	require.NotNil(t, db)

	basic, err := tabletheory.NewBasic(tabletheory.Config{
		Region:              "us-east-1",
		CredentialsProvider: creds,
	})
	require.NoError(t, err)
	require.NotNil(t, basic)

	ldb, err := tabletheory.NewLambdaOptimized()
	require.NoError(t, err)
	require.NotNil(t, ldb)

	ldb2, err := tabletheory.LambdaInit()
	require.NoError(t, err)
	require.NotNil(t, ldb2)

	multi, err := tabletheory.NewMultiAccount(map[string]tabletheory.AccountConfig{})
	require.NoError(t, err)
	require.NotNil(t, multi)
	require.NoError(t, multi.Close())

	// Exercise BenchmarkColdStart wrapper without relying on AWS network access.
	// A deliberately invalid shared config causes config.LoadDefaultConfig to fail fast.
	invalidCfg := filepath.Join(t.TempDir(), "awsconfig")
	require.NoError(t, os.WriteFile(invalidCfg, []byte("[default\ninvalid"), 0o600))
	t.Setenv("AWS_SDK_LOAD_CONFIG", "1")
	t.Setenv("AWS_CONFIG_FILE", invalidCfg)
	_ = tabletheory.BenchmarkColdStart()
}
