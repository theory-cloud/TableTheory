package schema_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v4"
	"github.com/theory-cloud/tabletheory/v4/pkg/schema"
	"github.com/theory-cloud/tabletheory/v4/pkg/session"
	"github.com/theory-cloud/tabletheory/v4/pkg/testing/fakedb"
)

type migrationPlainSource struct {
	PK     string `theorydb:"pk,attr:PK" json:"PK"`
	Secret string `theorydb:"attr:secret" json:"secret"`
}

func (migrationPlainSource) TableName() string { return "migration_plain_source" }

type migrationEncryptedTarget struct {
	PK     string `theorydb:"pk,attr:PK" json:"PK"`
	Secret string `theorydb:"attr:secret,encrypted" json:"secret"`
}

func (migrationEncryptedTarget) TableName() string { return "migration_encrypted_target" }

type migrationPlainTarget struct {
	PK     string `theorydb:"pk,attr:PK" json:"PK"`
	Secret string `theorydb:"attr:secret" json:"secret"`
}

func (migrationPlainTarget) TableName() string { return "migration_plain_target" }

type fakeMigrationKMS struct{}

func (fakeMigrationKMS) GenerateDataKey(_ context.Context, _ *kms.GenerateDataKeyInput, _ ...func(*kms.Options)) (*kms.GenerateDataKeyOutput, error) {
	return &kms.GenerateDataKeyOutput{
		Plaintext:      bytes.Repeat([]byte{7}, 32),
		CiphertextBlob: []byte("wrapped-dek"),
	}, nil
}

func (fakeMigrationKMS) Decrypt(_ context.Context, _ *kms.DecryptInput, _ ...func(*kms.Options)) (*kms.DecryptOutput, error) {
	return &kms.DecryptOutput{Plaintext: bytes.Repeat([]byte{7}, 32)}, nil
}

// S1: a plaintext source model migrating into an encrypted target model with no
// KMS configuration must be refused before any target write or table creation.
func TestAutoMigrateWithOptionsRefusesPlaintextIntoEncryptedTarget(t *testing.T) {
	fake := fakedb.New()
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, fake)
	require.NoError(t, err)

	err = db.AutoMigrateWithOptions(
		&migrationPlainSource{},
		schema.WithTargetModel(&migrationEncryptedTarget{}),
		schema.WithDataCopy(true),
	)
	require.Error(t, err)
	assert.True(t, errors.Is(err, schema.ErrMigrationEncryptionRequired), "want ErrMigrationEncryptionRequired, got %v", err)

	tables, listErr := fake.ListTables(context.Background(), &dynamodb.ListTablesInput{})
	require.NoError(t, listErr)
	assert.Empty(t, tables.TableNames, "no table may be created before the encryption preflight passes")
}

// S2: with a configured KMS provider the copy writes an authenticated envelope
// that the target model can read, and never the plaintext.
func TestAutoMigrateWithOptionsEncryptsIntoEncryptedTarget(t *testing.T) {
	fake := fakedb.New()
	db, err := tabletheory.NewWithClient(session.Config{
		Region:         "us-east-1",
		KMSKeyARN:      "arn:aws:kms:us-east-1:111111111111:key/migration",
		KMSClient:      fakeMigrationKMS{},
		EncryptionRand: bytes.NewReader(bytes.Repeat([]byte{9}, 256)),
	}, fake)
	require.NoError(t, err)

	require.NoError(t, db.AutoMigrate(&migrationPlainSource{}))
	require.NoError(t, fake.Seed("migration_plain_source", map[string]types.AttributeValue{
		"PK":     &types.AttributeValueMemberS{Value: "1"},
		"secret": &types.AttributeValueMemberS{Value: "top-secret"},
	}))

	require.NoError(t, db.AutoMigrateWithOptions(
		&migrationPlainSource{},
		schema.WithTargetModel(&migrationEncryptedTarget{}),
		schema.WithDataCopy(true),
	))

	items := fake.Items("migration_encrypted_target")
	require.Len(t, items, 1)

	envelope, ok := items[0]["secret"].(*types.AttributeValueMemberM)
	require.True(t, ok, "migrated encrypted attribute must be an envelope map, got %T", items[0]["secret"])
	version, ok := envelope.Value["v"].(*types.AttributeValueMemberN)
	require.True(t, ok, "envelope version must be N, got %T", envelope.Value["v"])
	assert.Equal(t, "1", version.Value)
	for _, field := range []string{"edk", "nonce", "ct"} {
		payload, ok := envelope.Value[field].(*types.AttributeValueMemberB)
		require.True(t, ok, "envelope field %s must be B, got %T", field, envelope.Value[field])
		assert.NotEmpty(t, payload.Value)
	}
}

// S4: a migration with no encryption semantic change keeps ordinary behavior.
func TestAutoMigrateWithOptionsWithoutEncryptionUnchanged(t *testing.T) {
	fake := fakedb.New()
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, fake)
	require.NoError(t, err)

	require.NoError(t, db.AutoMigrate(&migrationPlainSource{}))
	require.NoError(t, fake.Seed("migration_plain_source", map[string]types.AttributeValue{
		"PK":     &types.AttributeValueMemberS{Value: "1"},
		"secret": &types.AttributeValueMemberS{Value: "top-secret"},
	}))

	require.NoError(t, db.AutoMigrateWithOptions(
		&migrationPlainSource{},
		schema.WithTargetModel(&migrationPlainTarget{}),
		schema.WithDataCopy(true),
	))

	items := fake.Items("migration_plain_target")
	require.Len(t, items, 1)
	stored, ok := items[0]["secret"].(*types.AttributeValueMemberS)
	require.True(t, ok, "plaintext target attribute must stay S, got %T", items[0]["secret"])
	assert.Equal(t, "top-secret", stored.Value)
}

// A table-only migration that copies no data must not require KMS configuration.
func TestAutoMigrateWithOptionsEncryptedTargetWithoutDataCopyIsUnaffected(t *testing.T) {
	fake := fakedb.New()
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, fake)
	require.NoError(t, err)

	require.NoError(t, db.AutoMigrateWithOptions(&migrationEncryptedTarget{}))

	tables, listErr := fake.ListTables(context.Background(), &dynamodb.ListTablesInput{})
	require.NoError(t, listErr)
	assert.Equal(t, []string{"migration_encrypted_target"}, tables.TableNames)
}
