package model

// WritePolicyMode describes how high-level write APIs may mutate a model.
type WritePolicyMode string

const (
	// WritePolicyModeMutable preserves the historical TableTheory behavior:
	// create, update, upsert, and delete operations are allowed unless other
	// model metadata or call-level conditions reject them.
	WritePolicyModeMutable WritePolicyMode = "mutable"

	// WritePolicyModeWriteOnce permits initial creation but rejects generic
	// high-level mutation APIs such as update, upsert, and delete.
	WritePolicyModeWriteOnce WritePolicyMode = "write_once"
)

// WritePolicy declares opt-in model-level mutation guardrails.
//
// The zero value is equivalent to Mutable with no protected attributes.
// ProtectedAttributes are normalized by Metadata parsing to canonical
// DynamoDB attribute names.
type WritePolicy struct {
	Mode                WritePolicyMode
	ProtectedAttributes []string
}

// DefaultWritePolicy returns the default mutation policy for models that do
// not opt into write_policy metadata.
func DefaultWritePolicy() WritePolicy {
	return WritePolicy{
		Mode:                WritePolicyModeMutable,
		ProtectedAttributes: []string{},
	}
}
