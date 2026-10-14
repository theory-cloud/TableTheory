package errors

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// itemImageCause is a local stand-in for the AWS SDK
// *types.TransactionCanceledException shape: a message plus a cancellation
// reason list whose entries carry an item image (the Item attribute map).
// pkg/errors deliberately does not import the AWS SDK, so the probe mirrors the
// SDK field names instead of importing them.
type itemImageCause struct {
	Message             string
	CancellationReasons []itemImageReason
}

type itemImageReason struct {
	Item    map[string]any
	Code    string
	Message string
}

func (e *itemImageCause) Error() string { return "TransactionCanceledException: " + e.Message }

const itemImageSecret = "SECRET-item-image-9f8e"

func newItemImageTransactionError() *TransactionError {
	return &TransactionError{
		Err:            ErrTransactionFailed,
		Operation:      "update",
		Model:          "User",
		Reason:         "ConditionalCheckFailed",
		OperationIndex: 1,
		Reasons:        []CancellationReason{{Index: 1, Code: "ConditionalCheckFailed", Message: "cond"}},
		Cause: &itemImageCause{
			Message: "Transaction canceled",
			CancellationReasons: []itemImageReason{
				{Code: "ConditionalCheckFailed", Message: "cond", Item: map[string]any{"PK": itemImageSecret}},
			},
		},
	}
}

// TestTransactionErrorFormattingCarriesNoItemImage locks the docs/troubleshooting.md
// guarantee that the typed TransactionError surface never carries the item image
// held by the retained Cause: the reasons expose only Index/Code/Message, and
// Error()/fmt formatting render the error through Error() rather than reflecting
// into Cause. The item image lives only on the raw Cause, which a caller has to
// reach for deliberately (Cause field or errors.As); dumping that raw exception
// is what can leak it.
func TestTransactionErrorFormattingCarriesNoItemImage(t *testing.T) {
	txErr := newItemImageTransactionError()

	// The item image really is present on the retained cause, so the negative
	// assertions below are not vacuous.
	assert.Contains(t, fmt.Sprintf("%#v", txErr.Cause), itemImageSecret)

	// The typed surface — message, Error(), the reason list and fmt rendering of
	// the TransactionError — never includes it.
	assert.NotContains(t, txErr.Error(), itemImageSecret)
	assert.NotContains(t, txErr.Reason, itemImageSecret)
	assert.NotContains(t, fmt.Sprintf("%+v", txErr.Reasons), itemImageSecret)
	assert.NotContains(t, fmt.Sprintf("%+v", txErr.RealFailures()), itemImageSecret)
	for _, verb := range []string{"%v", "%+v", "%s", "%q", "%#v"} {
		assert.NotContains(t, fmt.Sprintf(verb, txErr), itemImageSecret, "verb %s", verb)
	}
}
