package encryption

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmsTypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/stretchr/testify/require"

	theorydbErrors "github.com/theory-cloud/tabletheory/v6/pkg/errors"
)

type fakeKMS struct {
	generateErr error
	decryptErr  error
	keyARN      string
	plaintext   []byte
	edk         []byte
}

func (f *fakeKMS) GenerateDataKey(ctx context.Context, params *kms.GenerateDataKeyInput, optFns ...func(*kms.Options)) (*kms.GenerateDataKeyOutput, error) {
	if f.generateErr != nil {
		return nil, f.generateErr
	}
	if params == nil || params.KeyId == nil || aws.ToString(params.KeyId) == "" {
		return nil, errors.New("missing key id")
	}
	if f.keyARN != "" && aws.ToString(params.KeyId) != f.keyARN {
		return nil, errors.New("unexpected key id")
	}
	if params.KeySpec != kmsTypes.DataKeySpecAes256 {
		return nil, errors.New("unexpected key spec")
	}
	return &kms.GenerateDataKeyOutput{
		Plaintext:      append([]byte(nil), f.plaintext...),
		CiphertextBlob: append([]byte(nil), f.edk...),
	}, nil
}

func (f *fakeKMS) Decrypt(ctx context.Context, params *kms.DecryptInput, optFns ...func(*kms.Options)) (*kms.DecryptOutput, error) {
	if f.decryptErr != nil {
		return nil, f.decryptErr
	}
	if params == nil || len(params.CiphertextBlob) == 0 {
		return nil, errors.New("missing ciphertext")
	}
	if f.keyARN != "" && (params.KeyId == nil || aws.ToString(params.KeyId) != f.keyARN) {
		return nil, errors.New("unexpected key id")
	}
	if len(f.edk) > 0 && !bytes.Equal(params.CiphertextBlob, f.edk) {
		return nil, errors.New("unexpected ciphertext")
	}
	return &kms.DecryptOutput{
		Plaintext: append([]byte(nil), f.plaintext...),
	}, nil
}

func newTestService(t *testing.T) *Service {
	t.Helper()

	key := bytes.Repeat([]byte{0x11}, 32)
	edk := []byte("edk")
	fake := &fakeKMS{
		keyARN:    "arn:aws:kms:us-east-1:123456789012:key/00000000-0000-0000-0000-000000000000",
		plaintext: key,
		edk:       edk,
	}
	svc := NewService(fake.keyARN, fake)
	svc.rand = bytes.NewReader(bytes.Repeat([]byte{0x02}, 12))
	return svc
}

func TestNewServiceFromAWSConfig(t *testing.T) {
	svc := NewServiceFromAWSConfig("arn:aws:kms:us-east-1:123456789012:key/00000000-0000-0000-0000-000000000000", aws.Config{})
	require.NotNil(t, svc)
	require.NotNil(t, svc.kms)
	require.NotNil(t, svc.rand)
	require.Equal(t, "arn:aws:kms:us-east-1:123456789012:key/00000000-0000-0000-0000-000000000000", svc.keyARN)
}

func TestService_EncryptDecrypt_RoundTrip_String(t *testing.T) {
	svc := newTestService(t)

	plaintext := &types.AttributeValueMemberS{Value: "hello"}
	envelope, err := svc.EncryptAttributeValue(context.Background(), "secret", plaintext)
	require.NoError(t, err)

	decrypted, err := svc.DecryptAttributeValue(context.Background(), "secret", envelope)
	require.NoError(t, err)
	require.Equal(t, plaintext, decrypted)

	_, err = svc.DecryptAttributeValue(context.Background(), "different", envelope)
	require.Error(t, err)
	require.ErrorContains(t, err, "aes-gcm decrypt failed")
}

