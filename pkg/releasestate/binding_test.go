package releasestate

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	theorydbErrors "github.com/theory-cloud/tabletheory/v6/pkg/errors"
	"github.com/theory-cloud/tabletheory/v6/pkg/naming"
)

type bindingActual struct {
	PK              string `theorydb:"pk,attr:PK"`
	SK              string `theorydb:"sk,attr:SK"`
	PinnedReleaseID string `theorydb:"attr:pinnedReleaseId"`
	Status          string `theorydb:"attr:status"`
	Version         int64  `theorydb:"version,attr:version"`
}

type bindingEvent struct {
	PK        string `theorydb:"pk,attr:PK"`
	SK        string `theorydb:"sk,attr:SK"`
	ReleaseID string `theorydb:"attr:releaseId"`
	EventType string `theorydb:"attr:eventType"`
	Version   int64  `theorydb:"attr:version"`
}

type bindingOutbox struct {
	PK        string `theorydb:"pk,attr:PK"`
	SK        string `theorydb:"sk,attr:SK"`
	ReleaseID string `theorydb:"attr:releaseId"`
	Operation string `theorydb:"attr:operation"`
}

type bindingKeyedOutbox struct {
	OutboxID  string `theorydb:"pk,attr:outboxId"`
	SK        string `theorydb:"sk,attr:SK"`
	ReleaseID string `theorydb:"attr:releaseId"`
}

func bindingEventOf(t *testing.T, input TransitionAppendEventInput) *bindingEvent {
	t.Helper()
	event, ok := input.Event.(*bindingEvent)
	require.True(t, ok)
	return event
}

func bindingActualOf(t *testing.T, input TransitionAppendEventInput) *bindingActual {
	t.Helper()
	actual, ok := input.Actual.(*bindingActual)
	require.True(t, ok)
	return actual
}

func bindingInput() TransitionAppendEventInput {
	return TransitionAppendEventInput{
		Actual: &bindingActual{PK: "RELEASE#service-a", SK: "ACTUAL"},
		Event: &bindingEvent{
			PK:        "RELEASE#service-a",
			SK:        "EVENT#1",
			ReleaseID: "rel_002",
			EventType: "active",
		},
		Set: map[string]any{
			"pinnedReleaseId": "rel_002",
			"status":          "active",
		},
	}
}

func TestTransitionEventBindingAcceptsMatchingTransition(t *testing.T) {
	input := bindingInput()
	input.EventBinding = map[string]string{
		"releaseId": "set:pinnedReleaseId",
		"eventType": "set:status",
	}

	writer := &fakeTransactionWriter{}
	require.NoError(t, TransitionAppendEvent(context.Background(), writer, input))
	require.True(t, writer.called)
}

func TestTransitionEventBindingRejectsMismatchedTransition(t *testing.T) {
	input := bindingInput()
	input.EventBinding = map[string]string{"releaseId": "set:pinnedReleaseId"}
	bindingEventOf(t, input).ReleaseID = "rel_001"

	writer := &fakeTransactionWriter{}
	err := TransitionAppendEvent(context.Background(), writer, input)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)
	require.False(t, writer.called, "a mismatched event must not reach the transaction")
}

func TestTransitionEventBindingRejectsMissingAttribute(t *testing.T) {
	input := bindingInput()
	input.EventBinding = map[string]string{"missing": "set:pinnedReleaseId"}

	err := TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)
}

func TestTransitionEventBindingResolvesActualFields(t *testing.T) {
	input := bindingInput()
	bindingActualOf(t, input).PinnedReleaseID = "rel_001"
	bindingEventOf(t, input).ReleaseID = "rel_001"
	input.EventBinding = map[string]string{"releaseId": "actual:pinnedReleaseId"}

	require.NoError(t, TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input))
}

func TestTransitionEventBindingSupportsNextVersion(t *testing.T) {
	expected := int64(4)
	input := bindingInput()
	input.ExpectedVersion = &expected
	bindingEventOf(t, input).Version = 5
	input.EventBinding = map[string]string{"version": "nextVersion"}

	require.NoError(t, TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input))

	bindingEventOf(t, input).Version = 4
	err := TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)

	input.ExpectedVersion = nil
	err = TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)
}

