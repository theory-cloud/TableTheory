package errors

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestErrorTypes tests all predefined error variables
func TestErrorTypes(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "ErrItemNotFound",
			err:      ErrItemNotFound,
			expected: "item not found",
		},
		{
			name:     "ErrInvalidModel",
			err:      ErrInvalidModel,
			expected: "invalid model",
		},
		{
			name:     "ErrMissingPrimaryKey",
			err:      ErrMissingPrimaryKey,
			expected: "missing primary key",
		},
		{
			name:     "ErrInvalidPrimaryKey",
			err:      ErrInvalidPrimaryKey,
			expected: "invalid primary key",
		},
		{
			name:     "ErrConditionFailed",
			err:      ErrConditionFailed,
			expected: "condition check failed",
		},
		{
			name:     "ErrIndexNotFound",
			err:      ErrIndexNotFound,
			expected: "index not found",
		},
		{
			name:     "ErrTransactionFailed",
			err:      ErrTransactionFailed,
			expected: "transaction failed",
		},
		{
			name:     "ErrBatchOperationFailed",
			err:      ErrBatchOperationFailed,
			expected: "batch operation failed",
		},
		{
			name:     "ErrUnsupportedType",
			err:      ErrUnsupportedType,
			expected: "unsupported type",
		},
		{
			name:     "ErrInvalidTag",
			err:      ErrInvalidTag,
			expected: "invalid struct tag",
		},
		{
			name:     "ErrTableNotFound",
			err:      ErrTableNotFound,
			expected: "table not found",
		},
		{
			name:     "ErrDuplicatePrimaryKey",
			err:      ErrDuplicatePrimaryKey,
			expected: "duplicate primary key definition",
		},
		{
			name:     "ErrEmptyValue",
			err:      ErrEmptyValue,
			expected: "empty value",
		},
		{
			name:     "ErrInvalidOperator",
			err:      ErrInvalidOperator,
			expected: "invalid query operator",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Error(t, tt.err)
			assert.Equal(t, tt.expected, tt.err.Error())
		})
	}
}

// TestTheorydbError_Error tests the Error method of TheorydbError
func TestTheorydbError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *TheorydbError
		expected string
	}{
		{
			name: "without context",
			err: &TheorydbError{
				Op:    "GetItem",
				Model: "User",
				Err:   ErrItemNotFound,
			},
			expected: "theorydb: GetItem operation failed: item not found",
		},
		{
			name: "with empty context",
			err: &TheorydbError{
				Op:      "UpdateItem",
				Model:   "Product",
				Err:     ErrConditionFailed,
				Context: map[string]any{},
			},
			expected: "theorydb: UpdateItem operation failed: condition check failed",
		},
		{
			name: "with context",
			err: &TheorydbError{
				Op:    "PutItem",
				Model: "Order",
				Err:   ErrInvalidModel,
				Context: map[string]any{
					"id":     "123",
					"status": "pending",
				},
			},
			expected: "theorydb: PutItem operation failed: invalid model",
		},
		{
			name: "with nil error",
			err: &TheorydbError{
				Op:    "DeleteItem",
				Model: "Session",
				Err:   nil,
			},
			expected: "theorydb: DeleteItem operation failed: <nil>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Contains(t, tt.err.Error(), tt.expected)
		})
	}
}

// TestTheorydbError_Unwrap tests the Unwrap method
func TestTheorydbError_Unwrap(t *testing.T) {
	baseErr := ErrItemNotFound
	dErr := &TheorydbError{
		Op:    "GetItem",
		Model: "User",
		Err:   baseErr,
	}

	unwrapped := dErr.Unwrap()
	assert.Equal(t, baseErr, unwrapped)

	// Test with nil error
	dErrNil := &TheorydbError{
		Op:    "GetItem",
		Model: "User",
		Err:   nil,
	}
	assert.Nil(t, dErrNil.Unwrap())
}

