package theorydb

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/internal/encryption"
	theorydbErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/session"
)

type fakeKMS struct {
	edk       []byte
	plaintext []byte
}

func (f fakeKMS) GenerateDataKey(_ context.Context, _ *kms.GenerateDataKeyInput, _ ...func(*kms.Options)) (*kms.GenerateDataKeyOutput, error) {
	return &kms.GenerateDataKeyOutput{
		CiphertextBlob: f.edk,
		Plaintext:      f.plaintext,
	}, nil
}

func (f fakeKMS) Decrypt(_ context.Context, _ *kms.DecryptInput, _ ...func(*kms.Options)) (*kms.DecryptOutput, error) {
	return &kms.DecryptOutput{
		Plaintext: f.plaintext,
	}, nil
}

func TestEncryptedTag_ReadTimeDecryption(t *testing.T) {
	keyARN := "arn:aws:kms:us-east-1:111111111111:key/test"
	plaintextKey := bytes.Repeat([]byte{0x02}, 32)
	plaintextKeyB64 := base64.StdEncoding.EncodeToString(plaintextKey)

	httpClient := newCapturingHTTPClient(nil)
	httpClient.SetResponseSequence("TrentService.Decrypt", []stubbedResponse{{
		headers: map[string]string{"Content-Type": "application/x-amz-json-1.1"},
		body:    `{"Plaintext":"` + plaintextKeyB64 + `"}`,
	}})

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	dbAny, err := New(session.Config{
		Region:    "us-east-1",
		KMSKeyARN: keyARN,
	})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	require.NoError(t, db.registry.Register(&encryptedTagWriteModel{}))
	metadata, err := db.registry.GetMetadata(&encryptedTagWriteModel{})
	require.NoError(t, err)

	encSvc := encryption.NewService(keyARN, fakeKMS{
		edk:       []byte("edk"),
		plaintext: plaintextKey,
	})

	envelope, err := encSvc.EncryptAttributeValue(context.Background(), "secret", &types.AttributeValueMemberS{Value: "top-secret"})
	require.NoError(t, err)

	executor := &queryExecutor{db: db, metadata: metadata, ctx: context.Background()}

	t.Run("Unmarshal decrypts into struct fields", func(t *testing.T) {
		item := map[string]types.AttributeValue{
			"pk":     &types.AttributeValueMemberS{Value: "pk1"},
			"sk":     &types.AttributeValueMemberS{Value: "sk1"},
			"secret": envelope,
		}

		var out encryptedTagWriteModel
		require.NoError(t, executor.decryptItem(item))
		require.NoError(t, executor.unmarshalItem(item, &out))
		require.Equal(t, "top-secret", out.Secret)

		decrypt := findCapturedRequest(t, httpClient, "TrentService.Decrypt")
		require.Equal(t, keyARN, decrypt.Payload["KeyId"])
	})

	t.Run("Invalid envelope returns typed error", func(t *testing.T) {
		item := map[string]types.AttributeValue{
			"pk":     &types.AttributeValueMemberS{Value: "pk2"},
			"sk":     &types.AttributeValueMemberS{Value: "sk2"},
			"secret": &types.AttributeValueMemberS{Value: "plaintext"},
		}

		err := executor.decryptItem(item)
		require.Error(t, err)

		var fieldErr *theorydbErrors.EncryptedFieldError
		require.True(t, errors.As(err, &fieldErr))
		require.Equal(t, "decrypt", fieldErr.Operation)
		require.Equal(t, "Secret", fieldErr.Field)
		require.ErrorIs(t, err, theorydbErrors.ErrInvalidEncryptedEnvelope)
		require.NotContains(t, err.Error(), "plaintext")
	})
}