func TestTransitionEventBindingRejectsUnknownSource(t *testing.T) {
	input := bindingInput()
	input.EventBinding = map[string]string{"releaseId": "policy:whatever"}

	err := TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)
}

func TestTransitionOutboxBindingRejectsMismatchedTransition(t *testing.T) {
	input := bindingInput()
	input.Outbox = &bindingOutbox{
		PK:        "RELEASE#service-a",
		SK:        "OUTBOX#1",
		ReleaseID: "rel_001",
	}
	input.OutboxBinding = map[string]string{"releaseId": "set:pinnedReleaseId"}

	writer := &fakeTransactionWriter{}
	err := TransitionAppendEvent(context.Background(), writer, input)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)
	require.False(t, writer.called)
}

func TestTransitionOutboxPartitionMustMatchActual(t *testing.T) {
	input := bindingInput()
	input.Outbox = &bindingOutbox{PK: "RELEASE#service-b", SK: "OUTBOX#1"}

	writer := &fakeTransactionWriter{}
	err := TransitionAppendEvent(context.Background(), writer, input)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)
	require.False(t, writer.called)
}

func TestTransitionOutboxWithDifferentKeyAttributeIsAllowed(t *testing.T) {
	input := bindingInput()
	input.Outbox = &bindingKeyedOutbox{
		OutboxID:  "outbox#1",
		SK:        "OUTBOX#1",
		ReleaseID: "rel_002",
	}
	input.OutboxBinding = map[string]string{"releaseId": "set:pinnedReleaseId"}

	writer := &fakeTransactionWriter{}
	require.NoError(t, TransitionAppendEvent(context.Background(), writer, input))
	require.True(t, writer.called)
}

func TestTransitionOutboxBindingWithoutOutboxIsRejected(t *testing.T) {
	input := bindingInput()
	input.OutboxBinding = map[string]string{"releaseId": "set:pinnedReleaseId"}

	err := TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)
}

type bindingNoPartitionOutbox struct {
	SK        string `theorydb:"sk,attr:SK"`
	Operation string `theorydb:"attr:operation"`
}

func TestTransitionBindingResolvesGoFieldNames(t *testing.T) {
	input := bindingInput()
	bindingActualOf(t, input).PinnedReleaseID = "rel_001"
	bindingEventOf(t, input).ReleaseID = "rel_001"
	input.EventBinding = map[string]string{"releaseId": "actual:PinnedReleaseID"}

	require.NoError(t, TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input))

	input.EventBinding = map[string]string{"releaseId": "actual:Missing"}
	err := TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)
}

func TestTransitionBindingAcceptsNumericEquivalence(t *testing.T) {
	expected := int64(4)
	input := bindingInput()
	input.ExpectedVersion = &expected
	input.Set["status"] = 5
	bindingEventOf(t, input).Version = 5
	input.EventBinding = map[string]string{"version": "set:status"}
	require.NoError(t, TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input))

	input.Set["status"] = uint(5)
	require.NoError(t, TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input))

	// Strings are not numeric identities: "5" and "007" must not satisfy an
	// integer row field, or a string-typed version could erode the binding.
	input.Set["status"] = "5"
	err := TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)

	input.Set["status"] = "not-a-number"
	err = TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)
}

func TestTransitionNextVersionRejectsNumericStringCoercion(t *testing.T) {
	type stringVersionEvent struct {
		PK      string `theorydb:"pk,attr:PK"`
		SK      string `theorydb:"sk,attr:SK"`
		Version string `theorydb:"attr:version"`
	}

	expected := int64(6)
	input := TransitionAppendEventInput{
		Actual:          &bindingActual{PK: "RELEASE#service-a", SK: "ACTUAL"},
		Event:           &stringVersionEvent{PK: "RELEASE#service-a", SK: "EVENT#1", Version: "007"},
		ExpectedVersion: &expected,
		Set:             map[string]any{"status": "active"},
		EventBinding:    map[string]string{"version": "nextVersion"},
	}

	err := TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel, `"007" must not satisfy nextVersion=7`)
}

func TestTransitionBindingEmptyMapDeclaresNothing(t *testing.T) {
	input := bindingInput()
	bindingEventOf(t, input).ReleaseID = "rel_999"
	bindingEventOf(t, input).EventType = "unrelated"
	input.EventBinding = map[string]string{}

	// An empty map is the explicit opt-out: no attribute is declared, so the
	// generically knowable checks (partition) still run and nothing else is
	// compared. A partial map checks exactly the attributes it names.
	require.NoError(t, TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input))

	input.EventBinding = map[string]string{"releaseId": "set:pinnedReleaseId"}
	err := TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)
}

