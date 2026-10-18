package encryption

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/theory-cloud/tabletheory/v6/pkg/model"
)

// RecordBinding identifies the stored row an encrypted envelope belongs to.
// Envelope v2 authenticates this binding in the AES-GCM additional data, so a
// valid envelope copied onto another record or another table fails to decrypt.
type RecordBinding struct {
	Table        string
	PartitionKey types.AttributeValue
	SortKey      types.AttributeValue
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
func aadForBoundRecord(attributeName string, binding RecordBinding) []byte {
	var builder strings.Builder
	builder.WriteString("theorydb:encrypted:v2|attr=")
	builder.WriteString(attributeName)
	builder.WriteString("|table=")
	builder.WriteString(binding.Table)
	builder.WriteString("|pk=")
	builder.WriteString(canonicalKeyValue(binding.PartitionKey))
	if binding.SortKey != nil {
		builder.WriteString("|sk=")
		builder.WriteString(canonicalKeyValue(binding.SortKey))
	}
	return []byte(builder.String())
}

// canonicalKeyValue encodes a key attribute value unambiguously. S and N keep
// their stored text; B is standard base64. The type prefix keeps a string key
// from colliding with a numeric key of the same text.
func canonicalKeyValue(av types.AttributeValue) string {
	switch value := av.(type) {
	case *types.AttributeValueMemberS:
		return "S:" + value.Value
	case *types.AttributeValueMemberN:
		return "N:" + value.Value
	case *types.AttributeValueMemberB:
		return "B:" + base64.StdEncoding.EncodeToString(value.Value)
	default:
		return ""
	}
}