// TestTheorydbError_Is tests the Is method
func TestTheorydbError_Is(t *testing.T) {
	tests := []struct {
		target error
		err    *TheorydbError
		name   string
		want   bool
	}{
		{
			name: "matches underlying error",
			err: &TheorydbError{
				Op:    "GetItem",
				Model: "User",
				Err:   ErrItemNotFound,
			},
			target: ErrItemNotFound,
			want:   true,
		},
		{
			name: "doesn't match different error",
			err: &TheorydbError{
				Op:    "GetItem",
				Model: "User",
				Err:   ErrItemNotFound,
			},
			target: ErrInvalidModel,
			want:   false,
		},
		{
			name: "matches wrapped error",
			err: &TheorydbError{
				Op:    "GetItem",
				Model: "User",
				Err:   fmt.Errorf("wrapped: %w", ErrConditionFailed),
			},
			target: ErrConditionFailed,
			want:   true,
		},
		{
			name: "nil underlying error",
			err: &TheorydbError{
				Op:    "GetItem",
				Model: "User",
				Err:   nil,
			},
			target: ErrItemNotFound,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.err.Is(tt.target))
		})
	}
}

// TestNewError tests the NewError constructor
func TestNewError(t *testing.T) {
	op := "CreateTable"
	model := "Product"
	baseErr := ErrTableNotFound

	err := NewError(op, model, baseErr)

	require.NotNil(t, err)
	assert.Equal(t, op, err.Op)
	assert.Equal(t, model, err.Model)
	assert.Equal(t, baseErr, err.Err)
	assert.Nil(t, err.Context)
}

// TestNewErrorWithContext tests the NewErrorWithContext constructor
func TestNewErrorWithContext(t *testing.T) {
	op := "UpdateItem"
	model := "Order"
	baseErr := ErrConditionFailed
	context := map[string]any{
		"orderId": "12345",
		"userId":  "67890",
		"amount":  99.99,
	}

	err := NewErrorWithContext(op, model, baseErr, context)

	require.NotNil(t, err)
	assert.Equal(t, op, err.Op)
	assert.Equal(t, model, err.Model)
	assert.Equal(t, baseErr, err.Err)
	assert.Equal(t, context, err.Context)
}

// TestIsNotFound tests the IsNotFound helper function
func TestIsNotFound(t *testing.T) {
	tests := []struct {
		err  error
		name string
		want bool
	}{
		{
			name: "direct ErrItemNotFound",
			err:  ErrItemNotFound,
			want: true,
		},
		{
			name: "wrapped ErrItemNotFound",
			err:  fmt.Errorf("operation failed: %w", ErrItemNotFound),
			want: true,
		},
		{
			name: "TheorydbError with ErrItemNotFound",
			err:  NewError("GetItem", "User", ErrItemNotFound),
			want: true,
		},
		{
			name: "different error",
			err:  ErrInvalidModel,
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "custom error",
			err:  errors.New("custom error"),
			want: false,
		},
		{
			name: "deeply wrapped ErrItemNotFound",
			err: fmt.Errorf("layer1: %w",
				fmt.Errorf("layer2: %w",
					NewError("GetItem", "User", ErrItemNotFound))),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsNotFound(tt.err))
		})
	}
}

// TestIsInvalidModel tests the IsInvalidModel helper function
func TestIsInvalidModel(t *testing.T) {
	tests := []struct {
		err  error
		name string
		want bool
	}{
		{
			name: "direct ErrInvalidModel",
			err:  ErrInvalidModel,
			want: true,
		},
		{
			name: "wrapped ErrInvalidModel",
			err:  fmt.Errorf("validation failed: %w", ErrInvalidModel),
			want: true,
		},
		{
			name: "TheorydbError with ErrInvalidModel",
			err:  NewError("ValidateModel", "Product", ErrInvalidModel),
			want: true,
		},
		{
			name: "different error",
			err:  ErrItemNotFound,
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "TheorydbError with context and ErrInvalidModel",
			err: NewErrorWithContext("ValidateModel", "Order", ErrInvalidModel,
				map[string]any{"field": "price"}),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsInvalidModel(tt.err))
		})
	}
}

// TestIsConditionFailed tests the IsConditionFailed helper function
func TestIsConditionFailed(t *testing.T) {
	tests := []struct {
		err  error
		name string
		want bool
	}{
		{
			name: "direct ErrConditionFailed",
			err:  ErrConditionFailed,
			want: true,
		},
		{
			name: "wrapped ErrConditionFailed",
			err:  fmt.Errorf("update failed: %w", ErrConditionFailed),
			want: true,
		},
		{
			name: "TheorydbError with ErrConditionFailed",
			err:  NewError("UpdateItem", "User", ErrConditionFailed),
			want: true,
		},
		{
			name: "different error",
			err:  ErrInvalidOperator,
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "nested TheorydbError with ErrConditionFailed",
			err: NewError("Transaction", "Multi",
				NewError("UpdateItem", "User", ErrConditionFailed)),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsConditionFailed(tt.err))
		})
	}
}

