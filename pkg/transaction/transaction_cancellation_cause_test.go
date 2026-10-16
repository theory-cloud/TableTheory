package transaction

import (
	"errors"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	theorydbErrors "github.com/theory-cloud/tabletheory/v6/pkg/errors"
)

func TestBuilderTransactionErrorExposesCauseAndOrderedReasons(t *testing.T) {
	canceled := &types.TransactionCanceledException{
		CancellationReasons: []types.CancellationReason{
			{Code: aws.String("None")},
			{Code: aws.String("ConditionalCheckFailed"), Message: aws.String("survivor condition failed")},
		},
	}
	builder := &Builder{operations: []transactOperation{
		{typ: opPut, model: &User{}},
		{typ: opConditionCheck, model: &Order{}},
	}}

	err := builder.buildTransactionError(canceled, canceled)
	require.ErrorIs(t, err, theorydbErrors.ErrConditionFailed)

	var txErr *theorydbErrors.TransactionError
	require.ErrorAs(t, err, &txErr)
	require.Len(t, txErr.Reasons, 2)
	require.Equal(t, theorydbErrors.CancellationReason{Index: 0, Code: "None"}, txErr.Reasons[0])
	require.Equal(t,
		theorydbErrors.CancellationReason{Index: 1, Code: "ConditionalCheckFailed", Message: "survivor condition failed"},
		txErr.Reasons[1],
	)

	// The original AWS exception is recoverable, unchanged, through errors.As.
	var recovered *types.TransactionCanceledException
	require.ErrorAs(t, err, &recovered)
	require.Same(t, canceled, recovered)

	// RealFailures keeps only the genuine failure, in order.
	failures := txErr.RealFailures()
	require.Equal(t, []theorydbErrors.CancellationReason{txErr.Reasons[1]}, failures)
}

func TestBuilderTransactionErrorNeverExposesItemImages(t *testing.T) {
	const secret = "top-secret-value"
	canceled := &types.TransactionCanceledException{
		CancellationReasons: []types.CancellationReason{
			{
				Code:    aws.String("ConditionalCheckFailed"),
				Message: aws.String("conditional request failed"),
				Item: map[string]types.AttributeValue{
					"secret": &types.AttributeValueMemberS{Value: secret},
				},
			},
		},
	}
	builder := &Builder{operations: []transactOperation{{typ: opPut, model: &User{}}}}

	err := builder.buildTransactionError(canceled, canceled)
	require.NotContains(t, err.Error(), secret)
	require.NotContains(t, fmt.Sprintf("%v", err), secret)
}

func TestTransactionCancellationErrorExposesCauseAndFirstRealFailure(t *testing.T) {
	canceled := &types.TransactionCanceledException{
		CancellationReasons: []types.CancellationReason{
			{Code: aws.String("None")},
			{Code: aws.String("TransactionConflict"), Message: aws.String("raced transaction")},
			{Code: aws.String("ConditionalCheckFailed"), Message: aws.String("loser")},
		},
	}
	tx := &Transaction{writes: []types.TransactWriteItem{
		{Put: &types.Put{}},
		{Update: &types.Update{}},
		{ConditionCheck: &types.ConditionCheck{}},
	}}

	err := tx.handleTransactionError(canceled)
	// The FIRST genuine failure (the conflict) decides the sentinel, matching
	// the Builder path; a later condition failure must not override it.
	require.ErrorIs(t, err, theorydbErrors.ErrTransactionConflict)
	require.ErrorIs(t, err, theorydbErrors.ErrTransactionFailed)
	require.NotErrorIs(t, err, theorydbErrors.ErrConditionFailed)

	var txErr *theorydbErrors.TransactionError
	require.ErrorAs(t, err, &txErr)
	require.Equal(t, 1, txErr.OperationIndex)
	require.Equal(t, "Update", txErr.Operation)
	require.Equal(t, "raced transaction", txErr.Reason)
	require.Len(t, txErr.Reasons, 3)
	require.Equal(t, "None", txErr.Reasons[0].Code)
	require.Equal(t, "ConditionalCheckFailed", txErr.Reasons[2].Code)

	var recovered *types.TransactionCanceledException
	require.ErrorAs(t, err, &recovered)
	require.Same(t, canceled, recovered)
}

