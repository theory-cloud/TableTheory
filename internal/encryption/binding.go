package encryption

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	customerrors "github.com/theory-cloud/tabletheory/v6/pkg/errors"
	"github.com/theory-cloud/tabletheory/v6/pkg/model"
)

// RecordBinding identifies the stored row an encrypted envelope belongs to.
// Envelope v2 authenticates this binding in the AES-GCM additional data, so a
// valid envelope copied onto another record or another table fails to decrypt.
type RecordBinding struct {
	PartitionKey types.AttributeValue
	SortKey      types.AttributeValue
	Table        string
}

// Valid reports whether the binding carries the minimum identity needed to
// construct the v2 additional authenticated data.
func (b RecordBinding) Valid() bool {
	return strings.TrimSpace(b.Table) != "" && b.PartitionKey != nil
}

// NewRecordBinding validates and returns a record binding.
func NewRecordBinding(table string, partitionKey, sortKey types.AttributeValue) (RecordBinding, error) {
	binding := RecordBinding{
		Table:        strings.TrimSpace(table),
		PartitionKey: partitionKey,
		SortKey:      sortKey,
	}
	if !binding.Valid() {
		return RecordBinding{}, fmt.Errorf("encrypted envelope record binding requires a table name and partition key")
	}
	if _, err := canonicalKeyValue(partitionKey); err != nil {
		return RecordBinding{}, err
	}
	if sortKey != nil {
		if _, err := canonicalKeyValue(sortKey); err != nil {
			return RecordBinding{}, err
		}
	}
	return binding, nil
}

// NewRecordBindingFromKey builds a binding from a model key map. metadata and
// the key map must describe the same row; a missing partition key is an error
// so a write cannot silently produce an unbound envelope.
func NewRecordBindingFromKey(table string, metadata *model.Metadata, key map[string]types.AttributeValue) (RecordBinding, error) {
	if metadata == nil || metadata.PrimaryKey == nil || metadata.PrimaryKey.PartitionKey == nil {
		return RecordBinding{}, fmt.Errorf("encrypted envelope record binding requires model key metadata")
	}
	if len(key) == 0 {
		return RecordBinding{}, fmt.Errorf("encrypted envelope record binding requires the record key")
	}

	partitionKey := key[metadata.PrimaryKey.PartitionKey.DBName]
	if partitionKey == nil {
		return RecordBinding{}, fmt.Errorf("encrypted envelope record binding is missing partition key %s", metadata.PrimaryKey.PartitionKey.DBName)
	}

	var sortKey types.AttributeValue
	if metadata.PrimaryKey.SortKey != nil {
		sortKey = key[metadata.PrimaryKey.SortKey.DBName]
		if sortKey == nil {
			return RecordBinding{}, fmt.Errorf("encrypted envelope record binding is missing sort key %s", metadata.PrimaryKey.SortKey.DBName)
		}
	}

	return NewRecordBinding(table, partitionKey, sortKey)
}

// BindingFromItem derives a record binding from a full or projected item. It
// reports false when the item does not carry the key attributes, which is the
// case for a projection that omits them.
func BindingFromItem(table string, metadata *model.Metadata, item map[string]types.AttributeValue) (RecordBinding, bool) {
	if len(item) == 0 || metadata == nil || metadata.PrimaryKey == nil || metadata.PrimaryKey.PartitionKey == nil {
		return RecordBinding{}, false
	}

	partitionKey := item[metadata.PrimaryKey.PartitionKey.DBName]
	if partitionKey == nil {
		return RecordBinding{}, false
	}

	var sortKey types.AttributeValue
	if metadata.PrimaryKey.SortKey != nil {
		sortKey = item[metadata.PrimaryKey.SortKey.DBName]
		if sortKey == nil {
			return RecordBinding{}, false
		}
	}

	binding, err := NewRecordBinding(table, partitionKey, sortKey)
	if err != nil {
		return RecordBinding{}, false
	}
	return binding, true
}

// aadForBoundRecord is the v2 additional authenticated data. The format is a
// cross-runtime contract: Go, TypeScript, and Python must produce exactly this
// string so envelopes stay readable across runtimes.
//
// Every variable-length component is length-prefixed with its UTF-8 byte
// length (`name=<length>:<value>`). A delimiter inside a table, attribute, or
// key value therefore cannot make two distinct record identities produce the
// same AAD.
func aadForBoundRecord(attributeName string, binding RecordBinding) ([]byte, error) {
	partitionKey, err := canonicalKeyValue(binding.PartitionKey)
	if err != nil {
		return nil, err
	}

	var builder strings.Builder
	builder.WriteString("theorydb:encrypted:v2")
	writeLengthPrefixed(&builder, "|attr=", attributeName)
	writeLengthPrefixed(&builder, "|table=", binding.Table)
	writeLengthPrefixed(&builder, "|pk=", partitionKey)
	if binding.SortKey != nil {
		sortKey, err := canonicalKeyValue(binding.SortKey)
		if err != nil {
			return nil, err
		}
		writeLengthPrefixed(&builder, "|sk=", sortKey)
	}
	return []byte(builder.String()), nil
}

func writeLengthPrefixed(builder *strings.Builder, prefix, value string) {
	builder.WriteString(prefix)
	builder.WriteString(strconv.Itoa(len(value)))
	builder.WriteString(":")
	builder.WriteString(value)
}

// canonicalKeyValue encodes a key attribute value unambiguously. S and N keep
// their stored text; B is standard base64. The type prefix keeps a string key
// from colliding with a numeric key of the same text. Unsupported key types
// fail closed instead of canonicalizing to an empty value.
func canonicalKeyValue(av types.AttributeValue) (string, error) {
	switch value := av.(type) {
	case *types.AttributeValueMemberS:
		return "S:" + value.Value, nil
	case *types.AttributeValueMemberN:
		return "N:" + value.Value, nil
	case *types.AttributeValueMemberB:
		return "B:" + base64.StdEncoding.EncodeToString(value.Value), nil
	default:
		return "", fmt.Errorf("%w: unsupported record binding key type %T", customerrors.ErrInvalidEncryptedEnvelope, av)
	}
}
