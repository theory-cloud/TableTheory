package marshal_test

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tabletheory "github.com/theory-cloud/tabletheory/v5"
	"github.com/theory-cloud/tabletheory/v5/pkg/core"
	customerrors "github.com/theory-cloud/tabletheory/v5/pkg/errors"
	"github.com/theory-cloud/tabletheory/v5/pkg/marshal"
	"github.com/theory-cloud/tabletheory/v5/pkg/model"
	"github.com/theory-cloud/tabletheory/v5/pkg/session"
	"github.com/theory-cloud/tabletheory/v5/pkg/testing/fakedb"
	pkgtypes "github.com/theory-cloud/tabletheory/v5/pkg/types"
)

// preflightNested is the nested container used to cover collection semantics
// below the root of the item.
type preflightNested struct {
	NestedNilMap    map[string]string `theorydb:"attr:nestedNilMap" json:"nestedNilMap"`
	NestedEmptyMap  map[string]string `theorydb:"attr:nestedEmptyMap" json:"nestedEmptyMap"`
	NestedNilList   []string          `theorydb:"attr:nestedNilList" json:"nestedNilList"`
	NestedEmptyList []string          `theorydb:"attr:nestedEmptyList" json:"nestedEmptyList"`
}

// preflightMatrix deliberately declares no lifecycle fields, so both writers
// produce a deterministic payload that can be compared byte for byte.
type preflightMatrix struct {
	PK string `theorydb:"pk,attr:PK" json:"PK"`
	SK string `theorydb:"sk,attr:SK" json:"SK"`

	NilList       []string          `theorydb:"attr:nilList" json:"nilList"`
	EmptyList     []string          `theorydb:"attr:emptyList" json:"emptyList"`
	NilMap        map[string]string `theorydb:"attr:nilMap" json:"nilMap"`
	EmptyMap      map[string]string `theorydb:"attr:emptyMap" json:"emptyMap"`
	NilListOmit   []string          `theorydb:"attr:nilListOmit,omitempty" json:"nilListOmit,omitempty"`
	EmptyListOmit []string          `theorydb:"attr:emptyListOmit,omitempty" json:"emptyListOmit,omitempty"`
	NilPtr        *string           `theorydb:"attr:nilPtr" json:"nilPtr"`
	NilAny        any               `theorydb:"attr:nilAny" json:"nilAny"`
	Nested        preflightNested   `theorydb:"attr:nested" json:"nested"`
	ZeroInt       int               `theorydb:"attr:zeroInt" json:"zeroInt"`
	FalseBool     bool              `theorydb:"attr:falseBool" json:"falseBool"`
}

func (preflightMatrix) TableName() string { return "preflight_matrix" }

type preflightEncrypted struct {
	PK     string `theorydb:"pk,attr:PK" json:"PK"`
	Secret string `theorydb:"attr:secret,encrypted" json:"secret"`
}

func (preflightEncrypted) TableName() string { return "preflight_encrypted" }

func newPreflightMatrix() *preflightMatrix {
	return &preflightMatrix{
		PK:            "p1",
		SK:            "s1",
		EmptyList:     []string{},
		EmptyMap:      map[string]string{},
		EmptyListOmit: []string{},
		Nested: preflightNested{
			NestedEmptyList: []string{},
			NestedEmptyMap:  map[string]string{},
		},
	}
}

func preflightMetadata(t *testing.T, m any) *model.Metadata {
	t.Helper()
	registry := model.NewRegistry()
	require.NoError(t, registry.Register(m))
	metadata, err := registry.GetMetadata(m)
	require.NoError(t, err)
	return metadata
}

func attrType(av types.AttributeValue) string {
	switch av.(type) {
	case *types.AttributeValueMemberNULL:
		return "NULL"
	case *types.AttributeValueMemberL:
		return "L"
	case *types.AttributeValueMemberM:
		return "M"
	case *types.AttributeValueMemberS:
		return "S"
	case *types.AttributeValueMemberN:
		return "N"
	case *types.AttributeValueMemberBOOL:
		return "BOOL"
	case nil:
		return "ABSENT"
	default:
		return "OTHER"
	}
}

func itemAttrType(item map[string]types.AttributeValue, name string) string {
	av, ok := item[name]
	if !ok {
		return "ABSENT"
	}
	return attrType(av)
}

func nestedAttrType(item map[string]types.AttributeValue, container string, name string) string {
	containerAV, ok := item[container].(*types.AttributeValueMemberM)
	if !ok {
		return "ABSENT"
	}
	av, ok := containerAV.Value[name]
	if !ok {
		return "ABSENT"
	}
	return attrType(av)
}