func TestTransactionCancellationThrottleKeepsCauseRecoverable(t *testing.T) {
	canceled := &types.TransactionCanceledException{
		CancellationReasons: []types.CancellationReason{
			{Code: aws.String("ThrottlingError"), Message: aws.String("slow down")},
		},
	}

	err := (&Transaction{}).handleTransactionError(canceled)
	require.ErrorIs(t, err, theorydbErrors.ErrThrottled)

	// The throttled path must no longer break errors.As on the original error.
	var recovered *types.TransactionCanceledException
	require.ErrorAs(t, err, &recovered)
	require.Same(t, canceled, recovered)

	var txErr *theorydbErrors.TransactionError
	require.ErrorAs(t, err, &txErr)
	require.Equal(t, "ThrottlingError", txErr.Reasons[0].Code)
}

func TestTransactionCancellationUnknownCodeIsGenericNotConditional(t *testing.T) {
	canceled := &types.TransactionCanceledException{
		CancellationReasons: []types.CancellationReason{
			{Code: aws.String("SomeUnmappedReason"), Message: aws.String("who knows")},
		},
	}

	err := (&Transaction{}).handleTransactionError(canceled)
	require.ErrorIs(t, err, theorydbErrors.ErrTransactionFailed)
	require.NotErrorIs(t, err, theorydbErrors.ErrConditionFailed)
	require.NotErrorIs(t, err, theorydbErrors.ErrTransactionConflict)
}

func TestTransactionCancellationAllNoneIsGenericNotConditional(t *testing.T) {
	canceled := &types.TransactionCanceledException{
		CancellationReasons: []types.CancellationReason{
			{Code: aws.String("None")},
			{Code: nil},
		},
	}

	err := (&Transaction{}).handleTransactionError(canceled)
	require.ErrorIs(t, err, theorydbErrors.ErrTransactionFailed)
	require.NotErrorIs(t, err, theorydbErrors.ErrConditionFailed)
	require.NotErrorIs(t, err, theorydbErrors.ErrTransactionConflict)

	var txErr *theorydbErrors.TransactionError
	require.ErrorAs(t, err, &txErr)
	require.Equal(t, -1, txErr.OperationIndex)
	require.Len(t, txErr.Reasons, 2)
	require.Equal(t, "None", txErr.Reasons[0].Code)
	require.Equal(t, "", txErr.Reasons[1].Code)

	var recovered *types.TransactionCanceledException
	require.ErrorAs(t, err, &recovered)
	require.Same(t, canceled, recovered)
}

func TestTransactionCancellationEmptyReasonsIsGeneric(t *testing.T) {
	canceled := &types.TransactionCanceledException{}

	err := (&Transaction{}).handleTransactionError(canceled)
	require.ErrorIs(t, err, theorydbErrors.ErrTransactionFailed)
	require.NotErrorIs(t, err, theorydbErrors.ErrConditionFailed)

	var txErr *theorydbErrors.TransactionError
	require.ErrorAs(t, err, &txErr)
	require.Empty(t, txErr.Reasons)
}

func TestTransactionConditionalCheckFailedExceptionExposesCause(t *testing.T) {
	exc := &types.ConditionalCheckFailedException{Message: aws.String("conditional request failed")}

	err := (&Transaction{}).handleTransactionError(exc)
	require.ErrorIs(t, err, theorydbErrors.ErrConditionFailed)

	var txErr *theorydbErrors.TransactionError
	require.ErrorAs(t, err, &txErr)

	var recovered *types.ConditionalCheckFailedException
	require.ErrorAs(t, err, &recovered)
	require.Same(t, exc, recovered)
}

func TestTransactionAmbiguousTransportErrorIsNotReclassified(t *testing.T) {
	transport := errors.New("dial tcp: connection refused (ConditionalCheckFailed inside the message)")

	err := (&Transaction{}).handleTransactionError(transport)
	require.ErrorIs(t, err, transport)
	require.NotErrorIs(t, err, theorydbErrors.ErrConditionFailed)
	require.NotErrorIs(t, err, theorydbErrors.ErrTransactionFailed)
	require.NotContains(t, err.Error(), "transaction canceled")
}
