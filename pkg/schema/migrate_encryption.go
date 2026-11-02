package schema

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/theory-cloud/tabletheory/v6/internal/encryption"
	"github.com/theory-cloud/tabletheory/v6/pkg/model"
)

// ErrMigrationEncryptionRequired reports that a data-copying schema migration
// cannot satisfy the target model's encryption contract. It is always returned
// before any target row is written.
var ErrMigrationEncryptionRequired = errors.New("schema migration encryption required")

// attributeEncryptor is the envelope-encryption surface the migration path needs.
type attributeEncryptor interface {
	EncryptAttributeValue(ctx context.Context, attributeName string, av types.AttributeValue) (types.AttributeValue, error)
	EncryptAttributeValueWithBinding(ctx context.Context, binding encryption.RecordBinding, attributeName string, av types.AttributeValue) (types.AttributeValue, error)
}

// attributeDecryptor decrypts an envelope with its source record binding. A
// same-name encrypted attribute cannot be copied verbatim: the target write
// must authenticate the target record, so the migration has to open the source
// envelope first.
type attributeDecryptor interface {
	DecryptAttributeValueWithBinding(ctx context.Context, binding encryption.RecordBinding, attributeName string, envelope types.AttributeValue) (types.AttributeValue, error)
}

type migrationEncryptionService interface {
	attributeEncryptor
	attributeDecryptor
}

// migrationEncryptionPlan records the per-attribute encryption decisions for a
// data-copying migration so each copied item is checked (and encrypted) before
// it is handed to a batch write.
type migrationEncryptionPlan struct {
	encryptor       attributeEncryptor
	decryptor       attributeDecryptor
	sourceEncrypted map[string]struct{}
	targetEncrypted map[string]struct{}
	sourceMetadata  *model.Metadata
	targetMetadata  *model.Metadata
	sourceName      string
	targetName      string
}

// migrationEncryptor builds the envelope encryption service from the session
// configuration, or returns nil when the session has no KMS key configured.
func (m *Manager) migrationEncryptor() migrationEncryptionService {
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
// a data migration that cannot satisfy the target's encryption contract. A
// same-name encrypted attribute must be opened with the source binding and
// re-sealed with the target binding, so crossing tables requires a provider
// even when both models declare the attribute as encrypted.
func preflightMigrationEncryption(_, targetMetadata *model.Metadata, configured bool, targetName string) error {
	targetEncrypted := encryptedAttributeNames(targetMetadata)
	if len(targetEncrypted) == 0 || configured {
		return nil
	}
	return fmt.Errorf(
		"%w: migration target model %q requires encryption for attribute(s) %s; configure session.Config.KMSKeyARN",
		ErrMigrationEncryptionRequired, targetName, strings.Join(sortedNames(targetEncrypted), ", "),
	)
}

func newMigrationEncryptionPlan(sourceMetadata, targetMetadata *model.Metadata, encryptor attributeEncryptor, sourceName, targetName string) *migrationEncryptionPlan {
	var decryptor attributeDecryptor
	if service, ok := encryptor.(attributeDecryptor); ok {
		decryptor = service
	}
	return &migrationEncryptionPlan{
		encryptor:       encryptor,
		decryptor:       decryptor,
		sourceEncrypted: encryptedAttributeNames(sourceMetadata),
		targetEncrypted: encryptedAttributeNames(targetMetadata),
		sourceMetadata:  sourceMetadata,
		targetMetadata:  targetMetadata,
		sourceName:      sourceName,
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

	for _, name := range sortedNames(p.targetEncrypted) {
		if _, shared := p.sourceEncrypted[name]; !shared {
			continue
		}
		if _, ok := item[name]; !ok {
			continue
		}
		if err := p.rebindAttribute(ctx, name, item); err != nil {
			return err
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

	return p.encryptMissingTargetAttributes(ctx, item)
}

// rebindAttribute opens a same-name encrypted attribute with the source
// record binding and re-seals it with the target binding. A v1 source envelope
// is upgraded to a target-bound v2 envelope.
func (p *migrationEncryptionPlan) rebindAttribute(ctx context.Context, name string, item map[string]types.AttributeValue) error {
	if p.decryptor == nil || p.encryptor == nil {
		return fmt.Errorf(
			"%w: migration target model %q requires encryption to rebind attribute %q; configure session.Config.KMSKeyARN",
			ErrMigrationEncryptionRequired, p.targetName, name,
		)
	}

	sourceBinding, err := encryption.NewRecordBindingFromKey(p.sourceName, p.sourceMetadata, item)
	if err != nil {
		return fmt.Errorf(
			"%w: migration cannot bind encrypted attribute %q to the source record: %v",
			ErrMigrationEncryptionRequired, name, err,
		)
	}
	plaintext, err := p.decryptor.DecryptAttributeValueWithBinding(ctx, sourceBinding, name, item[name])
	if err != nil {
		return fmt.Errorf("failed to decrypt migration attribute %s: %w", name, err)
	}

	targetBinding, err := encryption.NewRecordBindingFromKey(p.targetName, p.targetMetadata, item)
	if err != nil {
		return fmt.Errorf(
			"%w: migration cannot bind encrypted attribute %q to the target record: %v",
			ErrMigrationEncryptionRequired, name, err,
		)
	}
	encrypted, err := p.encryptor.EncryptAttributeValueWithBinding(ctx, targetBinding, name, plaintext)
	if err != nil {
		return fmt.Errorf("failed to encrypt migration attribute %s: %w", name, err)
	}
	item[name] = encrypted
	return nil
}

// encryptMissingTargetAttributes encrypts plaintext attributes that become
// encrypted under the target model, binding each new envelope to the target
// record identity.
func (p *migrationEncryptionPlan) encryptMissingTargetAttributes(ctx context.Context, item map[string]types.AttributeValue) error {
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
		binding, err := encryption.NewRecordBindingFromKey(p.targetName, p.targetMetadata, item)
		if err != nil {
			return fmt.Errorf(
				"%w: migration cannot bind encrypted attribute %q to the target record: %v",
				ErrMigrationEncryptionRequired, name, err,
			)
		}
		encrypted, err := p.encryptor.EncryptAttributeValueWithBinding(ctx, binding, name, av)
		if err != nil {
			return fmt.Errorf("failed to encrypt migration attribute %s: %w", name, err)
		}
		item[name] = encrypted
	}

	return nil
}

// isEncryptedEnvelope reports whether av is a theorydb v1 or v2 encrypted envelope.
func isEncryptedEnvelope(av types.AttributeValue) bool {
	envelope, ok := av.(*types.AttributeValueMemberM)
	if !ok || envelope == nil {
		return false
	}
	version, ok := envelope.Value["v"].(*types.AttributeValueMemberN)
	if !ok || version == nil || (version.Value != "1" && version.Value != "2") {
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