// capturePut records what Model().Create really sends by reading back the item
// the stateful fake stored.
func capturePut(t *testing.T, m any) map[string]types.AttributeValue {
	t.Helper()
	fake := fakedb.New()
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, fake)
	require.NoError(t, err)
	require.NoError(t, db.Model(m).Create())

	items := fake.Items("preflight_matrix")
	require.Len(t, items, 1, "the fake must have stored exactly one put item")
	return items[0]
}

// captureTransactWritePut records what TransactWrite with a real transaction
// builder really sends.
func captureTransactWritePut(t *testing.T, m any) map[string]types.AttributeValue {
	t.Helper()
	fake := fakedb.New()
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, fake)
	require.NoError(t, err)
	require.NoError(t, db.TransactWrite(context.Background(), func(tx core.TransactionBuilder) error {
		tx.Create(m)
		return nil
	}))

	items := fake.Items("preflight_matrix")
	require.Len(t, items, 1, "the fake must have stored exactly one transacted item")
	return items[0]
}

// TestMarshalItemForWriterPathByteEqualsCapturedWriterPayloads is the pin the
// issue asks for: each answer must byte-equal a payload captured from the real
// writer path it claims to predict.
func TestMarshalItemForWriterPathByteEqualsCapturedWriterPayloads(t *testing.T) {
	metadata := preflightMetadata(t, newPreflightMatrix())

	capturedPut := capturePut(t, newPreflightMatrix())
	gotPut, err := marshal.MarshalItemForWriterPath(newPreflightMatrix(), metadata, marshal.ItemWriterPathPut)
	require.NoError(t, err)
	assert.Equal(t, capturedPut, gotPut, "put preflight must byte-equal the captured Model().Create payload")

	capturedTransact := captureTransactWritePut(t, newPreflightMatrix())
	gotTransact, err := marshal.MarshalItemForWriterPath(
		newPreflightMatrix(), metadata, marshal.ItemWriterPathTransactWrite,
	)
	require.NoError(t, err)
	assert.Equal(t, capturedTransact, gotTransact, "transact_write preflight must byte-equal the captured TransactWrite payload")

	// The disagreement itself is pinned, so neither writer drifting silently
	// hides it and neither side of the fix can quietly swap the other.
	assert.Equal(t, "NULL", itemAttrType(capturedPut, "nilList"), "Put stores NULL for a nil list")
	assert.Equal(t, "L", itemAttrType(capturedTransact, "nilList"), "TransactWrite stores an empty L for a nil list")
	assert.Equal(t, "NULL", itemAttrType(capturedPut, "nilMap"), "Put stores NULL for a nil map")
	assert.Equal(t, "M", itemAttrType(capturedTransact, "nilMap"), "TransactWrite stores an empty M for a nil map")
}

// TestMarshalItemForWriterPathNilEmptyMatrix pins the full semantics per writer
// path for root and nested collections, omitempty, and pointer/interface
// wrappers.
func TestMarshalItemForWriterPathNilEmptyMatrix(t *testing.T) {
	metadata := preflightMetadata(t, newPreflightMatrix())

	putItem, err := marshal.MarshalItemForWriterPath(newPreflightMatrix(), metadata, marshal.ItemWriterPathPut)
	require.NoError(t, err)
	transactItem, err := marshal.MarshalItemForWriterPath(
		newPreflightMatrix(), metadata, marshal.ItemWriterPathTransactWrite,
	)
	require.NoError(t, err)

	root := func(name string) func(map[string]types.AttributeValue) string {
		return func(item map[string]types.AttributeValue) string { return itemAttrType(item, name) }
	}
	nested := func(name string) func(map[string]types.AttributeValue) string {
		return func(item map[string]types.AttributeValue) string {
			return nestedAttrType(item, "nested", name)
		}
	}

	cases := []struct {
		name          string
		read          func(map[string]types.AttributeValue) string
		put           string
		transactWrite string
	}{
		{"root nil list without omitempty", root("nilList"), "NULL", "L"},
		{"root empty list without omitempty", root("emptyList"), "L", "L"},
		{"root nil map without omitempty", root("nilMap"), "NULL", "M"},
		{"root empty map without omitempty", root("emptyMap"), "M", "M"},
		{"root nil list with omitempty", root("nilListOmit"), "ABSENT", "ABSENT"},
		{"root empty list with omitempty", root("emptyListOmit"), "ABSENT", "ABSENT"},
		{"root nil pointer", root("nilPtr"), "NULL", "NULL"},
		{"root nil interface", root("nilAny"), "NULL", "NULL"},
		{"root false bool without omitempty", root("falseBool"), "BOOL", "BOOL"},
		{"root zero int without omitempty", root("zeroInt"), "N", "N"},
		{"nested nil list", nested("nestedNilList"), "NULL", "L"},
		{"nested empty list", nested("nestedEmptyList"), "L", "L"},
		{"nested nil map", nested("nestedNilMap"), "NULL", "M"},
		{"nested empty map", nested("nestedEmptyMap"), "M", "M"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.put, tc.read(putItem), "put path")
			assert.Equal(t, tc.transactWrite, tc.read(transactItem), "transact_write path")
		})
	}
}

