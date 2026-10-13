package errors

import (
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type cancellationProbe struct {
	Code string
}

func (e *cancellationProbe) Error() string { return "probe: " + e.Code }

type cancellationProbeCarrier interface {
	error
	probeCode() string
}

func (e *cancellationProbe) probeCode() string { return e.Code }

func TestTransactionErrorAsRecoversCause(t *testing.T) {
	cause := &cancellationProbe{Code: "TransactionConflict"}
	txErr := &TransactionError{Err: ErrTransactionFailed, Cause: cause}

	var probe *cancellationProbe
	require.True(t, errors.As(txErr, &probe))
	require.Same(t, cause, probe)

	var carrier cancellationProbeCarrier
	require.True(t, errors.As(txErr, &carrier))
	require.Equal(t, "TransactionConflict", carrier.probeCode())

	// errors.Is sentinel matching is unchanged.
	require.ErrorIs(t, txErr, ErrTransactionFailed)
}

func TestTransactionErrorAsGuardsNilAndSelfCause(t *testing.T) {
	t.Run("nil cause", func(t *testing.T) {
		txErr := &TransactionError{Err: ErrTransactionFailed}
		var probe *cancellationProbe
		assert.False(t, txErr.As(&probe))
		assert.False(t, errors.As(txErr, &probe))
	})

	t.Run("nil receiver", func(t *testing.T) {
		var txErr *TransactionError
		var probe *cancellationProbe
		assert.False(t, txErr.As(&probe))
	})

	t.Run("self-referential cause terminates", func(t *testing.T) {
		txErr := &TransactionError{Err: ErrTransactionFailed}
		txErr.Cause = txErr
		var probe *cancellationProbe
		assert.False(t, errors.As(txErr, &probe))
	})
}

func TestTransactionErrorRealFailures(t *testing.T) {
	txErr := &TransactionError{
		Err: ErrConditionFailed,
		Reasons: []CancellationReason{
			{Index: 0, Code: "None"},
			{Index: 1, Code: "ConditionalCheckFailed", Message: "first"},
			{Index: 2, Code: "", Message: "missing code"},
			{Index: 3, Code: "TransactionConflict", Message: "second"},
		},
	}

	failures := txErr.RealFailures()
	require.Len(t, failures, 2)
	assert.Equal(t, CancellationReason{Index: 1, Code: "ConditionalCheckFailed", Message: "first"}, failures[0])
	assert.Equal(t, CancellationReason{Index: 3, Code: "TransactionConflict", Message: "second"}, failures[1])

	var nilErr *TransactionError
	assert.Nil(t, nilErr.RealFailures())
}

// TestCancellationReasonCarriesNoItemImage locks the security invariant that a
// cancellation reason can never hold a DynamoDB item image: the type exposes
// exactly Index, Code and Message.
func TestCancellationReasonCarriesNoItemImage(t *testing.T) {
	reasonType := reflect.TypeOf(CancellationReason{})
	require.Equal(t, 3, reasonType.NumField())
	fields := make([]string, 0, reasonType.NumField())
	for i := 0; i < reasonType.NumField(); i++ {
		fields = append(fields, reasonType.Field(i).Name)
	}
	assert.ElementsMatch(t, []string{"Index", "Code", "Message"}, fields)
}