func TestService_EncryptAttributeValue_Errors(t *testing.T) {
	ctx := context.Background()

	t.Run("nil_service", func(t *testing.T) {
		var svc *Service
		_, err := svc.EncryptAttributeValue(ctx, "secret", &types.AttributeValueMemberS{Value: "x"})
		require.Error(t, err)
		require.ErrorContains(t, err, "encryption service is nil")
	})

	t.Run("nil_kms", func(t *testing.T) {
		svc := NewService("arn", nil)
		_, err := svc.EncryptAttributeValue(ctx, "secret", &types.AttributeValueMemberS{Value: "x"})
		require.Error(t, err)
		require.ErrorContains(t, err, "kms client is nil")
	})

	t.Run("empty_kms_key_arn", func(t *testing.T) {
		svc := NewService("", &fakeKMS{plaintext: bytes.Repeat([]byte{0x11}, 32), edk: []byte("edk")})
		_, err := svc.EncryptAttributeValue(ctx, "secret", &types.AttributeValueMemberS{Value: "x"})
		require.Error(t, err)
		require.ErrorContains(t, err, "kms key ARN is empty")
	})

	t.Run("empty_attribute_name", func(t *testing.T) {
		svc := newTestService(t)
		_, err := svc.EncryptAttributeValue(ctx, "", &types.AttributeValueMemberS{Value: "x"})
		require.Error(t, err)
		require.ErrorContains(t, err, "attribute name is empty")
	})

	t.Run("unsupported_attribute_value", func(t *testing.T) {
		svc := newTestService(t)
		_, err := svc.EncryptAttributeValue(ctx, "secret", nil)
		require.Error(t, err)
		require.ErrorContains(t, err, "unsupported attribute value type")
	})

	t.Run("kms_generate_data_key_failure", func(t *testing.T) {
		fake := &fakeKMS{
			keyARN:      "arn:aws:kms:us-east-1:123456789012:key/00000000-0000-0000-0000-000000000000",
			plaintext:   bytes.Repeat([]byte{0x11}, 32),
			edk:         []byte("edk"),
			generateErr: errors.New("kms down"),
		}
		svc := NewService(fake.keyARN, fake)
		_, err := svc.EncryptAttributeValue(ctx, "secret", &types.AttributeValueMemberS{Value: "x"})
		require.Error(t, err)
		require.ErrorContains(t, err, "kms GenerateDataKey failed")
	})

	t.Run("kms_returns_short_plaintext", func(t *testing.T) {
		fake := &fakeKMS{
			keyARN:    "arn:aws:kms:us-east-1:123456789012:key/00000000-0000-0000-0000-000000000000",
			plaintext: bytes.Repeat([]byte{0x11}, 31),
			edk:       []byte("edk"),
		}
		svc := NewService(fake.keyARN, fake)
		_, err := svc.EncryptAttributeValue(ctx, "secret", &types.AttributeValueMemberS{Value: "x"})
		require.Error(t, err)
		require.ErrorContains(t, err, "unexpected data key plaintext length")
	})

	t.Run("kms_returns_empty_ciphertext", func(t *testing.T) {
		fake := &fakeKMS{
			keyARN:    "arn:aws:kms:us-east-1:123456789012:key/00000000-0000-0000-0000-000000000000",
			plaintext: bytes.Repeat([]byte{0x11}, 32),
			edk:       nil,
		}
		svc := NewService(fake.keyARN, fake)
		_, err := svc.EncryptAttributeValue(ctx, "secret", &types.AttributeValueMemberS{Value: "x"})
		require.Error(t, err)
		require.ErrorContains(t, err, "kms returned empty ciphertext data key")
	})

	t.Run("nonce_generation_failure", func(t *testing.T) {
		svc := newTestService(t)
		svc.rand = bytes.NewReader([]byte{0x01})

		_, err := svc.EncryptAttributeValue(ctx, "secret", &types.AttributeValueMemberS{Value: "x"})
		require.Error(t, err)
		require.ErrorContains(t, err, "nonce generation failed")
	})
}

