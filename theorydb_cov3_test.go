package theorydb

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/core"
	queryPkg "github.com/theory-cloud/tabletheory/pkg/query"
	"github.com/theory-cloud/tabletheory/pkg/session"
)

type cov3Item struct {
	ID   string `theorydb:"pk,attr:id"`
	Name string `theorydb:"attr:name"`
}

func (cov3Item) TableName() string { return "cov3_items" }

type cov3CompositeItem struct {
	PK string `theorydb:"pk,attr:pk"`
	SK string `theorydb:"sk,attr:sk"`
}

func (cov3CompositeItem) TableName() string { return "cov3_composite_items" }

func TestUnmarshalItems_Wrapper(t *testing.T) {
	type u struct {
		ID   string `dynamodb:"id"`
		Name string `dynamodb:"name"`
	}

	items := []map[string]types.AttributeValue{
		{
			"id":   &types.AttributeValueMemberS{Value: "u1"},
			"name": &types.AttributeValueMemberS{Value: "alice"},
		},
	}

	var out []u
	require.NoError(t, UnmarshalItems(items, &out))
	require.Len(t, out, 1)
	require.Equal(t, "u1", out[0].ID)
	require.Equal(t, "alice", out[0].Name)
}

func TestTransactionConditionHelpers(t *testing.T) {
	cond := Condition("field", "=", 1)
	require.Equal(t, core.TransactConditionKindField, cond.Kind)
	require.Equal(t, "field", cond.Field)
	require.Equal(t, "=", cond.Operator)
	require.Equal(t, 1, cond.Value)

	values := map[string]any{"v": 1}
	expr := ConditionExpression("a = :v", values)
	require.Equal(t, core.TransactConditionKindExpression, expr.Kind)
	require.Equal(t, "a = :v", expr.Expression)
	require.Equal(t, map[string]any{"v": 1}, expr.Values)

	values["v"] = 2
	require.Equal(t, map[string]any{"v": 1}, expr.Values)

	require.Equal(t, core.TransactConditionKindPrimaryKeyNotExists, IfNotExists().Kind)
	require.Equal(t, core.TransactConditionKindPrimaryKeyExists, IfExists().Kind)

	require.Equal(t, AtVersion(5), ConditionVersion(5))
}

func TestQuery_BuilderErrorsAndState(t *testing.T) {
	httpClient := newCapturingHTTPClient(nil)

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	qAny := db.Model(&cov3Item{}).
		Index("by-id").
		Offset(10).
		Select("Name").
		IfExists().
		WithCondition("Name", "=", "alice").
		WithConditionExpression("attribute_exists(#n)", map[string]any{"#n": "name"}).
		OrFilterGroup(func(sub core.Query) {
			_ = sub.Filter("Name", "=", "alice")
		})

	q, ok := qAny.(*queryPkg.Query)
	require.True(t, ok)
	compiled, err := q.Compile()
	require.NoError(t, err)
	require.Equal(t, "by-id", compiled.IndexName)
	require.NotEmpty(t, compiled.ProjectionExpression)
	require.Contains(t, mapValues(compiled.ExpressionAttributeNames), "name")

	// First recorded builder error should be returned.
	bad := db.Model(&cov3Item{}).
		Filter("Name", "", "x").
		WithConditionExpression("", nil)
	var out []cov3Item
	err = bad.All(&out)
	require.Error(t, err)
}

func mapValues(values map[string]string) []string {
	if len(values) == 0 {
		return nil
	}

	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, v)
	}
	return out
}