func TestTransitionOutboxWithoutPartitionKeyIsRejected(t *testing.T) {
	input := bindingInput()
	input.Outbox = &bindingNoPartitionOutbox{SK: "OUTBOX#1", Operation: "flip"}

	err := TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)
}

func TestTransitionBindingValueComparison(t *testing.T) {
	type payload struct {
		A int `json:"a"`
	}
	require.True(t, bindingValuesEqual(payload{A: 1}, payload{A: 1}))
	require.False(t, bindingValuesEqual(payload{A: 1}, payload{A: 2}))
	require.True(t, bindingValuesEqual(1.5, 1.5))
	require.False(t, bindingValuesEqual(nil, 1))
}

func TestTransitionBindingRejectsNonStructRows(t *testing.T) {
	input := bindingInput()
	input.Actual = 42

	err := TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)
}

func TestTransitionBindingRejectsNilOutboxBindingWithoutRow(t *testing.T) {
	input := bindingInput()
	input.Outbox = nil
	input.OutboxBinding = map[string]string{"releaseId": "set:pinnedReleaseId"}

	err := TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)
}

func TestTransitionBindingMatchesFieldAliases(t *testing.T) {
	input := bindingInput()
	bindingEventOf(t, input).ReleaseID = "rel_002"
	input.EventBinding = map[string]string{"releaseId": "set:pinnedReleaseId"}
	require.NoError(t, TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input))

	delete(input.Set, "pinnedReleaseId")
	input.Set["PinnedReleaseID"] = "rel_002"
	input.EventBinding = map[string]string{"releaseId": "set:pinnedReleaseId"}
	require.NoError(t, TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input))

	input.EventBinding = map[string]string{"releaseId": "set:PinnedReleaseID"}
	require.NoError(t, TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input))
}

func TestBindingValuesEqualFloats(t *testing.T) {
	require.True(t, bindingValuesEqual(float32(1.5), 1.5))
}

func TestBindingValuesEqualComparesNumbersExactly(t *testing.T) {
	tests := map[string]struct {
		left  any
		right any
		equal bool
	}{
		"equal int64 above 2^53": {
			left: int64(9007199254740993), right: int64(9007199254740993), equal: true,
		},
		"adjacent int64 above 2^53": {
			left: int64(9007199254740992), right: int64(9007199254740993), equal: false,
		},
		"adjacent uint64 above 2^53": {
			left: uint64(9007199254740992), right: uint64(9007199254740993), equal: false,
		},
		"int64 equals the same exact float64": {
			left: int64(9007199254740992), right: float64(9007199254740992), equal: true,
		},
		"int64 differs from a rounded float64": {
			left: int64(9007199254740993), right: float64(9007199254740992), equal: false,
		},
		"uint64 equals int64": {
			left: uint64(5), right: int64(5), equal: true,
		},
		"signed and unsigned differ": {
			left: int64(-1), right: uint64(1), equal: false,
		},
		"string never matches a number": {
			left: "5", right: int64(5), equal: false,
		},
		"lexical strings are distinct": {
			left: "001", right: "1", equal: false,
		},
		"nested maps recurse exactly": {
			left:  map[string]any{"ttl": int64(9007199254740992)},
			right: map[string]any{"ttl": int64(9007199254740993)},
			equal: false,
		},
		"nested numeric kinds match exactly": {
			left:  map[string]any{"ttl": uint64(5)},
			right: map[string]any{"ttl": float64(5)},
			equal: true,
		},
		"nested lists recurse exactly": {
			left:  []any{int64(9007199254740992)},
			right: []any{int64(9007199254740993)},
			equal: false,
		},
	}

	for name, testCase := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, testCase.equal, bindingValuesEqual(testCase.left, testCase.right))
		})
	}
}

