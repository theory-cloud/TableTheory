// Package errors defines error types and utilities for TableTheory
package errors

import (
	"errors"
	"fmt"
)

// Common errors that can occur in TableTheory operations
var (
	// ErrItemNotFound is returned when an item is not found in the database
	ErrItemNotFound = errors.New("item not found")

	// ErrInvalidModel is returned when a model struct is invalid
	ErrInvalidModel = errors.New("invalid model")

	// ErrMissingPrimaryKey is returned when a model doesn't have a primary key
	ErrMissingPrimaryKey = errors.New("missing primary key")

	// ErrInvalidPrimaryKey is returned when a primary key value is invalid
	ErrInvalidPrimaryKey = errors.New("invalid primary key")

	// ErrConditionFailed is returned when a condition check fails
	ErrConditionFailed = errors.New("condition check failed")

	// ErrVersionConflict is returned when an optimistic-lock version condition fails.
	// It matches ErrConditionFailed via errors.Is for backward compatibility.
	ErrVersionConflict = conditionSubtypeError("version conflict")

	// ErrThrottled is returned when DynamoDB rejects an operation due to throttling.
	ErrThrottled = errors.New("throttled")

	// ErrTransactionConflict is returned when DynamoDB reports a transaction conflict.
	// It matches ErrTransactionFailed via errors.Is for backward compatibility.
	ErrTransactionConflict = transactionSubtypeError("transaction conflict")

	// ErrIndexNotFound is returned when a specified index doesn't exist
	ErrIndexNotFound = errors.New("index not found")

	// ErrTransactionFailed is returned when a transaction fails
	ErrTransactionFailed = errors.New("transaction failed")

	// ErrBatchOperationFailed is returned when a batch operation partially fails
	ErrBatchOperationFailed = errors.New("batch operation failed")

	// ErrUnsupportedType is returned when a field type is not supported
	ErrUnsupportedType = errors.New("unsupported type")

	// ErrInvalidTag is returned when a struct tag is invalid
	ErrInvalidTag = errors.New("invalid struct tag")

	// ErrTableNotFound is returned when a table doesn't exist
	ErrTableNotFound = errors.New("table not found")

	// ErrDuplicatePrimaryKey is returned when multiple primary keys are defined.
	// It matches ErrInvalidModel via errors.Is because duplicate keys are invalid model shapes.
	// Its declared error type is part of the published API contract.
	ErrDuplicatePrimaryKey error = invalidModelSubtypeError("duplicate primary key definition")

	// ErrEmptyValue is returned when a required value is empty
	ErrEmptyValue = errors.New("empty value")

	// ErrNoUpdatableFields is returned when an update selects no field the caller
	// can write, so there is no assignment to apply.
	ErrNoUpdatableFields = errors.New("no non-key fields to update")

	// ErrInvalidOperator is returned when an invalid query operator is used
	ErrInvalidOperator = errors.New("invalid query operator")

	// ErrEncryptionNotConfigured is returned when a model uses theorydb:"encrypted" fields but no KMS key ARN is configured.
	ErrEncryptionNotConfigured = errors.New("encryption not configured")

	// ErrInvalidEncryptedEnvelope is returned when an encrypted attribute value is not a valid TableTheory envelope.
	ErrInvalidEncryptedEnvelope = errors.New("invalid encrypted envelope")

	// ErrEncryptedFieldNotQueryable is returned when a theorydb:"encrypted" field is used in query/filter conditions.
	ErrEncryptedFieldNotQueryable = errors.New("encrypted fields are not queryable/filterable")

	// ErrImmutableModelMutation is returned when a write-once model is mutated through a generic high-level API.
	ErrImmutableModelMutation = errors.New("immutable model mutation")

	// ErrProtectedFieldMutation is returned when a protected model attribute is mutated through a generic high-level API.
	ErrProtectedFieldMutation = errors.New("protected field mutation")

	// ErrRejectedDeployAuthorityEvidence is returned when provenance/confidence evidence is not deploy-authoritative.
	ErrRejectedDeployAuthorityEvidence = errors.New("rejected deploy authority evidence")
)

type conditionSubtypeError string

func (e conditionSubtypeError) Error() string {
	return string(e)
}

func (e conditionSubtypeError) Is(target error) bool {
	return target == ErrConditionFailed
}

type invalidModelSubtypeError string

func (e invalidModelSubtypeError) Error() string {
	return string(e)
}

func (e invalidModelSubtypeError) Is(target error) bool {
	return target == ErrInvalidModel
}

type transactionSubtypeError string

func (e transactionSubtypeError) Error() string {
	return string(e)
}

func (e transactionSubtypeError) Is(target error) bool {
	return target == ErrTransactionFailed
}

// EncryptedFieldError wraps failures related to theorydb:"encrypted" fields (encryption/decryption).
// It is safe-by-default: the error string must never include decrypted plaintext.
type EncryptedFieldError struct {
	Err       error
	Field     string
	Operation string
}

func (e *EncryptedFieldError) Error() string {
	if e == nil {
		return "theorydb: encrypted field error"
	}

	op := e.Operation
	if op == "" {
		op = "operation"
	}

	field := e.Field
	if field == "" {
		field = "field"
	}

	if e.Err == nil {
		return fmt.Sprintf("theorydb: encrypted %s failed for %s", op, field)
	}
	return fmt.Sprintf("theorydb: encrypted %s failed for %s: %v", op, field, e.Err)
}

