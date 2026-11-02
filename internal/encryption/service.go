package encryption

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmsTypes "github.com/aws/aws-sdk-go-v2/service/kms/types"

	customerrors "github.com/theory-cloud/tabletheory/v6/pkg/errors"
)

const (
	envelopeVersionV1 = "1"
	// envelopeVersionV2 authenticates the record binding in the AES-GCM
	// additional data. v1 envelopes remain readable for existing data.
	envelopeVersionV2 = "2"

	envelopeKeyVersion    = "v"
	envelopeKeyEDK        = "edk"
	envelopeKeyNonce      = "nonce"
	envelopeKeyCiphertext = "ct"

	// encryptionContextAADKey is the KMS EncryptionContext key that binds the
	// same AAD into the wrapped data key for v2 envelopes. KMS authenticates
	// the context on decrypt, so a data key wrapped for one record cannot be
	// unwrapped for another even before AES-GCM runs.
	encryptionContextAADKey = "theorydb:encryption:aad"

	// envelopeNonceSize is the AES-GCM nonce length every envelope must carry.
	// cipher.NewGCM panics when Open receives any other length, so a corrupted
	// nonce is rejected while parsing the envelope, before KMS or Open runs.
	envelopeNonceSize = 12
)

type kmsAPI interface {
	GenerateDataKey(ctx context.Context, params *kms.GenerateDataKeyInput, optFns ...func(*kms.Options)) (*kms.GenerateDataKeyOutput, error)
	Decrypt(ctx context.Context, params *kms.DecryptInput, optFns ...func(*kms.Options)) (*kms.DecryptOutput, error)
}

// Service implements envelope encryption for DynamoDB attribute values using AWS KMS.
type Service struct {
	kms  kmsAPI
	rand io.Reader

	keyARN string
}

func NewService(keyARN string, kmsClient kmsAPI) *Service {
	return NewServiceWithRand(keyARN, kmsClient, rand.Reader)
}

func NewServiceFromAWSConfig(keyARN string, cfg aws.Config) *Service {
	return NewServiceFromAWSConfigWithRand(keyARN, cfg, rand.Reader)
}

func NewServiceWithRand(keyARN string, kmsClient kmsAPI, rng io.Reader) *Service {
	if rng == nil {
		rng = rand.Reader
	}
	return &Service{
		keyARN: keyARN,
		kms:    kmsClient,
		rand:   rng,
	}
}

func NewServiceFromAWSConfigWithRand(keyARN string, cfg aws.Config, rng io.Reader) *Service {
	return NewServiceWithRand(keyARN, kms.NewFromConfig(cfg), rng)
}

// EncryptAttributeValue writes a legacy v1 envelope that authenticates only the
// attribute name. New writes to stored records should use
// EncryptAttributeValueWithBinding so an envelope cannot be replayed onto
// another record or table.
func (s *Service) EncryptAttributeValue(ctx context.Context, attributeName string, av types.AttributeValue) (types.AttributeValue, error) {
	return s.encryptAttributeValue(ctx, attributeName, av, aadForAttribute(attributeName), envelopeVersionV1, nil)
}

// EncryptAttributeValueWithBinding writes a v2 envelope bound to the record
// identity (table, partition key, sort key) in addition to the attribute name.
// The same binding is also supplied to KMS as the data key's EncryptionContext.
func (s *Service) EncryptAttributeValueWithBinding(ctx context.Context, binding RecordBinding, attributeName string, av types.AttributeValue) (types.AttributeValue, error) {
	if !binding.Valid() {
		return nil, fmt.Errorf("encrypted envelope record binding is required")
	}
	aad, err := aadForBoundRecord(attributeName, binding)
	if err != nil {
		return nil, err
	}
	return s.encryptAttributeValue(ctx, attributeName, av, aad, envelopeVersionV2, encryptionContextForAAD(aad, envelopeVersionV2))
}