func TestTransitionNextVersionRejectsAdjacentLargeIntegers(t *testing.T) {
	expected := int64(9007199254740992)
	input := bindingInput()
	input.ExpectedVersion = &expected
	input.EventBinding = map[string]string{"version": "nextVersion"}

	bindingEventOf(t, input).Version = 9007199254740992
	writer := &fakeTransactionWriter{}
	err := TransitionAppendEvent(context.Background(), writer, input)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)
	require.False(t, writer.called, "a lossy adjacent integer must not reach the transaction")

	bindingEventOf(t, input).Version = 9007199254740993
	require.NoError(t, TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input))
}

func TestTransitionBindingRejectsStringNumberConfusion(t *testing.T) {
	expected := int64(0)
	input := bindingInput()
	input.ExpectedVersion = &expected
	input.EventBinding = map[string]string{"releaseId": "nextVersion"}
	bindingEventOf(t, input).ReleaseID = "1"

	require.ErrorIs(t, TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input), theorydbErrors.ErrInvalidModel)
}

type bindingEmbeddedActual struct {
	PK string `theorydb:"pk,attr:PK"`
	SK string `theorydb:"sk,attr:SK"`
	BindingEmbeddedFields
}

type BindingEmbeddedFields struct {
	PinnedReleaseID string `theorydb:"attr:pinnedReleaseId"`
}

func TestTransitionBindingReadsPromotedEmbeddedFields(t *testing.T) {
	input := TransitionAppendEventInput{
		Actual: &bindingEmbeddedActual{
			PK:                    "RELEASE#service-a",
			SK:                    "ACTUAL",
			BindingEmbeddedFields: BindingEmbeddedFields{PinnedReleaseID: "rel_002"},
		},
		Event: &bindingEvent{
			PK:        "RELEASE#service-a",
			SK:        "EVENT#1",
			ReleaseID: "rel_002",
		},
		Set:          map[string]any{"status": "active"},
		EventBinding: map[string]string{"releaseId": "actual:pinnedReleaseId"},
	}

	require.NoError(t, TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input))
}

type bindingGsiFirst struct {
	GsiPK string `theorydb:"index:gsi1,pk,attr:gsi1pk"`
	PK    string `theorydb:"pk,attr:PK"`
	SK    string `theorydb:"sk,attr:SK"`
}

func TestTransitionOutboxPartitionUsesTablePKNotIndexKey(t *testing.T) {
	input := bindingInput()
	input.Actual = &bindingGsiFirst{
		GsiPK: "RELEASE#service-a",
		PK:    "RELEASE#service-b",
		SK:    "ACTUAL",
	}
	// Keep the event row in the actual row's release partition so the
	// always-on event check passes; only the outbox partition is under test.
	bindingEventOf(t, input).PK = "RELEASE#service-b"
	input.Outbox = &bindingGsiFirst{
		GsiPK: "RELEASE#service-a",
		PK:    "RELEASE#service-z",
		SK:    "OUTBOX#1",
	}

	writer := &fakeTransactionWriter{}
	err := TransitionAppendEvent(context.Background(), writer, input)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel,
		"a matching index key must not stand in for the table partition")
	require.ErrorContains(t, err, "outbox release partition")
	require.False(t, writer.called)
}

type bindingJSONTagActual struct {
	PK              string `theorydb:"pk,attr:PK"`
	SK              string `theorydb:"sk,attr:SK"`
	PinnedReleaseID string `json:"pinnedReleaseId"`
}

func TestTransitionBindingUsesStoredAttributeNamesNotJSONTags(t *testing.T) {
	input := bindingInput()
	input.Actual = &bindingJSONTagActual{
		PK:              "RELEASE#service-a",
		SK:              "ACTUAL",
		PinnedReleaseID: "rel_002",
	}
	input.EventBinding = map[string]string{"releaseId": "actual:pinnedReleaseID"}

	require.NoError(t, TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input))

	input.EventBinding = map[string]string{"releaseId": "actual:pinnedReleaseId"}
	err := TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel,
		"a json tag is not a stored attribute name")
}

type bindingExcludedEmbeddedFields struct {
	DupPK           string `theorydb:"pk,attr:PK"`
	PinnedReleaseID string `theorydb:"attr:pinnedReleaseId"`
}

type bindingExcludedEmbeddedActual struct {
	PK       string                        `theorydb:"pk,attr:PK"`
	SK       string                        `theorydb:"sk,attr:SK"`
	Excluded bindingExcludedEmbeddedFields `theorydb:"-"`
}