func TestMarshalItemForWriterPathRefusals(t *testing.T) {
	t.Run("unknown path", func(t *testing.T) {
		metadata := preflightMetadata(t, newPreflightMatrix())
		_, err := marshal.MarshalItemForWriterPath(newPreflightMatrix(), metadata, marshal.ItemWriterPath("nope"))
		require.Error(t, err)
		assert.ErrorIs(t, err, customerrors.ErrInvalidModel)
	})

	t.Run("nil metadata", func(t *testing.T) {
		_, err := marshal.MarshalItemForWriterPath(newPreflightMatrix(), nil, marshal.ItemWriterPathPut)
		require.Error(t, err)
		assert.ErrorIs(t, err, customerrors.ErrInvalidModel)
	})

	t.Run("encrypted model", func(t *testing.T) {
		metadata := preflightMetadata(t, &preflightEncrypted{})
		_, err := marshal.MarshalItemForWriterPath(&preflightEncrypted{PK: "p1"}, metadata, marshal.ItemWriterPathPut)
		require.Error(t, err)
		assert.ErrorIs(t, err, customerrors.ErrUnsupportedType)
	})

	t.Run("nil model", func(t *testing.T) {
		metadata := preflightMetadata(t, newPreflightMatrix())
		_, err := marshal.MarshalItemForWriterPath(nil, metadata, marshal.ItemWriterPathPut)
		require.Error(t, err)
	})
}

// TestMarshalItemForWriterPathPutMatchesSafeMarshaler pins that the put answer
// is literally the writer's serializer output, not a re-derivation of it.
func TestMarshalItemForWriterPathPutMatchesSafeMarshaler(t *testing.T) {
	metadata := preflightMetadata(t, newPreflightMatrix())

	want, err := marshal.NewSafeMarshaler().MarshalItem(newPreflightMatrix(), metadata)
	require.NoError(t, err)

	got, err := marshal.MarshalItemForWriterPath(newPreflightMatrix(), metadata, marshal.ItemWriterPathPut)
	require.NoError(t, err)

	assert.Equal(t, want, got)
}

// TestOptimizedMarshalerCollectionSemanticsArePinnedNotAligned records the
// second writer disagreement found while auditing the put family.
//
// The put family has two serializers: the default SafeMarshaler and the opt-in
// optimized marshaler created by New, which a database uses only when
// AllowUnsafeMarshaler is set with an explicit security acknowledgment. They do
// not agree for a nil collection, and the optimized marshaler does not even
// agree with itself between the root and a nested container. Its own rule is
// therefore not compatible with the default put writer, and it is recorded here
// rather than aligned, because aligning it would change what that writer stores.
func TestOptimizedMarshalerCollectionSemanticsArePinnedNotAligned(t *testing.T) {
	metadata := preflightMetadata(t, newPreflightMatrix())

	optimized, err := marshal.New(pkgtypes.NewConverter()).MarshalItem(newPreflightMatrix(), metadata)
	require.NoError(t, err)

	safe, err := marshal.NewSafeMarshaler().MarshalItem(newPreflightMatrix(), metadata)
	require.NoError(t, err)

	assert.Equal(t, "L", itemAttrType(optimized, "nilList"),
		"the optimized marshaler stores an empty L for a root nil list")
	assert.Equal(t, "NULL", nestedAttrType(optimized, "nested", "nestedNilList"),
		"the optimized marshaler stores NULL for a nested nil list, unlike its own root case")

	assert.Equal(t, "NULL", itemAttrType(safe, "nilList"), "SafeMarshaler stores NULL for a root nil list")
	assert.NotEqual(t, itemAttrType(safe, "nilList"), itemAttrType(optimized, "nilList"),
		"the default and opt-in put serializers are known to disagree; this pin fails if either is silently aligned")
}
