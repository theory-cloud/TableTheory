package schema

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v4/pkg/model"
)

type migrateEncryptionPlainSource struct {
	PK     string `theorydb:"pk,attr:PK" json:"PK"`
	Secret string `theorydb:"attr:secret" json:"secret"`
}

func (migrateEncryptionPlainSource) TableName() string { return "migration_plain_source" }

type migrateEncryptionEncryptedTarget struct {
	PK     string `theorydb:"pk,attr:PK" json:"PK"`
	Secret string `theorydb:"attr:secret,encrypted" json:"secret"`
}

func (migrateEncryptionEncryptedTarget) TableName() string { return "migration_encrypted_target" }

type migrateEncryptionSharedSource struct {
	PK     string `theorydb:"pk,attr:PK" json:"PK"`
	Secret string `theorydb:"attr:secret,encrypted" json:"secret"`
}

func (migrateEncryptionSharedSource) TableName() string { return "migration_shared_source" }

type migrateEncryptionSharedTarget struct {
	PK     string `theorydb:"pk,attr:PK" json:"PK"`
	Secret string `theorydb:"attr:secret,encrypted" json:"secret"`
}

func (migrateEncryptionSharedTarget) TableName() string { return "migration_shared_target" }

func migrationMetadata(t *testing.T, value any) *model.Metadata {
	t.Helper()
	registry := model.NewRegistry()
	require.NoError(t, registry.Register(value))
	metadata, err := registry.GetMetadata(value)
	require.NoError(t, err)
	return metadata
}

func envelopeAttributeValue() types.AttributeValue {
	return &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
		"v":     &types.AttributeValueMemberN{Value: "1"},
		"edk":   &types.AttributeValueMemberB{Value: []byte("edk")},
		"nonce": &types.AttributeValueMemberB{Value: []byte("nonce")},
		"ct":    &types.AttributeValueMemberB{Value: []byte("ciphertext")},
	}}
}

type recordingEncryptor struct {
	names []string
}

func (r *recordingEncryptor) EncryptAttributeValue(_ context.Context, attributeName string, _ types.AttributeValue) (types.AttributeValue, error) {
	r.names = append(r.names, attributeName)
	return envelopeAttributeValue(), nil
}

func TestPreflightMigrationEncryption(t *testing.T) {
	plainSource := migrationMetadata(t, &migrateEncryptionPlainSource{})
	encryptedTarget := migrationMetadata(t, &migrateEncryptionEncryptedTarget{})
	sharedSource := migrationMetadata(t, &migrateEncryptionSharedSource{})
	sharedTarget := migrationMetadata(t, &migrateEncryptionSharedTarget{})

	t.Run("refuses plaintext into encrypted target without a provider", func(t *testing.T) {
		err := preflightMigrationEncryption(plainSource, encryptedTarget, false, encryptedTarget.TableName)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrMigrationEncryptionRequired)
		assert.Contains(t, err.Error(), "secret")
	})

	t.Run("allows the same transition when encryption is configured", func(t *testing.T) {
		require.NoError(t, preflightMigrationEncryption(plainSource, encryptedTarget, true, encryptedTarget.TableName))
	})

	t.Run("ordinary migration with no encrypted target is unaffected", func(t *testing.T) {
		require.NoError(t, preflightMigrationEncryption(plainSource, plainSource, false, plainSource.TableName))
	})

	t.Run("carry-through of an already encrypted attribute needs no provider", func(t *testing.T) {
		require.NoError(t, preflightMigrationEncryption(sharedSource, sharedTarget, false, sharedTarget.TableName))
	})
}