func TestTransitionBindingHonorsEmbeddedExclusionTags(t *testing.T) {
	input := bindingInput()
	input.Actual = &bindingExcludedEmbeddedActual{
		PK:       "RELEASE#service-a",
		SK:       "ACTUAL",
		Excluded: bindingExcludedEmbeddedFields{PinnedReleaseID: "rel_002"},
	}
	input.EventBinding = map[string]string{"releaseId": "actual:pinnedReleaseId"}

	err := TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel,
		"an excluded embedded subtree is not part of the row")
}

type bindingDuplicateAttribute struct {
	PK    string `theorydb:"pk,attr:PK"`
	Other string `theorydb:"attr:PK"`
}

func TestDescribeBindingRowRejectsDuplicateAttributeNames(t *testing.T) {
	_, err := describeBindingRow(&bindingDuplicateAttribute{})
	require.ErrorIs(t, err, theorydbErrors.ErrDuplicatePrimaryKey)
}

type bindingStructuralExcluded struct {
	PinnedReleaseID string `theorydb:"attr:pinnedReleaseId"`
}

type bindingStructuralRow struct {
	_               struct{} `theorydb:"naming:snake_case"`
	GsiPK           string   `theorydb:"index:gsi1,pk,attr:gsi1pk"`
	PK              string   `theorydb:"pk,attr:release_id"`
	SK              string   `theorydb:"sk,attr:sort_id"`
	PinnedReleaseID string
	IndexOnly       string                    `theorydb:"index:gsi1,sk"`
	Excluded        bindingStructuralExcluded `theorydb:"-"`
}

func TestDescribeBindingRowStructurallyMirrorsRegistryRules(t *testing.T) {
	metadata, err := describeBindingRowStructurally(reflect.TypeOf(bindingStructuralRow{}))
	require.NoError(t, err)

	require.Equal(t, "release_id", metadata.partitionAttribute,
		"a pk token inside an index clause is not the table partition")
	require.Equal(t, "sort_id", metadata.sortAttribute,
		"a sk token inside an index clause is not the table sort key")

	pinned, ok := metadata.fieldsByDBName["pinned_release_id"]
	require.True(t, ok, "attribute names follow the model naming convention, not json tags")
	require.Equal(t, "PinnedReleaseID", pinned.name)

	_, ok = metadata.fieldsByDBName["pinnedReleaseId"]
	require.False(t, ok)

	_, ok = metadata.fieldsByDBName["PinnedReleaseID"]
	require.False(t, ok, "an excluded embedded subtree is not part of the row")
}

type bindingJSONExcluded struct {
	Other string
}

type bindingStructuralJSONRow struct {
	bindingJSONExcluded `json:"-"`
	PK                  string `theorydb:"pk,attr:PK"`
}

func TestDescribeBindingRowStructurallyHonorsJSONExclusion(t *testing.T) {
	metadata, err := describeBindingRowStructurally(reflect.TypeOf(bindingStructuralJSONRow{}))
	require.NoError(t, err)

	_, ok := metadata.fieldsByDBName["Other"]
	require.False(t, ok, "a json \"-\" tagged anonymous embed is not part of the row")
}

func TestDescribeBindingRowStructurallyRejectsDuplicateNames(t *testing.T) {
	type duplicate struct {
		PK   string `theorydb:"pk,attr:PK"`
		Also string `theorydb:"attr:PK"`
	}

	_, err := describeBindingRowStructurally(reflect.TypeOf(duplicate{}))
	require.ErrorIs(t, err, theorydbErrors.ErrDuplicatePrimaryKey)
}

func TestDetectBindingNamingConvention(t *testing.T) {
	cases := map[string]struct {
		model      reflect.Type
		convention naming.Convention
	}{
		"snake": {reflect.TypeOf(struct {
			_ struct{} `theorydb:"naming:snake_case"`
		}{}), naming.SnakeCase},
		"camel": {reflect.TypeOf(struct {
			_ struct{} `theorydb:"naming:camelCase"`
		}{}), naming.CamelCase},
		"pascal": {reflect.TypeOf(struct {
			_ struct{} `theorydb:"naming:pascal_case"`
		}{}), naming.PascalCase},
		"dynamorm": {reflect.TypeOf(struct {
			_ struct{} `theorydb:"naming:dynamorm"`
		}{}), naming.DynamORM},
		"default": {reflect.TypeOf(struct{}{}), naming.CamelCase},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.convention, detectBindingNamingConvention(tc.model))
		})
	}
}

