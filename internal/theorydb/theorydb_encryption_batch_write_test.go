package theorydb

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmsTypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/session"
)

type encryptedBatchRetryModel struct {
	PK     string `theorydb:"pk,attr:pk" json:"pk"`
	SK     string `theorydb:"sk,attr:sk" json:"sk"`
	Secret string `theorydb:"encrypted,attr:secret" json:"secret"`
}

func (encryptedBatchRetryModel) TableName() string { return "EncryptedBatchRetry" }

func TestEncryptedBatchWrite_UnprocessedItemsAreNotDoubleEncrypted(t *testing.T) {
	plaintextKey := bytes.Repeat([]byte{0x01}, 32)
	kmsClient := &countingBatchRetryKMS{
		keyARN:    "arn:aws:kms:us-east-1:111111111111:key/test",
		plaintext: plaintextKey,
		edk:       []byte("ciphertext-data-key"),
	}

	ddbClient := &batchWriteRetryHTTPClient{}

	dbAny, err := New(session.Config{
		Region:              "us-east-1",
		Endpoint:            "http://example.com",
		CredentialsProvider: credentials.NewStaticCredentialsProvider("AKID", "SECRET", ""),
		KMSKeyARN:           kmsClient.keyARN,
		KMSClient:           kmsClient,
		EncryptionRand:      bytes.NewReader(bytes.Repeat([]byte{0x02}, 1024)),
		DynamoDBOptions: []func(*dynamodb.Options){
			func(o *dynamodb.Options) {
				o.HTTPClient = ddbClient
			},
		},
	})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	err = db.Model(&encryptedBatchRetryModel{}).BatchWrite([]any{
		encryptedBatchRetryModel{
			PK:     "pk1",
			SK:     "sk1",
			Secret: "top-secret",
		},
	}, nil)
	require.NoError(t, err)

	require.Equal(t, 2, ddbClient.CallCount(), "expected first call plus unprocessed-item retry")
	require.Equal(t, 1, kmsClient.GenerateDataKeyCallCount(), "encrypted field should be encrypted exactly once")

	persistedItem := ddbClient.LastPutItem(t, "EncryptedBatchRetry")

	metadata, err := db.registry.GetMetadata(&encryptedBatchRetryModel{})
	require.NoError(t, err)
	executor := &queryExecutor{db: db, metadata: metadata, ctx: context.Background()}

	require.NoError(t, executor.decryptItem(persistedItem))

	var got encryptedBatchRetryModel
	require.NoError(t, executor.unmarshalItem(persistedItem, &got))
	require.Equal(t, "top-secret", got.Secret)
}

type countingBatchRetryKMS struct {
	keyARN    string
	plaintext []byte
	edk       []byte

	mu            sync.Mutex
	generateCalls int
	decryptCalls  int
}

func (c *countingBatchRetryKMS) GenerateDataKey(
	_ context.Context,
	params *kms.GenerateDataKeyInput,
	_ ...func(*kms.Options),
) (*kms.GenerateDataKeyOutput, error) {
	if params == nil || params.KeyId == nil || aws.ToString(params.KeyId) == "" {
		return nil, errors.New("missing key id")
	}
	if aws.ToString(params.KeyId) != c.keyARN {
		return nil, fmt.Errorf("unexpected key id: %s", aws.ToString(params.KeyId))
	}
	if params.KeySpec != kmsTypes.DataKeySpecAes256 {
		return nil, fmt.Errorf("unexpected key spec: %s", params.KeySpec)
	}

	c.mu.Lock()
	c.generateCalls++
	c.mu.Unlock()

	return &kms.GenerateDataKeyOutput{
		Plaintext:      append([]byte(nil), c.plaintext...),
		CiphertextBlob: append([]byte(nil), c.edk...),
	}, nil
}

func (c *countingBatchRetryKMS) Decrypt(
	_ context.Context,
	params *kms.DecryptInput,
	_ ...func(*kms.Options),
) (*kms.DecryptOutput, error) {
	if params == nil || len(params.CiphertextBlob) == 0 {
		return nil, errors.New("missing ciphertext")
	}
	if !bytes.Equal(params.CiphertextBlob, c.edk) {
		return nil, errors.New("unexpected ciphertext")
	}

	c.mu.Lock()
	c.decryptCalls++
	c.mu.Unlock()

	return &kms.DecryptOutput{
		Plaintext: append([]byte(nil), c.plaintext...),
	}, nil
}

