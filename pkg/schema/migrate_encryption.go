package schema

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/theory-cloud/tabletheory/v4/internal/encryption"
	"github.com/theory-cloud/tabletheory/v4/pkg/model"
)

// ErrMigrationEncryptionRequired reports that a data-copying schema migration
// cannot satisfy the target model's encryption contract. It is always returned
// before any target row is written.
var ErrMigrationEncryptionRequired = errors.New("schema migration encryption required")

// attributeEncryptor is the envelope-encryption surface the migration path needs.
type attributeEncryptor interface {
	EncryptAttributeValue(ctx context.Context, attributeName string, av types.AttributeValue) (types.AttributeValue, error)
}

// migrationEncryptionPlan records the per-attribute encryption decisions for a
// data-copying migration so each copied item is checked (and encrypted) before
// it is handed to a batch write.
type migrationEncryptionPlan struct {
	encryptor       attributeEncryptor
	sourceEncrypted map[string]struct{}
	targetEncrypted map[string]struct{}
	targetName      string
}

// migrationEncryptor builds the envelope encryption service from the session
// configuration, or returns nil when the session has no KMS key configured.
func (m *Manager) migrationEncryptor() attributeEncryptor {
	if m == nil || m.session == nil {
		return nil
	}
	cfg := m.session.Config()
	if cfg == nil || cfg.KMSKeyARN == "" {
		return nil
	}
	if cfg.KMSClient != nil {
		return encryption.NewServiceWithRand(cfg.KMSKeyARN, cfg.KMSClient, cfg.EncryptionRand)
	}
	return encryption.NewServiceFromAWSConfigWithRand(cfg.KMSKeyARN, m.session.AWSConfig(), cfg.EncryptionRand)
}

func encryptedAttributeNames(metadata *model.Metadata) map[string]struct{} {
	names := make(map[string]struct{})
	if metadata == nil {
		return names
	}
	for _, field := range metadata.Fields {
		if field == nil || !field.IsEncrypted {
			continue
		}
		name := field.DBName
		if name == "" {
			name = field.Name
		}
		names[name] = struct{}{}
	}
	return names
}

// absentFrom returns the sorted names present in values but absent from other.
func absentFrom(values, other map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for name := range values {
		if _, ok := other[name]; !ok {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func sortedNames(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for name := range values {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// preflightMigrationEncryption rejects, before any table is created or copied,
// a data migration that would write plaintext into a target encrypted field.
func preflightMigrationEncryption(sourceMetadata, targetMetadata *model.Metadata, configured bool, targetName string) error {
	targetEncrypted := encryptedAttributeNames(targetMetadata)
	if len(targetEncrypted) == 0 {
		return nil
	}
	needs := absentFrom(targetEncrypted, encryptedAttributeNames(sourceMetadata))
	if len(needs) == 0 || configured {
		return nil
	}
	return fmt.Errorf(
		"%w: migration target model %q requires encryption for attribute(s) %s; configure session.Config.KMSKeyARN",
		ErrMigrationEncryptionRequired, targetName, strings.Join(needs, ", "),
	)
}

func newMigrationEncryptionPlan(sourceMetadata, targetMetadata *model.Metadata, encryptor attributeEncryptor, targetName string) *migrationEncryptionPlan {
	return &migrationEncryptionPlan{
		encryptor:       encryptor,
		sourceEncrypted: encryptedAttributeNames(sourceMetadata),
		targetEncrypted: encryptedAttributeNames(targetMetadata),
		targetName:      targetName,
	}
}

// guardItem validates (and where required encrypts) one copied item. It returns
// an error rather than writing anything when the item cannot satisfy the
// target's encryption contract.
func (p *migrationEncryptionPlan) guardItem(ctx context.Context, item map[string]types.AttributeValue) error {
	if p == nil || (len(p.sourceEncrypted) == 0 && len(p.targetEncrypted) == 0) {
		return nil
	}

	for _, name := range sortedNames(p.sourceEncrypted) {
		av, ok := item[name]
		if !ok {
			continue
		}
		if !isEncryptedEnvelope(av) {
			return fmt.Errorf(
				"%w: migration attribute %q is declared encrypted in the source model but the copied value is not an encrypted envelope",
				ErrMigrationEncryptionRequired, name,
			)
		}
	}

	for _, name := range absentFrom(p.sourceEncrypted, p.targetEncrypted) {
		if av, ok := item[name]; ok && isEncryptedEnvelope(av) {
			return fmt.Errorf(
				"%w: migration cannot copy encrypted attribute %q into a target attribute that is not encrypted",
				ErrMigrationEncryptionRequired, name,
			)
		}
	}

	for _, name := range absentFrom(p.targetEncrypted, p.sourceEncrypted) {
		av, ok := item[name]
		if !ok {
			continue
		}
		if isEncryptedEnvelope(av) {
			return fmt.Errorf(
				"%w: migration cannot re-encrypt attribute %q: the value is already an encrypted envelope under a different attribute name",
				ErrMigrationEncryptionRequired, name,
			)
		}
		if p.encryptor == nil {
			return fmt.Errorf(
				"%w: migration target model %q requires encryption for attribute(s) %s; configure session.Config.KMSKeyARN",
				ErrMigrationEncryptionRequired, p.targetName, name,
			)
		}
		encrypted, err := p.encryptor.EncryptAttributeValue(ctx, name, av)
		if err != nil {
			return fmt.Errorf("failed to encrypt migration attribute %s: %w", name, err)
		}
		item[name] = encrypted
	}

	return nil
}

// isEncryptedEnvelope reports whether av is a theorydb v1 encrypted envelope.
func isEncryptedEnvelope(av types.AttributeValue) bool {
	envelope, ok := av.(*types.AttributeValueMemberM)
	if !ok || envelope == nil {
		return false
	}
	version, ok := envelope.Value["v"].(*types.AttributeValueMemberN)
	if !ok || version == nil || version.Value != "1" {
		return false
	}
	for _, key := range []string{"edk", "nonce", "ct"} {
		binary, ok := envelope.Value[key].(*types.AttributeValueMemberB)
		if !ok || binary == nil || len(binary.Value) == 0 {
			return false
		}
	}
	return true
}
