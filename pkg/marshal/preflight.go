package marshal

import (
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/theory-cloud/tabletheory/v5/pkg/model"
)

// ItemWriterPath names a TableTheory writer path whose stored DynamoDB
// representation an item preflight must reproduce.
//
// TableTheory serializes a model through two different writers, and for a nil
// collection written without `omitempty` those writers do not agree. The put
// family (Model().Create, Model().CreateOrUpdate, BatchCreate,
// BatchCreateWithResult and the puts issued by BatchWrite) serializes through
// SafeMarshaler, which stores DynamoDB NULL for a nil slice or a nil map. The
// transactional family (Transact().Put, Transact().Create) serializes through
// the transaction converter, which stores an empty list (L) or an empty map (M)
// instead.
//
// ItemWriterPath selects which of those answers MarshalItemForWriterPath
// returns. It changes nothing that is stored: neither writer's output is
// affected, and the disagreement is pinned by tests against captured writer
// payloads rather than resolved. Choosing a writer path therefore also selects
// which stored representation a caller is measuring.
type ItemWriterPath string

const (
	// ItemWriterPathPut selects the put-family writer paths: Model().Create,
	// Model().CreateOrUpdate, BatchCreate, BatchCreateWithResult and the puts
	// issued by BatchWrite.
	//
	// Those paths store the SafeMarshaler representation. In it, a nil slice or
	// nil map written without `omitempty` becomes DynamoDB NULL, a non-nil empty
	// slice becomes an empty list (L), and a non-nil empty map becomes an empty
	// map (M).
	ItemWriterPathPut ItemWriterPath = "put"

	// ItemWriterPathTransactWrite selects the transactional writer paths:
	// Transact().Put and Transact().Create.
	//
	// Those paths store the transaction converter representation. In it, a nil
	// slice written without `omitempty` becomes an empty list (L) and a nil map
	// becomes an empty map (M); a non-nil empty slice or map is stored the same
	// way.
	ItemWriterPathTransactWrite ItemWriterPath = "transact_write"
)

// MarshalItemForWriterPath returns the DynamoDB item that the named writer path
// stores for model, so a caller can measure the bytes it is about to write.
//
// The returned item is byte-equal to a payload captured from the real writer:
// ItemWriterPathPut matches a Model().Create or BatchCreate capture, and
// ItemWriterPathTransactWrite matches a Transact().Put or Transact().Create
// capture. Both are pinned by regression tests that capture live payloads
// through a fake session, so neither answer can drift from its writer without
// failing the build.
//
// metadata must be the registered metadata for model's type, the same value
// SafeMarshaler.MarshalItem takes, so both writers and the preflight read the
// same field set, names and `omitempty` flags.
//
// Scope:
//
//   - The preflight reproduces attribute serialization with TableTheory's
//     default type converter. Custom converters registered through
//     RegisterTypeConverter live on the database's converter, which is not
//     reachable from a standalone preflight; a model whose serialization
//     depends on one of those converters is not covered.
//   - Models that declare `encrypted` attributes are refused with an error. The
//     stored bytes of an encrypted attribute depend on KMS key material and
//     envelope randomness, so no static preflight can predict them.
//
// An unrecognized path is refused with an error. The preflight has no side
// effects and never contacts DynamoDB.
func MarshalItemForWriterPath(model any, metadata *model.Metadata, path ItemWriterPath) (map[string]types.AttributeValue, error) {
	return nil, fmt.Errorf("marshal: MarshalItemForWriterPath: writer path %q is not implemented", path)
}
