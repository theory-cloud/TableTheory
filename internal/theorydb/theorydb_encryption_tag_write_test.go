package theorydb

import (
	"bytes"
	"context"
	"encoding/base64"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/session"
)

type encryptedTagWriteModel struct {
	PK     string `theorydb:"pk,attr:pk" json:"pk"`
	SK     string `theorydb:"sk,attr:sk" json:"sk"`
	Secret string `theorydb:"encrypted,attr:secret" json:"secret"`
}

func (encryptedTagWriteModel) TableName() string {
	return "EncryptedTagWriteModels"
}

func TestEncryptedTag_WriteTimeEncryption(t *testing.T) {
	plaintextKey := bytes.Repeat([]byte{0x01}, 32)
	plaintextKeyB64 := base64.StdEncoding.EncodeToString(plaintextKey)
	edkBytes := []byte("ciphertext-data-key")
	edkB64 := base64.StdEncoding.EncodeToString(edkBytes)

	httpClient := newCapturingHTTPClient(nil)
	httpClient.SetResponseSequence("TrentService.GenerateDataKey", []stubbedResponse{{
		headers: map[string]string{"Content-Type": "application/x-amz-json-1.1"},
		body:    `{"Plaintext":"` + plaintextKeyB64 + `","CiphertextBlob":"` + edkB64 + `","KeyId":"arn:aws:kms:us-east-1:111111111111:key/test"}`,
	}})

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	dbAny, err := New(session.Config{
		Region:    "us-east-1",
		KMSKeyARN: "arn:aws:kms:us-east-1:111111111111:key/test",
	})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	t.Run("CreateOrUpdate encrypts item attribute", func(t *testing.T) {
		err := db.Model(&encryptedTagWriteModel{
			PK:     "pk1",
			SK:     "sk1",
			Secret: "top-secret",
		}).CreateOrUpdate()
		require.NoError(t, err)

		put := findCapturedRequest(t, httpClient, "DynamoDB_20120810.PutItem")
		item := requireMap(t, put.Payload["Item"])
		secret := requireMap(t, item["secret"])
		secretM := requireMap(t, secret["M"])

		v := requireMap(t, secretM["v"])
		require.Equal(t, "1", v["N"])

		edk := requireMap(t, secretM["edk"])
		require.Equal(t, edkB64, edk["B"])

		nonce := requireMap(t, secretM["nonce"])
		require.NotEmpty(t, nonce["B"])

		ct := requireMap(t, secretM["ct"])
		require.NotEmpty(t, ct["B"])
	})

	t.Run("Update encrypts expression attribute values", func(t *testing.T) {
		httpClient.mu.Lock()
		httpClient.requests = nil
		httpClient.callCount = make(map[string]int)
		httpClient.mu.Unlock()

		err := db.Model(&encryptedTagWriteModel{
			PK:     "pk2",
			SK:     "sk2",
			Secret: "new-secret",
		}).Update("Secret")
		require.NoError(t, err)

		update := findCapturedRequest(t, httpClient, "DynamoDB_20120810.UpdateItem")
		values := requireMap(t, update.Payload["ExpressionAttributeValues"])

		// Find any value placeholder that is an encrypted envelope.
		var envelope map[string]any
		for _, raw := range values {
			av := requireMap(t, raw)
			if m, ok := av["M"].(map[string]any); ok {
				envelope = m
				break
			}
		}
		require.NotNil(t, envelope)
		require.Contains(t, envelope, "v")
		require.Contains(t, envelope, "edk")
		require.Contains(t, envelope, "nonce")
		require.Contains(t, envelope, "ct")
	})

	t.Run("UpdateBuilder encrypts expression attribute values", func(t *testing.T) {
		httpClient.mu.Lock()
		httpClient.requests = nil
		httpClient.callCount = make(map[string]int)
		httpClient.mu.Unlock()

		builder := db.Model(&encryptedTagWriteModel{}).
			Where("PK", "=", "pk3").
			Where("SK", "=", "sk3").
			UpdateBuilder()

		err := builder.Set("Secret", "builder-secret").Execute()
		require.NoError(t, err)

		update := findCapturedRequest(t, httpClient, "DynamoDB_20120810.UpdateItem")
		values := requireMap(t, update.Payload["ExpressionAttributeValues"])

		var envelope map[string]any
		for _, raw := range values {
			av := requireMap(t, raw)
			if m, ok := av["M"].(map[string]any); ok {
				envelope = m
				break
			}
		}
		require.NotNil(t, envelope)
		require.Contains(t, envelope, "v")
	})

	t.Run("Transact Put encrypts item attribute", func(t *testing.T) {
		httpClient.mu.Lock()
		httpClient.requests = nil
		httpClient.callCount = make(map[string]int)
		httpClient.mu.Unlock()

		err := db.Transact().
			Put(&encryptedTagWriteModel{PK: "pk4", SK: "sk4", Secret: "tx-secret"}).
			Execute()
		require.NoError(t, err)

		tx := findCapturedRequest(t, httpClient, "DynamoDB_20120810.TransactWriteItems")
		itemsAny := requireSlice(t, tx.Payload["TransactItems"])
		require.NotEmpty(t, itemsAny)

		first := requireMap(t, itemsAny[0])
		put := requireMap(t, first["Put"])
		item := requireMap(t, put["Item"])
		secret := requireMap(t, item["secret"])
		secretM := requireMap(t, secret["M"])
		require.Contains(t, secretM, "ct")
	})
}

func findCapturedRequest(t *testing.T, c *capturingHTTPClient, target string) capturedRequest {
	t.Helper()

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, req := range c.requests {
		if req.Target == target {
			return req
		}
	}
	t.Fatalf("missing captured request for target %s", target)
	return capturedRequest{}
}

func requireMap(t *testing.T, value any) map[string]any {
	t.Helper()
	m, ok := value.(map[string]any)
	require.True(t, ok, "expected map[string]any, got %T", value)
	return m
}

func requireSlice(t *testing.T, value any) []any {
	t.Helper()
	s, ok := value.([]any)
	require.True(t, ok, "expected []any, got %T", value)
	return s
}
