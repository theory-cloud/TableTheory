package transaction

import (
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	theorydbErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
)

// classifyTransactionCancellationCode keeps the Transaction and Builder paths
// aligned on DynamoDB's cancellation taxonomy.
func classifyTransactionCancellationCode(code string) error {
	switch code {
	case "ConditionalCheckFailed", "ConditionalCheckFailedException":
		return theorydbErrors.ErrConditionFailed
	case "TransactionConflict", "TransactionConflictException":
		return theorydbErrors.ErrTransactionConflict
	case "ProvisionedThroughputExceeded", "ProvisionedThroughputExceededException",
		"ThrottlingException", "ThrottlingError", "RequestLimitExceeded",
		"InternalServerError", "ServiceUnavailable":
		return theorydbErrors.ErrThrottled
	default:
		return nil
	}
}

func transactionCanceledHasClassification(exc *types.TransactionCanceledException, classification error) bool {
	if exc == nil {
		return false
	}
	for _, reason := range exc.CancellationReasons {
		if reason.Code == nil {
			continue
		}
		if classifyTransactionCancellationCode(*reason.Code) == classification {
			return true
		}
	}
	return false
}
