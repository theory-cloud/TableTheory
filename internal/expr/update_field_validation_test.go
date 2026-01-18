package expr_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/internal/expr"
)

func TestUpdateExpression_FieldValidation(t *testing.T) {
	builder := expr.NewBuilder()

	require.Error(t, builder.AddUpdateSet("bad-field", "value"))
	require.Error(t, builder.AddUpdateAdd("bad-field", 1))
	require.Error(t, builder.AddUpdateDelete("bad-field", []string{"value"}))
	require.Error(t, builder.AddUpdateRemove("bad-field"))
}