func (c *countingBatchRetryKMS) GenerateDataKeyCallCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.generateCalls
}

type batchWriteRetryHTTPClient struct {
	requests []capturedRequest
	mu       sync.Mutex
}

func (c *batchWriteRetryHTTPClient) Do(req *http.Request) (*http.Response, error) {
	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	if err := req.Body.Close(); err != nil {
		return nil, err
	}

	payload := make(map[string]any)
	if len(bodyBytes) > 0 {
		if err := json.Unmarshal(bodyBytes, &payload); err != nil {
			return nil, err
		}
	}

	target := req.Header.Get("X-Amz-Target")

	c.mu.Lock()
	c.requests = append(c.requests, capturedRequest{
		Target:  target,
		Payload: payload,
	})
	call := len(c.requests)
	c.mu.Unlock()

	if target != "DynamoDB_20120810.BatchWriteItem" {
		return batchWriteRetryJSONResponse(http.StatusBadRequest, `{"__type":"UnknownOperationException","message":"unexpected operation"}`), nil
	}

	body := `{"UnprocessedItems":{}}`
	if call == 1 {
		requestItems, err := json.Marshal(payload["RequestItems"])
		if err != nil {
			return nil, err
		}
		body = `{"UnprocessedItems":` + string(requestItems) + `}`
	}

	return batchWriteRetryJSONResponse(http.StatusOK, body), nil
}

func (c *batchWriteRetryHTTPClient) CallCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.requests)
}

func (c *batchWriteRetryHTTPClient) LastPutItem(t *testing.T, tableName string) map[string]types.AttributeValue {
	t.Helper()

	c.mu.Lock()
	defer c.mu.Unlock()

	require.NotEmpty(t, c.requests)
	lastRequest := c.requests[len(c.requests)-1]
	requestItems := requireMap(t, lastRequest.Payload["RequestItems"])
	tableRequests := requireSlice(t, requestItems[tableName])
	require.Len(t, tableRequests, 1)

	writeRequest := requireMap(t, tableRequests[0])
	putRequest := requireMap(t, writeRequest["PutRequest"])
	itemWire := requireMap(t, putRequest["Item"])

	item := make(map[string]types.AttributeValue, len(itemWire))
	for name, value := range itemWire {
		item[name] = wireAttributeValue(t, value)
	}
	return item
}

func wireAttributeValue(t *testing.T, value any) types.AttributeValue {
	t.Helper()

	typed := requireMap(t, value)
	require.Len(t, typed, 1)
	for kind, raw := range typed {
		switch kind {
		case "S":
			s, ok := raw.(string)
			require.True(t, ok, "expected S string, got %T", raw)
			return &types.AttributeValueMemberS{Value: s}
		case "N":
			n, ok := raw.(string)
			require.True(t, ok, "expected N string, got %T", raw)
			return &types.AttributeValueMemberN{Value: n}
		case "B":
			encoded, ok := raw.(string)
			require.True(t, ok, "expected B string, got %T", raw)
			decoded, err := base64.StdEncoding.DecodeString(encoded)
			require.NoError(t, err)
			return &types.AttributeValueMemberB{Value: decoded}
		case "M":
			rawMap := requireMap(t, raw)
			out := make(map[string]types.AttributeValue, len(rawMap))
			for name, nested := range rawMap {
				out[name] = wireAttributeValue(t, nested)
			}
			return &types.AttributeValueMemberM{Value: out}
		default:
			t.Fatalf("unsupported wire AttributeValue kind %q", kind)
		}
	}

	t.Fatal("empty wire AttributeValue")
	return nil
}

func batchWriteRetryJSONResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:     http.Header{"Content-Type": []string{"application/x-amz-json-1.0"}},
		Body:       io.NopCloser(bytes.NewReader([]byte(body))),
	}
}
