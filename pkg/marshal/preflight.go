package marshal

import (
	"fmt"
	"reflect"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/theory-cloud/tabletheory/v6/internal/encryption"
	"github.com/theory-cloud/tabletheory/v6/internal/reflectutil"
	customerrors "github.com/theory-cloud/tabletheory/v6/pkg/errors"
	"github.com/theory-cloud/tabletheory/v6/pkg/model"
	pkgTypes "github.com/theory-cloud/tabletheory/v6/pkg/types"
)

// ItemWriterPath names a TableTheory writer path whose stored DynamoDB
// representation an item preflight must reproduce.
//
// TableTheory serializes a model through two different writers, and for a nil
// collection written without `omitempty` those writers store different
// representations. The put family (Model().Create, Model().CreateOrUpdate,
// BatchCreate, BatchCreateWithResult and the puts issued by BatchWrite)
// serializes through SafeMarshaler, which stores DynamoDB NULL for a nil slice or
// a nil map. The transactional family (Transact().Put, Transact().Create)
// serializes through the transaction converter, which stores an empty list (L)
// or an empty map (M) instead.
//
// Each writer path's stored representation is a permanent compatibility
// contract: it is frozen by the operator's policy that TableTheory supports its
// consumers rather than changing stored data, so no writer's output may change.
// ItemWriterPath selects which frozen representation MarshalItemForWriterPath
// returns, so a caller measures exactly what the named writer stores. Neither
// writer is edited by this preflight.
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

// PreflightOptions carries the writer configuration an item preflight cannot
// infer from the model alone.
type PreflightOptions struct {
	// Now is the clock the named writer path reads when it stamps lifecycle
	// timestamps. Pass the same clock the writer is configured with (for
	// example `session.Config.Now`).
	//
	// A model that declares `createdAt` or `updatedAt` cannot be preflighted on
	// the put path without it: that writer stamps a time the preflight cannot
	// predict, so no byte-equality claim is supportable and the preflight fails
	// closed instead of returning a payload that will not match. The
	// transactional path in this runtime never stamps lifecycle fields, so it
	// needs no clock.
	Now func() time.Time
}

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
//   - A model that declares `createdAt` or `updatedAt` is refused on the put
//     path with an error, because that writer stamps a time this call cannot
//     predict. Use MarshalItemForWriterPathWithOptions with the writer's clock
//     to make the claim truthful.
//
// An unrecognized path is refused with an error. The preflight has no side
// effects and never contacts DynamoDB.
func MarshalItemForWriterPath(model any, metadata *model.Metadata, path ItemWriterPath) (map[string]types.AttributeValue, error) {
	return MarshalItemForWriterPathWithOptions(model, metadata, path, PreflightOptions{})
}

// MarshalItemForWriterPathWithOptions is MarshalItemForWriterPath with the
// writer configuration the preflight cannot infer.
//
// The byte-equality claim above only holds when a lifecycle-stamped writer and
// this preflight read the same clock, so the caller supplies it. When a model
// declares lifecycle fields and the put path is requested without a clock, the
// claim is unsupported and the preflight fails closed with
// customerrors.ErrUnsupportedType instead of returning a payload that will not
// match what the writer stores.
func MarshalItemForWriterPathWithOptions(
	model any,
	metadata *model.Metadata,
	path ItemWriterPath,
	opts PreflightOptions,
) (map[string]types.AttributeValue, error) {
	if metadata == nil {
		return nil, fmt.Errorf("%w: metadata is required for an item preflight", customerrors.ErrInvalidModel)
	}
	if encryption.MetadataHasEncryptedFields(metadata) {
		return nil, fmt.Errorf(
			"%w: item preflight does not cover the encrypted attributes of %s",
			customerrors.ErrUnsupportedType, metadata.TableName,
		)
	}

	switch path {
	case ItemWriterPathPut:
		if metadataHasLifecycleFields(metadata) && opts.Now == nil {
			return nil, fmt.Errorf(
				"%w: item preflight for %s needs the writer's lifecycle clock",
				customerrors.ErrUnsupportedType, metadata.TableName,
			)
		}
		// The put family marshals through a SafeMarshaler built the same way
		// internal/theorydb builds the default marshaler, so this branch is the
		// writer's own serializer rather than a copy of its rule.
		return NewSafeMarshalerWithConverterAndClock(pkgTypes.NewConverter(), opts.Now).
			MarshalItem(model, metadata)

	case ItemWriterPathTransactWrite:
		return marshalTransactWritePlainItem(model, metadata, pkgTypes.NewConverter())

	default:
		return nil, fmt.Errorf("%w: unknown item writer path %q", customerrors.ErrInvalidModel, path)
	}
}

// metadataHasLifecycleFields reports whether the writer stamps a timestamp the
// caller cannot supply itself.
func metadataHasLifecycleFields(metadata *model.Metadata) bool {
	for _, field := range metadata.Fields {
		if field.IsCreatedAt || field.IsUpdatedAt {
			return true
		}
	}
	return false
}

// marshalTransactWritePlainItem reproduces the item a transactional writer path
// stores, before any encryption step.
//
// It mirrors the converter rule in pkg/transaction.(*Transaction).marshalPlainItem:
// a field tagged omitempty is dropped when empty, and every remaining field is
// converted with the transaction converter, which stores a nil slice as an empty
// list and a nil map as an empty map. The rule is mirrored rather than shared so
// that no writer path is edited by this change; the captured-payload test in
// preflight_test.go fails if this mirror and the writer drift apart.
func marshalTransactWritePlainItem(model any, metadata *model.Metadata, converter *pkgTypes.Converter) (map[string]types.AttributeValue, error) {
	modelValue, err := derefStructValue(model)
	if err != nil {
		return nil, err
	}

	item := make(map[string]types.AttributeValue, len(metadata.Fields))
	for fieldName, fieldMeta := range metadata.Fields {
		fieldValue, err := preflightFieldByIndexPath(modelValue, fieldMeta.IndexPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read field %s: %w", fieldName, err)
		}

		if fieldMeta.OmitEmpty && reflectutil.IsEmpty(fieldValue) {
			continue
		}

		av, err := converter.ToAttributeValue(fieldValue.Interface())
		if err != nil {
			return nil, fmt.Errorf("failed to convert field %s: %w", fieldName, err)
		}
		if av == nil {
			continue
		}

		item[fieldMeta.DBName] = av
	}

	return item, nil
}

// preflightFieldByIndexPath resolves a metadata field index path, reporting a
// shape disagreement as an error instead of letting reflect panic.
func preflightFieldByIndexPath(modelValue reflect.Value, indexPath []int) (field reflect.Value, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			field = reflect.Value{}
			err = fmt.Errorf("model field path %v does not resolve on %s", indexPath, modelValue.Type())
		}
	}()

	if len(indexPath) == 0 {
		return reflect.Value{}, fmt.Errorf("model field path %v does not resolve on %s", indexPath, modelValue.Type())
	}

	return modelValue.FieldByIndex(indexPath), nil
}
