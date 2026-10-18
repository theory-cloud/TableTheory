package theorydb

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v6/internal/encryption"
	theorydbErrors "github.com/theory-cloud/tabletheory/v6/pkg/errors"
	"github.com/theory-cloud/tabletheory/v6/pkg/session"
)

func TestEncryptedTag_DecryptBindsEnvelopeToRecord(t *testing.T) {
	keyARN := "arn:aws:kms:us-east-1:111111111111:key/test"
	plaintextKey := bytes.Repeat([]byte{0x03}, 32)

	dbAny, err := New(session.Config{
		Region:    "us-east-1",
		KMSKeyARN: keyARN,
		KMSClient: fakeKMS{
			edk:       []byte("edk"),
			plaintext: plaintextKey,
		},
	})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	require.NoError(t, db.registry.Register(&encryptedTagWriteModel{}))
	metadata, err := db.registry.GetMetadata(&encryptedTagWriteModel{})
	require.NoError(t, err)

	svc := encryption.NewService(keyARN, fakeKMS{
		edk:       []byte("edk"),
		plaintext: plaintextKey,
	})

	binding, err := encryption.NewRecordBindingFromKey(metadata.TableName, metadata, map[string]types.AttributeValue{
		"pk": &types.AttributeValueMemberS{Value: "pk1"},
		"sk": &types.AttributeValueMemberS{Value: "sk1"},
	})
	require.NoError(t, err)

	envelope, err := svc.EncryptAttributeValueWithBinding(
		context.Background(),
		binding,
		"secret",
		&types.AttributeValueMemberS{Value: "top-secret"},
	)
	require.NoError(t, err)

	executor := &queryExecutor{db: db, metadata: metadata, ctx: context.Background()}

	t.Run("the envelope decrypts on its own record", func(t *testing.T) {
		item := map[string]types.AttributeValue{
			"pk":     &types.AttributeValueMemberS{Value: "pk1"},
			"sk":     &types.AttributeValueMemberS{Value: "sk1"},
			"secret": envelope,
		}
		require.NoError(t, executor.decryptItem(item))
	})

	t.Run("a replayed envelope fails on another record", func(t *testing.T) {
		item := map[string]types.AttributeValue{
			"pk":     &types.AttributeValueMemberS{Value: "pk2"},
			"sk":     &types.AttributeValueMemberS{Value: "sk1"},
			"secret": envelope,
		}

		err := executor.decryptItem(item)
		require.Error(t, err)

		var fieldErr *theorydbErrors.EncryptedFieldError
		require.True(t, errors.As(err, &fieldErr))
		require.Equal(t, "decrypt", fieldErr.Operation)
		require.Equal(t, "Secret", fieldErr.Field)
		require.ErrorIs(t, err, theorydbErrors.ErrInvalidEncryptedEnvelope)
	})

	t.Run("a projected item without the key fails closed", func(t *testing.T) {
		item := map[string]types.AttributeValue{
			"secret": envelope,
		}

		err := executor.decryptItem(item)
		require.Error(t, err)
		require.ErrorIs(t, err, theorydbErrors.ErrInvalidEncryptedEnvelope)
	})

	t.Run("legacy v1 envelopes still decrypt", func(t *testing.T) {
		legacy, err := svc.EncryptAttributeValue(context.Background(), "secret", &types.AttributeValueMemberS{Value: "legacy"})
		require.NoError(t, err)

		item := map[string]types.AttributeValue{
			"pk":     &types.AttributeValueMemberS{Value: "pk1"},
			"sk":     &types.AttributeValueMemberS{Value: "sk1"},
			"secret": legacy,
		}
		require.NoError(t, executor.decryptItem(item))
	})
}
