package marshal_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	customerrors "github.com/theory-cloud/tabletheory/v6/pkg/errors"
	"github.com/theory-cloud/tabletheory/v6/pkg/marshal"
)

const clockTaskFixed = "2026-01-02T03:04:05.123456789Z"

func staticClock(t *testing.T, value string) func() time.Time {
	t.Helper()
	stamp := parseLifecycleStamp(t, value)
	return func() time.Time { return stamp }
}

// TestMarshalItemForWriterPathWithOptionsMatchesFixedClockWriters pins the
// truthful half of the lifecycle claim: when the preflight is given the clock
// the writer read, both answers byte-equal the captured payloads.
func TestMarshalItemForWriterPathWithOptionsMatchesFixedClockWriters(t *testing.T) {
	metadata := preflightMetadata(t, newLifecycleClocksItem())
	capturedPut := captureLifecyclePut(t, &lifecycleClock{stamps: []time.Time{parseLifecycleStamp(t, clockTaskFixed)}})
	capturedTransact := captureLifecycleTransact(t, &lifecycleClock{stamps: []time.Time{parseLifecycleStamp(t, clockTaskFixed)}})

	gotPut, err := marshal.MarshalItemForWriterPathWithOptions(
		newLifecycleClocksItem(), metadata, marshal.ItemWriterPathPut,
		marshal.PreflightOptions{Now: staticClock(t, clockTaskFixed)},
	)
	require.NoError(t, err)
	assert.Equal(t, capturedPut, gotPut, "a put preflight with the writer's clock must byte-equal the capture")

	// The transactional writer never stamps lifecycle fields in this runtime,
	// so it needs no clock and its preflight already matches it byte for byte.
	gotTransact, err := marshal.MarshalItemForWriterPathWithOptions(
		newLifecycleClocksItem(), metadata, marshal.ItemWriterPathTransactWrite,
		marshal.PreflightOptions{},
	)
	require.NoError(t, err)
	assert.Equal(t, capturedTransact, gotTransact,
		"the transactional preflight must byte-equal the capture without a clock")
}

// TestMarshalItemForWriterPathRefusesLifecycleModelWithoutClock is the negative
// control: without the writer's clock the equality claim cannot be supported, so
// the preflight must fail closed instead of returning a payload that will not
// match what the writer stores.
func TestMarshalItemForWriterPathRefusesLifecycleModelWithoutClock(t *testing.T) {
	metadata := preflightMetadata(t, newLifecycleClocksItem())

	_, err := marshal.MarshalItemForWriterPath(newLifecycleClocksItem(), metadata, marshal.ItemWriterPathPut)
	require.Error(t, err)
	assert.True(t, errors.Is(err, customerrors.ErrUnsupportedType), "the refusal must be a typed unsupported error: %v", err)

	_, err = marshal.MarshalItemForWriterPathWithOptions(
		newLifecycleClocksItem(), metadata, marshal.ItemWriterPathPut, marshal.PreflightOptions{},
	)
	require.Error(t, err)
	assert.True(t, errors.Is(err, customerrors.ErrUnsupportedType))

	// The transactional path has no lifecycle stamp to predict, so it stays
	// available without a clock.
	_, err = marshal.MarshalItemForWriterPath(newLifecycleClocksItem(), metadata, marshal.ItemWriterPathTransactWrite)
	require.NoError(t, err)
}

// TestMarshalItemForWriterPathWithClockIsStillAClockComparison shows that the
// option is not a permission flag: a clock the writer did not read produces a
// different payload, which is why an advancing or missing clock cannot support
// the claim.
func TestMarshalItemForWriterPathWithClockIsStillAClockComparison(t *testing.T) {
	metadata := preflightMetadata(t, newLifecycleClocksItem())
	capturedPut := captureLifecyclePut(t, &lifecycleClock{stamps: []time.Time{parseLifecycleStamp(t, clockTaskFixed)}})

	gotPut, err := marshal.MarshalItemForWriterPathWithOptions(
		newLifecycleClocksItem(), metadata, marshal.ItemWriterPathPut,
		marshal.PreflightOptions{Now: staticClock(t, "2026-01-02T03:04:06.000000002Z")},
	)
	require.NoError(t, err)
	assert.NotEqual(t, capturedPut, gotPut,
		"a preflight must report the bytes its own clock produces, not claim a match it cannot support")
}