func TestService_DecryptAttributeValue_Errors(t *testing.T) {
	ctx := context.Background()

	t.Run("validateDecryptInputs_errors", func(t *testing.T) {
		var nilSvc *Service
		_, err := nilSvc.DecryptAttributeValue(ctx, "secret", &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{}})
		require.Error(t, err)
		require.ErrorContains(t, err, "encryption service is nil")

		svcNoKMS := NewService("arn", nil)
		_, err = svcNoKMS.DecryptAttributeValue(ctx, "secret", &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{}})
		require.Error(t, err)
		require.ErrorContains(t, err, "kms client is nil")

		svcNoKey := NewService("", &fakeKMS{plaintext: bytes.Repeat([]byte{0x11}, 32), edk: []byte("edk")})
		_, err = svcNoKey.DecryptAttributeValue(ctx, "secret", &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{}})
		require.Error(t, err)
		require.ErrorContains(t, err, "kms key ARN is empty")

		svc := newTestService(t)
		_, err = svc.DecryptAttributeValue(ctx, "", &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{}})
		require.Error(t, err)
		require.ErrorContains(t, err, "attribute name is empty")
	})

	t.Run("invalid_envelope_type", func(t *testing.T) {
		svc := newTestService(t)
		_, err := svc.DecryptAttributeValue(ctx, "secret", &types.AttributeValueMemberS{Value: "x"})
		require.Error(t, err)
		require.ErrorIs(t, err, theorydbErrors.ErrInvalidEncryptedEnvelope)
	})

	t.Run("invalid_envelope_version", func(t *testing.T) {
		svc := newTestService(t)
		envelope := &types.AttributeValueMemberM{
			Value: map[string]types.AttributeValue{
				envelopeKeyVersion: &types.AttributeValueMemberN{Value: "99"},
				envelopeKeyEDK:     &types.AttributeValueMemberB{Value: []byte("edk")},
				envelopeKeyNonce:   &types.AttributeValueMemberB{Value: []byte("nonce")},
				envelopeKeyCiphertext: &types.AttributeValueMemberB{
					Value: []byte("ciphertext"),
				},
			},
		}
		_, err := svc.DecryptAttributeValue(ctx, "secret", envelope)
		require.Error(t, err)
		require.ErrorIs(t, err, theorydbErrors.ErrInvalidEncryptedEnvelope)
	})

	t.Run("missing_envelope_parts", func(t *testing.T) {
		svc := newTestService(t)

		missingEDK := &types.AttributeValueMemberM{
			Value: map[string]types.AttributeValue{
				envelopeKeyVersion:    &types.AttributeValueMemberN{Value: envelopeVersionV1},
				envelopeKeyNonce:      &types.AttributeValueMemberB{Value: []byte("nonce")},
				envelopeKeyCiphertext: &types.AttributeValueMemberB{Value: []byte("ciphertext")},
			},
		}
		_, err := svc.DecryptAttributeValue(ctx, "secret", missingEDK)
		require.Error(t, err)
		require.ErrorIs(t, err, theorydbErrors.ErrInvalidEncryptedEnvelope)

		missingNonce := &types.AttributeValueMemberM{
			Value: map[string]types.AttributeValue{
				envelopeKeyVersion:    &types.AttributeValueMemberN{Value: envelopeVersionV1},
				envelopeKeyEDK:        &types.AttributeValueMemberB{Value: []byte("edk")},
				envelopeKeyCiphertext: &types.AttributeValueMemberB{Value: []byte("ciphertext")},
			},
		}
		_, err = svc.DecryptAttributeValue(ctx, "secret", missingNonce)
		require.Error(t, err)
		require.ErrorIs(t, err, theorydbErrors.ErrInvalidEncryptedEnvelope)

		missingCiphertext := &types.AttributeValueMemberM{
			Value: map[string]types.AttributeValue{
				envelopeKeyVersion: &types.AttributeValueMemberN{Value: envelopeVersionV1},
				envelopeKeyEDK:     &types.AttributeValueMemberB{Value: []byte("edk")},
				envelopeKeyNonce:   &types.AttributeValueMemberB{Value: []byte("nonce")},
			},
		}
		_, err = svc.DecryptAttributeValue(ctx, "secret", missingCiphertext)
		require.Error(t, err)
		require.ErrorIs(t, err, theorydbErrors.ErrInvalidEncryptedEnvelope)
	})

	t.Run("kms_decrypt_failure_propagates", func(t *testing.T) {
		key := bytes.Repeat([]byte{0x11}, 32)
		fake := &fakeKMS{
			keyARN:     "arn:aws:kms:us-east-1:123456789012:key/00000000-0000-0000-0000-000000000000",
			plaintext:  key,
			edk:        []byte("edk"),
			decryptErr: errors.New("kms down"),
		}

		svc := NewService(fake.keyARN, fake)
		svc.rand = bytes.NewReader(bytes.Repeat([]byte{0x02}, 12))

		envelope, err := svc.EncryptAttributeValue(ctx, "secret", &types.AttributeValueMemberS{Value: "x"})
		require.NoError(t, err)

		_, err = svc.DecryptAttributeValue(ctx, "secret", envelope)
		require.Error(t, err)
		require.ErrorContains(t, err, "kms Decrypt failed")
	})

	t.Run("decryptDataKey_rejects_short_plaintext", func(t *testing.T) {
		fake := &fakeKMS{
			keyARN:    "arn:aws:kms:us-east-1:123456789012:key/00000000-0000-0000-0000-000000000000",
			plaintext: bytes.Repeat([]byte{0x11}, 31),
			edk:       []byte("edk"),
		}
		svc := NewService(fake.keyARN, fake)
		_, err := svc.decryptDataKey(ctx, []byte("edk"))
		require.Error(t, err)
		require.ErrorContains(t, err, "unexpected data key plaintext length")
	})

	t.Run("newGCM_rejects_invalid_key_length", func(t *testing.T) {
		_, err := newGCM([]byte("short"))
		require.Error(t, err)
	})

	t.Run("decodeAttributeValue_rejects_invalid_json", func(t *testing.T) {
		_, err := decodeAttributeValue([]byte("not-json"))
		require.Error(t, err)
		require.ErrorContains(t, err, "failed to decode attribute value")
	})
}

