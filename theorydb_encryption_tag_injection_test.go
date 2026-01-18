package theorydb

import (
	"bytes"
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"github.com/theory-cloud/tabletheory/pkg/session"
)

type encryptedTagInjectionModel struct {
	PK        string    `theorydb:"pk,attr:pk" json:"pk"`
	SK        string    `theorydb:"sk,attr:sk" json:"sk"`
	CreatedAt time.Time `theorydb:"created_at,attr:createdAt" json:"createdAt"`
	UpdatedAt time.Time `theorydb:"updated_at,attr:updatedAt" json:"updatedAt"`
	Secret    string    `theorydb:"encrypted,attr:secret" json:"secret"`
	Version   int64     `theorydb:"version,attr:version" json:"version"`
}

func (encryptedTagInjectionModel) TableName() string {
	return "EncryptedTagInjectionModels"
}

func TestEncryptedTag_UsesConfigInjectedKMSRandAndNow(t *testing.T) {
	plaintextKey := bytes.Repeat([]byte{0x01}, 32)
	edkBytes := []byte("ciphertext-data-key")
	nonceBytes := bytes.Repeat([]byte{0x02}, 12)
	fixedNow := time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC)
	fixedNowStr := fixedNow.Format(time.RFC3339Nano)

	kmsMock := new(mocks.MockKMSClient)
	kmsMock.On("GenerateDataKey", mock.Anything, mock.Anything, mock.Anything).
		Return(&kms.GenerateDataKeyOutput{
			Plaintext:      plaintextKey,
			CiphertextBlob: edkBytes,
		}, nil).
		Once()

	httpClient := newCapturingHTTPClient(nil)
	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	dbAny, err := New(session.Config{
		Region:         "us-east-1",
		KMSKeyARN:      "arn:aws:kms:us-east-1:111111111111:key/test",
		KMSClient:      kmsMock,
		EncryptionRand: bytes.NewReader(nonceBytes),
		Now:            func() time.Time { return fixedNow },
	})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	err = db.Model(&encryptedTagInjectionModel{
		PK:     "pk1",
		SK:     "sk1",
		Secret: "top-secret",
	}).CreateOrUpdate()
	require.NoError(t, err)

	kmsMock.AssertExpectations(t)

	put := findCapturedRequest(t, httpClient, "DynamoDB_20120810.PutItem")
	item := requireMap(t, put.Payload["Item"])

	created := requireMap(t, item["createdAt"])
	require.Equal(t, fixedNowStr, created["S"])

	updated := requireMap(t, item["updatedAt"])
	require.Equal(t, fixedNowStr, updated["S"])

	version := requireMap(t, item["version"])
	require.Equal(t, "0", version["N"])

	secret := requireMap(t, item["secret"])
	secretM := requireMap(t, secret["M"])

	edk := requireMap(t, secretM["edk"])
	require.Equal(t, base64.StdEncoding.EncodeToString(edkBytes), edk["B"])

	nonce := requireMap(t, secretM["nonce"])
	require.Equal(t, base64.StdEncoding.EncodeToString(nonceBytes), nonce["B"])
}
