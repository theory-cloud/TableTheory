package transaction

import (
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	theorydbErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/model"
	pkgTypes "github.com/theory-cloud/tabletheory/v3/pkg/types"
)

func TestBuilderRacedDeleteAndConditionCheckClassifiesConflict(t *testing.T) {
	registry := model.NewRegistry()
	require.NoError(t, registry.Register(&User{}))

	conflict := &types.TransactionCanceledException{
		CancellationReasons: []types.CancellationReason{
			{Code: aws.String("TransactionConflict"), Message: aws.String("raced transaction")},
		},
	}
	builder := NewBuilder(nil, registry, pkgTypes.NewConverter())
	// Keep every retry in the simulated race on the conflict path so Execute
	// returns the translated terminal error rather than succeeding on retry.
	builder.client = newMockTransactClient(t, conflict, conflict, conflict, conflict, conflict)

	err := builder.
		Delete(&User{ID: "target"}).
		ConditionCheck(&User{ID: "survivor"}, core.TransactCondition{Field: "ID", Operator: "=", Value: "survivor"}).
		Execute()

	require.Error(t, err)
	require.True(t, errors.Is(err, theorydbErrors.ErrTransactionConflict))
	require.True(t, errors.Is(err, theorydbErrors.ErrTransactionFailed))

	var transactionErr *theorydbErrors.TransactionError
	require.ErrorAs(t, err, &transactionErr)
	require.Equal(t, 0, transactionErr.OperationIndex)
	require.Equal(t, "Delete", transactionErr.Operation)
	require.Equal(t, "*transaction.User", transactionErr.Model)
}

func TestBuilderCancellationTaxonomyMatchesTransaction(t *testing.T) {
	testCases := []struct {
		code string
		want error
	}{
		{code: "ConditionalCheckFailed", want: theorydbErrors.ErrConditionFailed},
		{code: "TransactionConflict", want: theorydbErrors.ErrTransactionConflict},
		{code: "TransactionConflictException", want: theorydbErrors.ErrTransactionConflict},
		{code: "ThrottlingException", want: theorydbErrors.ErrThrottled},
		{code: "ThrottlingError", want: theorydbErrors.ErrThrottled},
		{code: "RequestLimitExceeded", want: theorydbErrors.ErrThrottled},
	}

	for _, testCase := range testCases {
		t.Run(testCase.code, func(t *testing.T) {
			canceled := &types.TransactionCanceledException{
				CancellationReasons: []types.CancellationReason{{Code: aws.String(testCase.code)}},
			}
			builder := &Builder{operations: []transactOperation{{typ: opDelete, model: &User{}}}}

			builderErr := builder.buildTransactionError(canceled, canceled)
			require.ErrorIs(t, builderErr, testCase.want)

			transactionErr := (&Transaction{}).handleTransactionError(canceled)
			require.ErrorIs(t, transactionErr, testCase.want)
		})
	}
}