func TestEncryptAttributeValueWithBindingRejectsReplay(t *testing.T) {
	svc := newTestService(t)
	bindingA := RecordBinding{Table: "blog", PartitionKey: &types.AttributeValueMemberS{Value: "post#1"}}
	bindingB := RecordBinding{Table: "blog", PartitionKey: &types.AttributeValueMemberS{Value: "post#2"}}
	bindingOtherTable := RecordBinding{Table: "other", PartitionKey: &types.AttributeValueMemberS{Value: "post#1"}}

	envelope, err := svc.EncryptAttributeValueWithBinding(context.Background(), bindingA, "secret", &types.AttributeValueMemberS{Value: "top-secret"})
	require.NoError(t, err)

	parts, err := parseEncryptedEnvelope(envelope)
	require.NoError(t, err)
	require.Equal(t, envelopeVersionV2, parts.version)

	for name, binding := range map[string]RecordBinding{
		"another record": bindingB,
		"another table":  bindingOtherTable,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := svc.DecryptAttributeValueWithBinding(context.Background(), binding, "secret", envelope)
			require.Error(t, err)
		})
	}

	plaintext, err := svc.DecryptAttributeValueWithBinding(context.Background(), bindingA, "secret", envelope)
	require.NoError(t, err)
	require.Equal(t, "top-secret", plaintext.(*types.AttributeValueMemberS).Value)

	_, err = svc.DecryptAttributeValue(context.Background(), "secret", envelope)
	require.Error(t, err, "a v2 envelope must not decrypt without its binding")
}

func TestEncryptAttributeValueWithBindingBindsTheSortKey(t *testing.T) {
	svc := newTestService(t)
	binding := RecordBinding{
		Table:        "blog",
		PartitionKey: &types.AttributeValueMemberS{Value: "release#a"},
		SortKey:      &types.AttributeValueMemberS{Value: "ACTUAL"},
	}
	replayed := RecordBinding{
		Table:        "blog",
		PartitionKey: &types.AttributeValueMemberS{Value: "release#a"},
		SortKey:      &types.AttributeValueMemberS{Value: "EVENT#1"},
	}

	envelope, err := svc.EncryptAttributeValueWithBinding(context.Background(), binding, "secret", &types.AttributeValueMemberS{Value: "top-secret"})
	require.NoError(t, err)

	_, err = svc.DecryptAttributeValueWithBinding(context.Background(), replayed, "secret", envelope)
	require.Error(t, err)
	_, err = svc.DecryptAttributeValueWithBinding(context.Background(), binding, "secret", envelope)
	require.NoError(t, err)
}

func TestDecryptAttributeValueWithBindingAcceptsLegacyV1Envelopes(t *testing.T) {
	svc := newTestService(t)
	binding := RecordBinding{Table: "blog", PartitionKey: &types.AttributeValueMemberS{Value: "post#1"}}

	envelope, err := svc.EncryptAttributeValue(context.Background(), "secret", &types.AttributeValueMemberS{Value: "legacy"})
	require.NoError(t, err)

	plaintext, err := svc.DecryptAttributeValueWithBinding(context.Background(), binding, "secret", envelope)
	require.NoError(t, err)
	require.Equal(t, "legacy", plaintext.(*types.AttributeValueMemberS).Value)

	plaintext, err = svc.DecryptAttributeValue(context.Background(), "secret", envelope)
	require.NoError(t, err)
	require.Equal(t, "legacy", plaintext.(*types.AttributeValueMemberS).Value)
}