func TestQuery_BatchWriteCreateAndDelete(t *testing.T) {
	httpClient := newCapturingHTTPClient(map[string]string{
		"DynamoDB_20120810.BatchWriteItem": `{}`,
	})
	httpClient.SetResponseSequence("DynamoDB_20120810.BatchWriteItem", []stubbedResponse{
		{body: `{"UnprocessedItems":{"cov3_items":[{"PutRequest":{"Item":{"id":{"S":"u1"}}}}]}}`},
		{body: `{}`},
	})

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	items := []cov3Item{
		{ID: "u1", Name: "alice"},
		{ID: "u2", Name: "bob"},
	}

	require.NoError(t, db.Model(&cov3Item{}).BatchCreate(items))

	require.Error(t, db.Model(&cov3Item{}).BatchCreate(cov3Item{ID: "u3"}))

	require.NoError(t, db.Model(&cov3Item{}).BatchDelete([]any{"u1", "u2"}))

	require.Error(t, db.Model(&cov3Item{}).BatchDelete([]any{map[string]any{}}))

	require.NoError(t, db.Model(&cov3Item{}).BatchWrite(
		[]any{cov3Item{ID: "u3", Name: "carol"}},
		[]any{"u1"},
	))

	reqs := httpClient.Requests()
	require.GreaterOrEqual(t, countRequestsByTarget(reqs, "DynamoDB_20120810.BatchWriteItem"), 2)
}

func TestQuery_BatchDelete_CompositeKeyFromStruct(t *testing.T) {
	httpClient := newCapturingHTTPClient(map[string]string{
		"DynamoDB_20120810.BatchWriteItem": `{}`,
	})

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)

	db := mustDB(t, dbAny)
	require.NoError(t, db.Model(&cov3CompositeItem{}).BatchDelete([]any{cov3CompositeItem{PK: "p1", SK: "s1"}}))
}

func TestQuery_BatchGet_WithBuilder(t *testing.T) {
	httpClient := newCapturingHTTPClient(map[string]string{
		"DynamoDB_20120810.BatchGetItem": `{"Responses":{"cov3_items":[{"id":{"S":"u1"},"name":{"S":"alice"}}]},"UnprocessedKeys":{}}`,
	})

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	var out []cov3Item
	require.NoError(t, db.Model(&cov3Item{}).BatchGet([]any{"u1"}, &out))
	require.Len(t, out, 1)
	require.Equal(t, "u1", out[0].ID)

	out = nil
	builder := db.Model(&cov3Item{}).BatchGetBuilder()
	require.NoError(t, builder.Keys([]any{"u1"}).Execute(&out))
	require.Len(t, out, 1)
}

func TestQuery_WithRetry_FirstAndAll(t *testing.T) {
	httpClient := newCapturingHTTPClient(nil)
	httpClient.SetResponseSequence("DynamoDB_20120810.GetItem", []stubbedResponse{
		{body: `{"Item":null}`},
		{body: `{"Item":{"id":{"S":"u1"},"name":{"S":"alice"}}}`},
	})
	httpClient.SetResponseSequence("DynamoDB_20120810.Query", []stubbedResponse{
		{body: `{"Items":[],"Count":0,"ScannedCount":0}`},
		{body: `{"Items":[{"id":{"S":"u1"},"name":{"S":"alice"}}],"Count":1,"ScannedCount":1}`},
	})

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	var got cov3Item
	err = db.Model(&cov3Item{}).Where("ID", "=", "u1").WithRetry(1, 0).First(&got)
	require.NoError(t, err)
	require.Equal(t, "u1", got.ID)

	var all []cov3Item
	require.NoError(t, db.Model(&cov3Item{}).Where("ID", "=", "u1").WithRetry(1, 0).All(&all))
	require.Len(t, all, 1)

	reqs := httpClient.Requests()
	require.Equal(t, 2, countRequestsByTarget(reqs, "DynamoDB_20120810.GetItem"))
	require.Equal(t, 2, countRequestsByTarget(reqs, "DynamoDB_20120810.Query"))
}

