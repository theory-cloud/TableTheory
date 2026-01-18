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

	// ErrDuplicatePrimaryKey is returned when multiple primary keys are defined
	ErrDuplicatePrimaryKey = errors.New("duplicate primary key definition")

	// ErrEmptyValue is returned when a required value is empty
	ErrEmptyValue = errors.New("empty value")

	// ErrInvalidOperator is returned when an invalid query operator is used
	ErrInvalidOperator = errors.New("invalid query operator")

	// ErrEncryptionNotConfigured is returned when a model uses theorydb:"encrypted" fields but no KMS key ARN is configured.
	ErrEncryptionNotConfigured = errors.New("encryption not configured")

	// ErrInvalidEncryptedEnvelope is returned when an encrypted attribute value is not a valid TableTheory envelope.
	ErrInvalidEncryptedEnvelope = errors.New("invalid encrypted envelope")

	// ErrEncryptedFieldNotQueryable is returned when a theorydb:"encrypted" field is used in query/filter conditions.
	ErrEncryptedFieldNotQueryable = errors.New("encrypted fields are not queryable/filterable")
)

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

// TransactionError provides context for transactional failures.
type TransactionError struct {
	Err            error
	Operation      string
	Model          string
	Reason         string
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

// Unwrap returns the underlying error.
func (e *TransactionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
