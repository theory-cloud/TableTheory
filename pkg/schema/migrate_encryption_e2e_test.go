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

	"github.com/theory-cloud/tabletheory/v6"
	"github.com/theory-cloud/tabletheory/v6/internal/encryption"
	theorydbErrors "github.com/theory-cloud/tabletheory/v6/pkg/errors"
	"github.com/theory-cloud/tabletheory/v6/pkg/schema"
	"github.com/theory-cloud/tabletheory/v6/pkg/session"
	"github.com/theory-cloud/tabletheory/v6/pkg/testing/fakedb"
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
	assert.Equal(t, "2", version.Value, "migration writes record-bound envelopes")
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

type migrationSharedSource struct {
	PK     string `theorydb:"pk,attr:PK" json:"PK"`
	SK     string `theorydb:"sk,attr:SK" json:"SK"`
	Secret string `theorydb:"attr:secret,encrypted" json:"secret"`
}

func (migrationSharedSource) TableName() string { return "migration_shared_source_e2e" }

type migrationSharedTarget struct {
	PK     string `theorydb:"pk,attr:PK" json:"PK"`
	SK     string `theorydb:"sk,attr:SK" json:"SK"`
	Secret string `theorydb:"attr:secret,encrypted" json:"secret"`
}

func (migrationSharedTarget) TableName() string { return "migration_shared_target_e2e" }

// S5: same-name encrypted attributes move across tables only after the source
// envelope is opened with the source binding and re-sealed with the target
// binding. Legacy v1 rows are upgraded to v2, and the target model can read
// every migrated row.
func TestAutoMigrateWithOptionsRebindsSharedEncryptedAttributes(t *testing.T) {
	fake := fakedb.New()
	keyARN := "arn:aws:kms:us-east-1:111111111111:key/migration-shared"
	db, err := tabletheory.NewWithClient(session.Config{
		Region:         "us-east-1",
		KMSKeyARN:      keyARN,
		KMSClient:      fakeMigrationKMS{},
		EncryptionRand: bytes.NewReader(bytes.Repeat([]byte{0x0a}, 512)),
	}, fake)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&migrationSharedSource{}))

	svc := encryption.NewService(keyARN, fakeMigrationKMS{})
	ctx := context.Background()

	v1Envelope, err := svc.EncryptAttributeValue(ctx, "secret", &types.AttributeValueMemberS{Value: "v1-secret"})
	require.NoError(t, err)

	sourceBinding, err := encryption.NewRecordBinding(
		"migration_shared_source_e2e",
		&types.AttributeValueMemberS{Value: "V2"},
		&types.AttributeValueMemberS{Value: "PROFILE"},
	)
	require.NoError(t, err)
	v2Envelope, err := svc.EncryptAttributeValueWithBinding(ctx, sourceBinding, "secret", &types.AttributeValueMemberS{Value: "v2-secret"})
	require.NoError(t, err)

	require.NoError(t, fake.Seed("migration_shared_source_e2e",
		map[string]types.AttributeValue{
			"PK":     &types.AttributeValueMemberS{Value: "V1"},
			"SK":     &types.AttributeValueMemberS{Value: "PROFILE"},
			"secret": v1Envelope,
		},
		map[string]types.AttributeValue{
			"PK":     &types.AttributeValueMemberS{Value: "V2"},
			"SK":     &types.AttributeValueMemberS{Value: "PROFILE"},
			"secret": v2Envelope,
		},
	))

	require.NoError(t, db.AutoMigrateWithOptions(
		&migrationSharedSource{},
		schema.WithTargetModel(&migrationSharedTarget{}),
		schema.WithDataCopy(true),
		schema.WithBatchSize(1),
	))

	for key, want := range map[string]string{"V1": "v1-secret", "V2": "v2-secret"} {
		var got migrationSharedTarget
		require.NoError(t, db.Model(&migrationSharedTarget{}).
			Where("PK", "=", key).
			Where("SK", "=", "PROFILE").
			First(&got))
		assert.Equal(t, want, got.Secret)
	}

	items := fake.Items("migration_shared_target_e2e")
	require.Len(t, items, 2)
	for _, item := range items {
		envelope, ok := item["secret"].(*types.AttributeValueMemberM)
		require.True(t, ok, "target rows only carry envelopes, got %T", item["secret"])
		version, ok := envelope.Value["v"].(*types.AttributeValueMemberN)
		require.True(t, ok)
		assert.Equal(t, "2", version.Value, "migrated rows are target-bound v2 envelopes")
	}
}

