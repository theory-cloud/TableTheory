package transaction

import (
	theorydbErrors "github.com/theory-cloud/tabletheory/v6/pkg/errors"
)

const (
	// cancellationNoneCode is DynamoDB's placeholder for a cancellation reason
	// whose operation did not fail.
	cancellationNoneCode = "None"
	// unknownOperationName labels a transaction operation TableTheory cannot name.
	unknownOperationName = "unknown"
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