func (e *EncryptedFieldError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// TheorydbError represents a detailed error with context
type TheorydbError struct {
	Err     error
	Context map[string]any
	Op      string
	Model   string
}

// Error implements the error interface
func (e *TheorydbError) Error() string {
	// SECURITY: Don't expose model names or context data in error messages
	// Only return the operation and underlying error for secure logging
	return fmt.Sprintf("theorydb: %s operation failed: %v", e.Op, e.Err)
}

// Unwrap returns the underlying error
func (e *TheorydbError) Unwrap() error {
	return e.Err
}

// Is checks if the error matches the target error
func (e *TheorydbError) Is(target error) bool {
	return errors.Is(e.Err, target)
}

// NewError creates a new TheorydbError
func NewError(op, model string, err error) *TheorydbError {
	return &TheorydbError{
		Op:    op,
		Model: model,
		Err:   err,
	}
}

// NewErrorWithContext creates a new TheorydbError with context
func NewErrorWithContext(op, model string, err error, context map[string]any) *TheorydbError {
	return &TheorydbError{
		Op:      op,
		Model:   model,
		Err:     err,
		Context: context,
	}
}

// IsNotFound checks if an error indicates an item was not found
func IsNotFound(err error) bool {
	return errors.Is(err, ErrItemNotFound)
}

// IsInvalidModel checks if an error indicates an invalid model
func IsInvalidModel(err error) bool {
	return errors.Is(err, ErrInvalidModel)
}

// IsConditionFailed checks if an error indicates a condition check failure
func IsConditionFailed(err error) bool {
	return errors.Is(err, ErrConditionFailed)
}

// CancellationReason captures a single entry of a DynamoDB transaction
// cancellation reason list in its original position.
//
// It deliberately carries only the classification code and the service message:
// the DynamoDB item image (the cancellation reason's Item attribute map) is never
// copied here, so reasons are always safe to log. Entries whose code is missing
// or the "None" placeholder keep their position with an empty Code so Index stays
// aligned with the transaction operations.
type CancellationReason struct {
	Code    string
	Message string
	Index   int
}

// TransactionError provides context for transactional failures.
type TransactionError struct {
	Err error

	// Cause is the original error that produced the transaction failure. For a
	// DynamoDB transaction cancellation it is the SDK's
	// *types.TransactionCanceledException (wrapped errors preserved), so callers
	// can recover the exception and its ordered CancellationReasons with
	// errors.As. Error() never consults Cause, so logging stays free of item
	// images; a caller that inspects Cause directly owns that decision.
	Cause error

	Operation string
	Model     string
	Reason    string

	// Reasons carries every entry of the DynamoDB cancellation reason list in
	// its original order, including entries with a missing or "None" code, so
	// positions stay aligned with the transaction operations. Use RealFailures
	// to iterate only the genuine failures.
	Reasons []CancellationReason

	// OperationIndex is the position of the first genuine per-operation failure.
	// It is -1 for a cancellation with no genuine per-operation failure — an empty
	// reason list, or every reason missing its code or carrying the "None"
	// placeholder. That deliberate marker keeps the case distinct from a real
	// failure at operation 0; see
	// TestTransactionCancellationAllNoneIsGenericNotConditional in pkg/transaction.
	OperationIndex int
}

// Error implements the error interface.
func (e *TransactionError) Error() string {
	if e == nil {
		return "theorydb: transaction failed"
	}

	op := "transaction"
	if e.Operation != "" {
		op = fmt.Sprintf("%s operation %s", op, e.Operation)
	}
	if e.OperationIndex >= 0 {
		op = fmt.Sprintf("%s (index %d)", op, e.OperationIndex)
	}
	if e.Reason != "" {
		return fmt.Sprintf("theorydb: %s failed: %s", op, e.Reason)
	}
	return fmt.Sprintf("theorydb: %s failed", op)
}

// Unwrap returns the underlying classified error. It stays a single-error
// Unwrap (not the []error form) so errors.Unwrap keeps returning the sentinel
// for existing callers.
func (e *TransactionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// As lets errors.As recover the original failure stored in Cause and therefore
// the AWS exception (with its ordered CancellationReasons). It delegates to
// errors.As on Cause and guards nil and a self-referential cause so it can never
// recurse into itself. errors.Is sentinel matching is unaffected.
func (e *TransactionError) As(target any) bool {
	if e == nil || e.Cause == nil {
		return false
	}
	if cause, ok := e.Cause.(*TransactionError); ok && cause == e {
		return false
	}
	return errors.As(e.Cause, target)
}

// RealFailures returns the cancellation reasons that represent genuine
// failures, in original order: every entry with a non-empty code that is not
// DynamoDB's "None" placeholder. The returned slice may be empty.
func (e *TransactionError) RealFailures() []CancellationReason {
	if e == nil {
		return nil
	}
	failures := make([]CancellationReason, 0, len(e.Reasons))
	for _, reason := range e.Reasons {
		if reason.Code == "" || reason.Code == "None" {
			continue
		}
		failures = append(failures, reason)
	}
	return failures
}