// S6: a source page that cannot be opened fails before that page is written.
func TestAutoMigrateWithOptionsWritesNoPageOnSourceDecryptFailure(t *testing.T) {
	fake := fakedb.New()
	keyARN := "arn:aws:kms:us-east-1:111111111111:key/migration-shared"
	db, err := tabletheory.NewWithClient(session.Config{
		Region:         "us-east-1",
		KMSKeyARN:      keyARN,
		KMSClient:      fakeMigrationKMS{},
		EncryptionRand: bytes.NewReader(bytes.Repeat([]byte{0x0b}, 512)),
	}, fake)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&migrationSharedSource{}))

	svc := encryption.NewService(keyARN, fakeMigrationKMS{})
	tampered, err := svc.EncryptAttributeValue(context.Background(), "secret", &types.AttributeValueMemberS{Value: "tampered"})
	require.NoError(t, err)
	envelope, ok := tampered.(*types.AttributeValueMemberM)
	require.True(t, ok)
	ciphertext, ok := envelope.Value["ct"].(*types.AttributeValueMemberB)
	require.True(t, ok)
	ciphertext.Value[len(ciphertext.Value)-1] ^= 0xff

	require.NoError(t, fake.Seed("migration_shared_source_e2e", map[string]types.AttributeValue{
		"PK":     &types.AttributeValueMemberS{Value: "BROKEN"},
		"SK":     &types.AttributeValueMemberS{Value: "PROFILE"},
		"secret": tampered,
	}))

	err = db.AutoMigrateWithOptions(
		&migrationSharedSource{},
		schema.WithTargetModel(&migrationSharedTarget{}),
		schema.WithDataCopy(true),
		schema.WithBatchSize(1),
	)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidEncryptedEnvelope)
	assert.Empty(t, fake.Items("migration_shared_target_e2e"), "the failing page must not be written")
}

// S7: without a provider the migration refuses before creating the target.
func TestAutoMigrateWithOptionsRefusesSharedEncryptionWithoutProvider(t *testing.T) {
	fake := fakedb.New()
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, fake)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&migrationSharedSource{}))

	svc := encryption.NewService("arn:aws:kms:us-east-1:111111111111:key/unused", fakeMigrationKMS{})
	envelope, err := svc.EncryptAttributeValue(context.Background(), "secret", &types.AttributeValueMemberS{Value: "top-secret"})
	require.NoError(t, err)
	require.NoError(t, fake.Seed("migration_shared_source_e2e", map[string]types.AttributeValue{
		"PK":     &types.AttributeValueMemberS{Value: "NOKMS"},
		"SK":     &types.AttributeValueMemberS{Value: "PROFILE"},
		"secret": envelope,
	}))

	err = db.AutoMigrateWithOptions(
		&migrationSharedSource{},
		schema.WithTargetModel(&migrationSharedTarget{}),
		schema.WithDataCopy(true),
	)
	require.ErrorIs(t, err, schema.ErrMigrationEncryptionRequired)
	assert.Empty(t, fake.Items("migration_shared_target_e2e"))
	tables, listErr := fake.ListTables(context.Background(), &dynamodb.ListTablesInput{})
	require.NoError(t, listErr)
	assert.NotContains(t, tables.TableNames, "migration_shared_target_e2e")
}