func TestMigrationEncryptionPlanGuardItem(t *testing.T) {
	plainSource := migrationMetadata(t, &migrateEncryptionPlainSource{})
	encryptedTarget := migrationMetadata(t, &migrateEncryptionEncryptedTarget{})
	sharedSource := migrationMetadata(t, &migrateEncryptionSharedSource{})
	sharedTarget := migrationMetadata(t, &migrateEncryptionSharedTarget{})

	t.Run("carries a shared encrypted attribute through unchanged", func(t *testing.T) {
		plan := newMigrationEncryptionPlan(sharedSource, sharedTarget, nil, sharedTarget.TableName)
		item := map[string]types.AttributeValue{"PK": &types.AttributeValueMemberS{Value: "1"}, "secret": envelopeAttributeValue()}
		require.NoError(t, plan.guardItem(context.Background(), item))
		assert.True(t, isEncryptedEnvelope(item["secret"]))
	})

	t.Run("refuses plaintext in a source attribute declared encrypted", func(t *testing.T) {
		plan := newMigrationEncryptionPlan(sharedSource, sharedTarget, nil, sharedTarget.TableName)
		item := map[string]types.AttributeValue{"PK": &types.AttributeValueMemberS{Value: "1"}, "secret": &types.AttributeValueMemberS{Value: "plaintext"}}
		err := plan.guardItem(context.Background(), item)
		require.ErrorIs(t, err, ErrMigrationEncryptionRequired)
	})

	t.Run("refuses an envelope copied into a plaintext target attribute", func(t *testing.T) {
		plan := newMigrationEncryptionPlan(sharedSource, plainSource, nil, plainSource.TableName)
		item := map[string]types.AttributeValue{"PK": &types.AttributeValueMemberS{Value: "1"}, "secret": envelopeAttributeValue()}
		err := plan.guardItem(context.Background(), item)
		require.ErrorIs(t, err, ErrMigrationEncryptionRequired)
	})

	t.Run("refuses re-encrypting an existing envelope under a new attribute name", func(t *testing.T) {
		plan := newMigrationEncryptionPlan(plainSource, encryptedTarget, &recordingEncryptor{}, encryptedTarget.TableName)
		item := map[string]types.AttributeValue{"PK": &types.AttributeValueMemberS{Value: "1"}, "secret": envelopeAttributeValue()}
		err := plan.guardItem(context.Background(), item)
		require.ErrorIs(t, err, ErrMigrationEncryptionRequired)
	})

	t.Run("encrypts plaintext into an encrypted target attribute", func(t *testing.T) {
		encryptor := &recordingEncryptor{}
		plan := newMigrationEncryptionPlan(plainSource, encryptedTarget, encryptor, encryptedTarget.TableName)
		item := map[string]types.AttributeValue{"PK": &types.AttributeValueMemberS{Value: "1"}, "secret": &types.AttributeValueMemberS{Value: "top-secret"}}
		require.NoError(t, plan.guardItem(context.Background(), item))
		assert.Equal(t, []string{"secret"}, encryptor.names)
		assert.True(t, isEncryptedEnvelope(item["secret"]))
	})

	t.Run("refuses encryption without a provider at write time", func(t *testing.T) {
		plan := newMigrationEncryptionPlan(plainSource, encryptedTarget, nil, encryptedTarget.TableName)
		item := map[string]types.AttributeValue{"PK": &types.AttributeValueMemberS{Value: "1"}, "secret": &types.AttributeValueMemberS{Value: "top-secret"}}
		require.ErrorIs(t, plan.guardItem(context.Background(), item), ErrMigrationEncryptionRequired)
	})

	t.Run("nil plan guards nothing", func(t *testing.T) {
		var plan *migrationEncryptionPlan
		require.NoError(t, plan.guardItem(context.Background(), map[string]types.AttributeValue{}))
	})
}

func TestIsEncryptedEnvelope(t *testing.T) {
	assert.False(t, isEncryptedEnvelope(nil))
	assert.False(t, isEncryptedEnvelope(&types.AttributeValueMemberS{Value: "plaintext"}))
	assert.False(t, isEncryptedEnvelope(&types.AttributeValueMemberM{Value: map[string]types.AttributeValue{}}))
	assert.True(t, isEncryptedEnvelope(envelopeAttributeValue()))

	missingCiphertext, ok := envelopeAttributeValue().(*types.AttributeValueMemberM)
	require.True(t, ok)
	delete(missingCiphertext.Value, "ct")
	assert.False(t, isEncryptedEnvelope(missingCiphertext))
}