func (s *Service) encryptAttributeValue(ctx context.Context, attributeName string, av types.AttributeValue, aad []byte, envelopeVersion string, encryptionContext map[string]string) (types.AttributeValue, error) {
	if s == nil {
		return nil, fmt.Errorf("encryption service is nil")
	}
	if s.kms == nil {
		return nil, fmt.Errorf("kms client is nil")
	}
	if s.keyARN == "" {
		return nil, fmt.Errorf("kms key ARN is empty")
	}
	if attributeName == "" {
		return nil, fmt.Errorf("attribute name is empty")
	}

	plaintext, err := encodeAttributeValue(av)
	if err != nil {
		return nil, err
	}

	dataKey, err := s.kms.GenerateDataKey(ctx, &kms.GenerateDataKeyInput{
		KeyId:             aws.String(s.keyARN),
		KeySpec:           kmsTypes.DataKeySpecAes256,
		EncryptionContext: encryptionContext,
	})
	if err != nil {
		return nil, fmt.Errorf("kms GenerateDataKey failed: %w", err)
	}
	if len(dataKey.Plaintext) != 32 {
		return nil, fmt.Errorf("unexpected data key plaintext length: %d", len(dataKey.Plaintext))
	}
	if len(dataKey.CiphertextBlob) == 0 {
		return nil, fmt.Errorf("kms returned empty ciphertext data key")
	}

	block, err := aes.NewCipher(dataKey.Plaintext)
	if err != nil {
		return nil, fmt.Errorf("aes cipher init failed: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("aes-gcm init failed: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(s.rand, nonce); err != nil {
		return nil, fmt.Errorf("nonce generation failed: %w", err)
	}

	ct := gcm.Seal(nil, nonce, plaintext, aad)

	return &types.AttributeValueMemberM{
		Value: map[string]types.AttributeValue{
			envelopeKeyVersion:    &types.AttributeValueMemberN{Value: envelopeVersion},
			envelopeKeyEDK:        &types.AttributeValueMemberB{Value: dataKey.CiphertextBlob},
			envelopeKeyNonce:      &types.AttributeValueMemberB{Value: nonce},
			envelopeKeyCiphertext: &types.AttributeValueMemberB{Value: ct},
		},
	}, nil
}

// DecryptAttributeValue decrypts a legacy v1 envelope. A v2 envelope requires
// the record binding and is rejected here rather than decrypted without it.
func (s *Service) DecryptAttributeValue(ctx context.Context, attributeName string, envelope types.AttributeValue) (types.AttributeValue, error) {
	return s.decryptAttributeValue(ctx, attributeName, envelope, RecordBinding{}, false)
}

// DecryptAttributeValueWithBinding decrypts v2 envelopes with their record
// binding and still accepts legacy v1 envelopes for read compatibility.
func (s *Service) DecryptAttributeValueWithBinding(ctx context.Context, binding RecordBinding, attributeName string, envelope types.AttributeValue) (types.AttributeValue, error) {
	return s.decryptAttributeValue(ctx, attributeName, envelope, binding, true)
}

func (s *Service) decryptAttributeValue(ctx context.Context, attributeName string, envelope types.AttributeValue, binding RecordBinding, bindingSupplied bool) (types.AttributeValue, error) {
	if err := s.validateDecryptInputs(attributeName); err != nil {
		return nil, err
	}

	parts, err := parseEncryptedEnvelope(envelope)
	if err != nil {
		return nil, err
	}

	aad := aadForAttribute(attributeName)
	var encryptionContext map[string]string
	if parts.version == envelopeVersionV2 {
		if !bindingSupplied || !binding.Valid() {
			return nil, fmt.Errorf("%w: encrypted envelope v2 requires the record binding", customerrors.ErrInvalidEncryptedEnvelope)
		}
		aad, err = aadForBoundRecord(attributeName, binding)
		if err != nil {
			return nil, err
		}
		encryptionContext = encryptionContextForAAD(aad, parts.version)
	}

	dataKey, err := s.decryptDataKey(ctx, parts.edk, encryptionContext)
	if err != nil {
		return nil, err
	}

	gcm, err := newGCM(dataKey)
	if err != nil {
		return nil, err
	}
	if len(parts.nonce) != gcm.NonceSize() {
		return nil, fmt.Errorf(
			"%w: invalid nonce length %d (want %d)",
			customerrors.ErrInvalidEncryptedEnvelope, len(parts.nonce), gcm.NonceSize(),
		)
	}

	plaintext, err := gcm.Open(nil, parts.nonce, parts.ciphertext, aad)
	if err != nil {
		return nil, fmt.Errorf("%w: aes-gcm decrypt failed: %w", customerrors.ErrInvalidEncryptedEnvelope, err)
	}

	return decodeAttributeValue(plaintext)
}

func aadForAttribute(attributeName string) []byte {
	return []byte(fmt.Sprintf("theorydb:encrypted:v1|attr=%s", attributeName))
}

// encryptionContextForAAD returns the KMS EncryptionContext for an envelope
// version. v1 envelopes carry no context so existing data stays readable.
func encryptionContextForAAD(aad []byte, version string) map[string]string {
	if version != envelopeVersionV2 {
		return nil
	}
	return map[string]string{encryptionContextAADKey: string(aad)}
}

type encryptedEnvelopeParts struct {
	version    string
	edk        []byte
	nonce      []byte
	ciphertext []byte
}

func (s *Service) validateDecryptInputs(attributeName string) error {
	if s == nil {
		return fmt.Errorf("encryption service is nil")
	}
	if s.kms == nil {
		return fmt.Errorf("kms client is nil")
	}
	if s.keyARN == "" {
		return fmt.Errorf("kms key ARN is empty")
	}
	if attributeName == "" {
		return fmt.Errorf("attribute name is empty")
	}
	return nil
}

func parseEncryptedEnvelope(envelope types.AttributeValue) (encryptedEnvelopeParts, error) {
	env, ok := envelope.(*types.AttributeValueMemberM)
	if !ok || env == nil {
		return encryptedEnvelopeParts{}, fmt.Errorf("%w: expected encrypted envelope map, got %T", customerrors.ErrInvalidEncryptedEnvelope, envelope)
	}

	version, err := encryptedEnvelopeVersion(env.Value)
	if err != nil {
		return encryptedEnvelopeParts{}, err
	}

	edkAV, ok := env.Value[envelopeKeyEDK].(*types.AttributeValueMemberB)
	if !ok || edkAV == nil || len(edkAV.Value) == 0 {
		return encryptedEnvelopeParts{}, fmt.Errorf("%w: missing encrypted data key", customerrors.ErrInvalidEncryptedEnvelope)
	}

	nonceAV, ok := env.Value[envelopeKeyNonce].(*types.AttributeValueMemberB)
	if !ok || nonceAV == nil || len(nonceAV.Value) == 0 {
		return encryptedEnvelopeParts{}, fmt.Errorf("%w: missing nonce", customerrors.ErrInvalidEncryptedEnvelope)
	}
	if len(nonceAV.Value) != envelopeNonceSize {
		return encryptedEnvelopeParts{}, fmt.Errorf(
			"%w: invalid nonce length %d (want %d)",
			customerrors.ErrInvalidEncryptedEnvelope, len(nonceAV.Value), envelopeNonceSize,
		)
	}

	ctAV, ok := env.Value[envelopeKeyCiphertext].(*types.AttributeValueMemberB)
	if !ok || ctAV == nil {
		return encryptedEnvelopeParts{}, fmt.Errorf("%w: missing ciphertext", customerrors.ErrInvalidEncryptedEnvelope)
	}

	return encryptedEnvelopeParts{
		version:    version,
		edk:        edkAV.Value,
		nonce:      nonceAV.Value,
		ciphertext: ctAV.Value,
	}, nil
}

func encryptedEnvelopeVersion(values map[string]types.AttributeValue) (string, error) {
	versionAV, ok := values[envelopeKeyVersion].(*types.AttributeValueMemberN)
	if !ok || versionAV == nil {
		return "", fmt.Errorf("%w: unsupported encrypted envelope version", customerrors.ErrInvalidEncryptedEnvelope)
	}
	if versionAV.Value != envelopeVersionV1 && versionAV.Value != envelopeVersionV2 {
		return "", fmt.Errorf("%w: unsupported encrypted envelope version", customerrors.ErrInvalidEncryptedEnvelope)
	}
	return versionAV.Value, nil
}

func (s *Service) decryptDataKey(ctx context.Context, edk []byte, encryptionContext map[string]string) ([]byte, error) {
	dec, err := s.kms.Decrypt(ctx, &kms.DecryptInput{
		CiphertextBlob:    edk,
		KeyId:             aws.String(s.keyARN),
		EncryptionContext: encryptionContext,
	})
	if err != nil {
		return nil, fmt.Errorf("kms Decrypt failed: %w", err)
	}
	if len(dec.Plaintext) != 32 {
		return nil, fmt.Errorf("unexpected data key plaintext length: %d", len(dec.Plaintext))
	}
	return dec.Plaintext, nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes cipher init failed: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("aes-gcm init failed: %w", err)
	}
	return gcm, nil
}

type avJSON struct {
	Type string            `json:"t"`
	S    *string           `json:"s,omitempty"`
	N    *string           `json:"n,omitempty"`
	B    *string           `json:"b,omitempty"`
	BOOL *bool             `json:"bool,omitempty"`
	L    []avJSON          `json:"l,omitempty"`
	M    map[string]avJSON `json:"m,omitempty"`
	SS   []string          `json:"ss,omitempty"`
	NS   []string          `json:"ns,omitempty"`
	BS   []string          `json:"bs,omitempty"`
	NULL bool              `json:"null,omitempty"`
}

func encodeAttributeValue(av types.AttributeValue) ([]byte, error) {
	enc, err := marshalAVJSON(av)
	if err != nil {
		return nil, err
	}
	out, err := json.Marshal(enc)
	if err != nil {
		return nil, fmt.Errorf("failed to encode attribute value: %w", err)
	}
	return out, nil
}

func decodeAttributeValue(data []byte) (types.AttributeValue, error) {
	var enc avJSON
	if err := json.Unmarshal(data, &enc); err != nil {
		return nil, fmt.Errorf("failed to decode attribute value: %w", err)
	}
	return unmarshalAVJSON(enc)
}