// TestErrorWrapping tests complex error wrapping scenarios
func TestErrorWrapping(t *testing.T) {
	// Test multiple levels of wrapping
	baseErr := ErrItemNotFound
	wrapped1 := NewError("GetItem", "User", baseErr)
	wrapped2 := fmt.Errorf("database operation failed: %w", wrapped1)
	wrapped3 := NewErrorWithContext("FetchUser", "UserService", wrapped2,
		map[string]any{"userId": "123"})

	// All levels should detect the base error
	assert.True(t, errors.Is(wrapped3, baseErr))
	assert.True(t, IsNotFound(wrapped3))

	// The outermost error should have proper error message
	errMsg := wrapped3.Error()
	assert.Contains(t, errMsg, "FetchUser")
	assert.Contains(t, errMsg, "operation failed")
}

// TestErrorChaining tests error chain behavior
func TestErrorChaining(t *testing.T) {
	// Create a chain of errors
	err1 := ErrInvalidPrimaryKey
	err2 := NewError("ValidateKey", "Model", err1)
	err3 := fmt.Errorf("validation error: %w", err2)
	err4 := NewErrorWithContext("SaveItem", "User", err3,
		map[string]any{"action": "create"})

	// Test unwrapping chain
	assert.Equal(t, err3, err4.Unwrap())
	assert.True(t, errors.Is(err4, err1))

	// Test error message contains operation
	errMsg := err4.Error()
	assert.Contains(t, errMsg, "SaveItem")
	assert.Contains(t, errMsg, "operation failed")
}

func TestTransactionError_ErrorAndUnwrap(t *testing.T) {
	t.Run("nil receiver", func(t *testing.T) {
		var txErr *TransactionError
		assert.Equal(t, "theorydb: transaction failed", txErr.Error())
		assert.Nil(t, txErr.Unwrap())
	})

	t.Run("includes operation, index, and reason", func(t *testing.T) {
		baseErr := errors.New("boom")
		txErr := &TransactionError{
			Err:            baseErr,
			Operation:      "update",
			Reason:         "ConditionalCheckFailed",
			OperationIndex: 3,
		}

		assert.Contains(t, txErr.Error(), "transaction operation update (index 3) failed: ConditionalCheckFailed")
		assert.ErrorIs(t, txErr.Unwrap(), baseErr)
	})

	t.Run("omits index when negative and omits reason when empty", func(t *testing.T) {
		txErr := &TransactionError{
			Operation:      "delete",
			OperationIndex: -1,
		}

		assert.Equal(t, "theorydb: transaction operation delete failed", txErr.Error())
	})
}

// TestConcurrentErrorAccess tests thread safety of error operations
func TestConcurrentErrorAccess(t *testing.T) {
	err := NewErrorWithContext("ConcurrentOp", "TestModel", ErrTransactionFailed,
		map[string]any{"test": "concurrent"})

	// Run concurrent reads
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_ = err.Error()
			unwrapped := err.Unwrap()
			if unwrapped != nil {
				_ = unwrapped.Error()
			}
			_ = err.Is(ErrTransactionFailed)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

// BenchmarkErrorCreation benchmarks error creation performance
func BenchmarkErrorCreation(b *testing.B) {
	b.Run("NewError", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = NewError("Operation", "Model", ErrItemNotFound).Error()
		}
	})

	b.Run("NewErrorWithContext", func(b *testing.B) {
		ctx := map[string]any{"key": "value", "count": 42}
		for i := 0; i < b.N; i++ {
			_ = NewErrorWithContext("Operation", "Model", ErrItemNotFound, ctx).Error()
		}
	})
}

// BenchmarkErrorChecking benchmarks error checking performance
func BenchmarkErrorChecking(b *testing.B) {
	// Create various error types
	directErr := ErrItemNotFound
	wrappedErr := fmt.Errorf("wrapped: %w", ErrItemNotFound)
	dynamErr := NewError("Op", "Model", ErrItemNotFound)

	b.Run("IsNotFound-Direct", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = IsNotFound(directErr)
		}
	})

	b.Run("IsNotFound-Wrapped", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = IsNotFound(wrappedErr)
		}
	})

	b.Run("IsNotFound-TableTheory", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = IsNotFound(dynamErr)
		}
	})
}