type bindingNestedMapEvent struct {
	Payload map[string]any `theorydb:"attr:payload"`
	PK      string         `theorydb:"pk,attr:PK"`
	SK      string         `theorydb:"sk,attr:SK"`
}

func TestTransitionNestedMapKeyTypeMismatchRejectsWithoutPanic(t *testing.T) {
	input := TransitionAppendEventInput{
		Actual: &bindingActual{PK: "RELEASE#service-a", SK: "ACTUAL"},
		Event: &bindingNestedMapEvent{
			PK:      "RELEASE#service-a",
			SK:      "EVENT#1",
			Payload: map[string]any{"1": "one"},
		},
		Set:          map[string]any{"payload": map[int]string{1: "one"}},
		EventBinding: map[string]string{"payload": "set:payload"},
	}

	writer := &fakeTransactionWriter{}
	var err error
	require.NotPanics(t, func() {
		err = TransitionAppendEvent(context.Background(), writer, input)
	})
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel,
		"incompatible nested map key types must reject, not compare")
	require.False(t, writer.called, "a key-type mismatch must not reach the transaction")

	input.Set["payload"] = map[string]any{"1": "one"}
	require.NoError(t, TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input),
		"compatible nested map key types must keep comparing exactly")
}

func TestTransitionNextVersionRejectsMaxInt64Overflow(t *testing.T) {
	expected := int64(math.MaxInt64)
	input := bindingInput()
	input.ExpectedVersion = &expected
	input.EventBinding = map[string]string{"version": "nextVersion"}
	// The defective addition wraps MaxInt64 to MinInt64, which would satisfy
	// this binding and reach the writer.
	bindingEventOf(t, input).Version = math.MinInt64

	writer := &fakeTransactionWriter{}
	err := TransitionAppendEvent(context.Background(), writer, input)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)
	require.False(t, writer.called, "an overflowing nextVersion must not reach the transaction")
}

func TestTransitionNextVersionAcceptsMaxInt64MinusOne(t *testing.T) {
	expected := int64(math.MaxInt64 - 1)
	input := bindingInput()
	input.ExpectedVersion = &expected
	input.EventBinding = map[string]string{"version": "nextVersion"}
	bindingEventOf(t, input).Version = math.MaxInt64

	writer := &fakeTransactionWriter{}
	require.NoError(t, TransitionAppendEvent(context.Background(), writer, input))
	require.True(t, writer.called)
}

type bindingSliceEvent struct {
	PK     string   `theorydb:"pk,attr:PK"`
	SK     string   `theorydb:"sk,attr:SK"`
	Events []string `theorydb:"attr:events"`
}

func TestBindingValuesDistinguishNilAndEmptySlices(t *testing.T) {
	require.False(t, bindingValuesEqual([]string(nil), []string{}),
		"a nil list and an empty list are different DynamoDB identities")
	require.True(t, bindingValuesEqual([]string{}, []string{}))
	require.False(t, bindingValuesEqual(
		map[string]any{"events": []string(nil)},
		map[string]any{"events": []string{}},
	))
	require.True(t, bindingValuesEqual(
		map[string]any{"events": []string{}},
		map[string]any{"events": []string{}},
	))
	require.True(t, bindingValuesEqual([0]string{}, [0]string{}),
		"arrays are not slices and keep structural equality")
}

func TestTransitionBindingDistinguishesNilAndEmptySlices(t *testing.T) {
	input := TransitionAppendEventInput{
		Actual:       &bindingActual{PK: "RELEASE#service-a", SK: "ACTUAL"},
		Event:        &bindingSliceEvent{PK: "RELEASE#service-a", SK: "EVENT#1", Events: []string{}},
		Set:          map[string]any{"events": []string(nil)},
		EventBinding: map[string]string{"events": "set:events"},
	}

	writer := &fakeTransactionWriter{}
	err := TransitionAppendEvent(context.Background(), writer, input)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)
	require.False(t, writer.called, "a nil list must not satisfy an empty list")

	input.Set["events"] = []string{}
	require.NoError(t, TransitionAppendEvent(context.Background(), &fakeTransactionWriter{}, input))
}