func TestDB_TableOperationsAndAutoMigrate(t *testing.T) {
	httpClient := newCapturingHTTPClient(map[string]string{
		"DynamoDB_20120810.CreateTable":   `{}`,
		"DynamoDB_20120810.DeleteTable":   `{}`,
		"DynamoDB_20120810.DescribeTable": `{"Table":{"TableName":"cov3_items","TableStatus":"ACTIVE","BillingModeSummary":{"BillingMode":"PAY_PER_REQUEST"}}}`,
	})

	httpClient.SetResponseSequence("DynamoDB_20120810.DescribeTable", []stubbedResponse{
		{
			status:  400,
			headers: map[string]string{"X-Amzn-ErrorType": "ResourceNotFoundException"},
			body:    `{"__type":"com.amazonaws.dynamodb.v20120810#ResourceNotFoundException","message":"not found"}`,
		},
		{body: `{"Table":{"TableName":"cov3_items","TableStatus":"ACTIVE","BillingModeSummary":{"BillingMode":"PAY_PER_REQUEST"}}}`},
		{body: `{"Table":{"TableName":"cov3_items","TableStatus":"ACTIVE","BillingModeSummary":{"BillingMode":"PAY_PER_REQUEST"}}}`},
		{body: `{"Table":{"TableName":"cov3_items","TableStatus":"ACTIVE","BillingModeSummary":{"BillingMode":"PAY_PER_REQUEST"}}}`},
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

	require.Error(t, db.Migrate())

	// AutoMigrate should create the missing table (DescribeTable -> ResourceNotFound).
	require.NoError(t, db.AutoMigrate(&cov3Item{}))

	// AutoMigrateWithOptions defaults to no data copy.
	require.NoError(t, db.AutoMigrateWithOptions(&cov3Item{}))

	desc, err := db.DescribeTable(&cov3Item{})
	require.NoError(t, err)
	require.NotNil(t, desc)

	require.NoError(t, db.DeleteTable("cov3_items"))
}

func TestDB_TransactWriteAndExecutorStubs(t *testing.T) {
	httpClient := newCapturingHTTPClient(map[string]string{
		"DynamoDB_20120810.TransactWriteItems": `{}`,
		"DynamoDB_20120810.UpdateItem":         `{}`,
	})

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	require.Error(t, db.TransactWrite(context.Background(), nil))

	require.NoError(t, db.TransactWrite(context.Background(), func(tb core.TransactionBuilder) error {
		tb.Put(&cov3Item{ID: "u1", Name: "alice"})
		return nil
	}))

	qe := &queryExecutor{db: db}
	require.Error(t, qe.ExecuteQuery(&core.CompiledQuery{}, &struct{}{}))
	require.Error(t, qe.ExecuteScan(&core.CompiledQuery{}, &struct{}{}))
}

func TestQuery_BatchUpdateWithOptions(t *testing.T) {
	httpClient := newCapturingHTTPClient(map[string]string{
		"DynamoDB_20120810.UpdateItem": `{}`,
	})

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	items := []any{
		&cov3Item{ID: "u1", Name: "alice"},
		&cov3Item{ID: "u2", Name: "bob"},
	}

	require.NoError(t, db.Model(&cov3Item{}).BatchUpdateWithOptions(nil, []string{"Name"}))
	require.NoError(t, db.Model(&cov3Item{}).BatchUpdateWithOptions(items, []string{"Name"}))
}

func TestMultiAccount_SanitizationAndCaching(t *testing.T) {
	globalLambdaDB = nil
	lambdaOnce = sync.Once{}

	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret")
	t.Setenv("AWS_LAMBDA_FUNCTION_NAME", "fn")
	t.Setenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE", "512")

	httpClient := newCapturingHTTPClient(nil)
	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	mdb, err := NewMultiAccount(map[string]AccountConfig{
		"partner1": {
			RoleARN:         "arn:aws:iam::123456789012:role/TestRole",
			ExternalID:      "external",
			Region:          "us-east-1",
			SessionDuration: 10 * time.Minute,
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, mdb.Close())
	})

	base, err := mdb.Partner("")
	require.NoError(t, err)
	require.NotNil(t, base)

	_, err = mdb.Partner("unknown")
	require.Error(t, err)

	p1, err := mdb.Partner("partner1")
	require.NoError(t, err)
	p1b, err := mdb.Partner("partner1")
	require.NoError(t, err)
	require.Same(t, p1, p1b)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	withCtx := mdb.WithContext(ctx)
	require.Same(t, mdb.cache, withCtx.cache)

	mdb.RemovePartner("partner1")
	_, err = mdb.Partner("partner1")
	require.Error(t, err)

	mdb.AddPartner("partner1", AccountConfig{
		RoleARN:    "arn:aws:iam::123456789012:role/TestRole",
		ExternalID: "external",
		Region:     "us-east-1",
	})
	_, err = mdb.Partner("partner1")
	require.NoError(t, err)

	require.True(t, isNumeric("123"))
	require.False(t, isNumeric("12a3"))

	require.Equal(t, "[empty]", sanitizePartnerID(""))
	require.Equal(t, "1234****9012", sanitizePartnerID("123456789012"))
	require.Equal(t, "[masked_arn]", sanitizePartnerID("arn:aws:iam::123:role/test"))
	require.Equal(t, "abc_def-123", sanitizePartnerID("abc_def-123!!"))

	opID := generateOperationID()
	require.NotEmpty(t, opID)
}

func TestTransaction_ConditionExpressionValuesAreCloned(t *testing.T) {
	values := map[string]any{"v": 1}
	cond := ConditionExpression("a = :v", values)
	values["v"] = 2
	require.Equal(t, map[string]any{"v": 1}, cond.Values)
}

func TestRetryableQuery_DefaultResponsesDontPanic(t *testing.T) {
	// Ensure default Query/Scan/GetItem stubs are sufficient for basic operations.
	httpClient := newCapturingHTTPClient(nil)

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	var got cov3Item
	err = db.Model(&cov3Item{}).Where("ID", "=", "u1").First(&got)
	require.Error(t, err)
}

func TestErrorQuery_ImplementsCoreQuery(t *testing.T) {
	q := &errorQuery{err: errors.New("boom")}

	require.Same(t, q, q.Where("id", "=", "u1"))
	require.Same(t, q, q.Index("by-id"))
	require.Same(t, q, q.Filter("name", "=", "alice"))
	require.Same(t, q, q.OrFilter("name", "=", "alice"))
	require.Same(t, q, q.FilterGroup(func(core.Query) {}))
	require.Same(t, q, q.OrFilterGroup(func(core.Query) {}))
	require.Same(t, q, q.IfNotExists())
	require.Same(t, q, q.IfExists())
	require.Same(t, q, q.WithCondition("name", "=", "alice"))
	require.Same(t, q, q.WithConditionExpression("a = :v", map[string]any{":v": 1}))
	require.Same(t, q, q.OrderBy("id", "ASC"))
	require.Same(t, q, q.Limit(10))
	require.Same(t, q, q.Offset(10))
	require.Same(t, q, q.Select("id"))
	require.Same(t, q, q.ConsistentRead())
	require.Same(t, q, q.WithRetry(1, 0))
	require.Same(t, q, q.WithContext(context.Background()))
	require.Same(t, q, q.ParallelScan(0, 1))
	require.Same(t, q, q.Cursor("c"))

	require.Error(t, q.Create())
	require.Error(t, q.CreateOrUpdate())
	require.Error(t, q.Delete())
	require.Error(t, q.Update("Name"))
	require.Error(t, q.First(&cov3Item{}))
	var out []cov3Item
	require.Error(t, q.All(&out))
	_, err := q.AllPaginated(&out)
	require.Error(t, err)
	_, err = q.Count()
	require.Error(t, err)

	require.Error(t, q.Scan(&out))
	require.Error(t, q.BatchGet([]any{"u1"}, &out))
	require.Error(t, q.BatchGetWithOptions([]any{"u1"}, &out, nil))
	require.Error(t, q.BatchCreate(out))
	require.Error(t, q.BatchDelete([]any{"u1"}))
	require.Error(t, q.BatchWrite([]any{}, []any{}))
	require.Error(t, q.BatchUpdateWithOptions(nil, []string{"Name"}))

	updateBuilder := q.UpdateBuilder()
	require.NotNil(t, updateBuilder)
	require.Error(t, updateBuilder.Set("Name", "x").Execute())
	require.Error(t, q.ScanAllSegments(&out, 1))
	require.Error(t, q.SetCursor("c"))

	builder := q.BatchGetBuilder()
	require.Error(t, builder.
		Keys([]any{"u1"}).
		ChunkSize(1).
		ConsistentRead().
		Parallel(1).
		WithRetry(nil).
		Select("id").
		OnProgress(func(int, int) {}).
		OnError(func([]any, error) error { return nil }).
		Execute(&out))
}
