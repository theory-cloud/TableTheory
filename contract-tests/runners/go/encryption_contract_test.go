package contracttests

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory"
	"github.com/theory-cloud/tabletheory/pkg/session"
)

type encryptionContractModel struct {
	PK      string `theorydb:"pk"`
	SK      string `theorydb:"sk"`
	SecretA string `theorydb:"encrypted,attr:secretA"`
	SecretB string `theorydb:"encrypted,attr:secretB"`
}

func (encryptionContractModel) TableName() string { return "enc_contract" }

func TestEncryption_EnvelopeShapeAndAADBinding(t *testing.T) {
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

	_, err = recreatePKSKTable(ctx, ddb, "enc_contract", "PK", "SK")
	require.NoError(t, err)
	defer func() {
		_, _ = ddb.DeleteTable(ctx, &dynamodb.DeleteTableInput{TableName: aws.String("enc_contract")})
	}()

	plaintextKey := bytesRepeat(0x01, 32)
	plaintextKeyB64 := base64.StdEncoding.EncodeToString(plaintextKey)
	edk := []byte("ciphertext-data-key")
	edkB64 := base64.StdEncoding.EncodeToString(edk)

	httpClient := &http.Client{
		Transport: &kmsStubTransport{
			inner:           http.DefaultTransport,
			plaintextKeyB64: plaintextKeyB64,
			edkB64:          edkB64,
		},
	}

	db, err := theorydb.New(session.Config{
		Region:    region,
		Endpoint:  endpoint,
		KMSKeyARN: "arn:aws:kms:us-east-1:111111111111:key/test",
		AWSConfigOptions: []func(*config.LoadOptions) error{
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")),
			config.WithRegion(region),
			config.WithHTTPClient(httpClient),
		},
	})
	require.NoError(t, err)

	const pk = "USER#1"
	const sk = "PROFILE"

	err = db.WithContext(ctx).Model(&encryptionContractModel{
		PK:      pk,
		SK:      sk,
		SecretA: "top-secret",
		SecretB: "other-secret",
	}).CreateOrUpdate()
	require.NoError(t, err)

	var got encryptionContractModel
	err = db.WithContext(ctx).
		Model(&encryptionContractModel{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&got)
	require.NoError(t, err)
	require.Equal(t, "top-secret", got.SecretA)
	require.Equal(t, "other-secret", got.SecretB)

	raw, err := ddb.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String("enc_contract"),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: sk},
		},
		ConsistentRead: aws.Bool(true),
	})
	require.NoError(t, err)
	require.NotNil(t, raw.Item)

	requireEncryptedEnvelope(t, raw.Item["secretA"], edk)
	requireEncryptedEnvelope(t, raw.Item["secretB"], edk)

	// Swap encrypted envelopes between attributes; AAD binding must cause decrypt failure.
	_, err = ddb.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String("enc_contract"),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: sk},
		},
		UpdateExpression: aws.String("SET #a = :b, #b = :a"),
		ExpressionAttributeNames: map[string]string{
			"#a": "secretA",
			"#b": "secretB",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":a": raw.Item["secretA"],
			":b": raw.Item["secretB"],
		},
	})
	require.NoError(t, err)

	var swapped encryptionContractModel
	err = db.WithContext(ctx).
		Model(&encryptionContractModel{}).
		Where("PK", "=", pk).
		Where("SK", "=", sk).
		First(&swapped)
	require.Error(t, err)
}

func pingDynamoDB(ctx context.Context, ddb *dynamodb.Client) error {
	_, err := ddb.ListTables(ctx, &dynamodb.ListTablesInput{Limit: aws.Int32(1)})
	return err
}

func dynamodbLocalClient(ctx context.Context, region string, endpoint string) (*dynamodb.Client, error) {
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")),
	)
	if err != nil {
		return nil, err
	}

	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	}), nil
}

func recreatePKSKTable(ctx context.Context, ddb *dynamodb.Client, tableName string, pk string, sk string) (*dynamodb.CreateTableOutput, error) {
	_, _ = ddb.DeleteTable(ctx, &dynamodb.DeleteTableInput{TableName: aws.String(tableName)})
	_ = dynamodb.NewTableNotExistsWaiter(ddb).Wait(ctx, &dynamodb.DescribeTableInput{TableName: aws.String(tableName)}, 2*time.Minute)

	out, err := ddb.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(tableName),
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String(pk), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String(sk), AttributeType: types.ScalarAttributeTypeS},
		},
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String(pk), KeyType: types.KeyTypeHash},
			{AttributeName: aws.String(sk), KeyType: types.KeyTypeRange},
		},
		BillingMode: types.BillingModePayPerRequest,
	})
	if err != nil {
		return nil, err
	}

	if err := dynamodb.NewTableExistsWaiter(ddb).Wait(ctx, &dynamodb.DescribeTableInput{TableName: aws.String(tableName)}, 2*time.Minute); err != nil {
		return nil, err
	}

	return out, nil
}

func requireEncryptedEnvelope(t *testing.T, av types.AttributeValue, wantEDK []byte) {
	t.Helper()

	m, ok := av.(*types.AttributeValueMemberM)
	require.True(t, ok, "expected encrypted envelope map, got %T", av)
	require.NotNil(t, m.Value)

	version, ok := m.Value["v"].(*types.AttributeValueMemberN)
	require.True(t, ok, "missing v")
	require.Equal(t, "1", version.Value)

	edk, ok := m.Value["edk"].(*types.AttributeValueMemberB)
	require.True(t, ok, "missing edk")
	require.Equal(t, wantEDK, edk.Value)

	nonce, ok := m.Value["nonce"].(*types.AttributeValueMemberB)
	require.True(t, ok, "missing nonce")
	require.NotEmpty(t, nonce.Value)

	ct, ok := m.Value["ct"].(*types.AttributeValueMemberB)
	require.True(t, ok, "missing ct")
	require.NotEmpty(t, ct.Value)
}

type kmsStubTransport struct {
	inner http.RoundTripper

	mu sync.Mutex

	plaintextKeyB64 string
	edkB64          string
}

func (t *kmsStubTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	target := req.Header.Get("X-Amz-Target")
	if strings.HasPrefix(target, "TrentService.") {
		t.mu.Lock()
		defer t.mu.Unlock()

		switch target {
		case "TrentService.GenerateDataKey":
			return jsonResponse(200, `{"Plaintext":"`+t.plaintextKeyB64+`","CiphertextBlob":"`+t.edkB64+`","KeyId":"arn:aws:kms:us-east-1:111111111111:key/test"}`), nil
		case "TrentService.Decrypt":
			return jsonResponse(200, `{"Plaintext":"`+t.plaintextKeyB64+`","KeyId":"arn:aws:kms:us-east-1:111111111111:key/test"}`), nil
		default:
			return nil, fmt.Errorf("kms stub: unexpected target %q", target)
		}
	}

	host := strings.ToLower(req.URL.Host)
	if host == "localhost:8000" || host == "127.0.0.1:8000" {
		return t.inner.RoundTrip(req)
	}

	return nil, fmt.Errorf("kms stub: unexpected network call to %s", req.URL.String())
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/x-amz-json-1.1"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func bytesRepeat(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}
