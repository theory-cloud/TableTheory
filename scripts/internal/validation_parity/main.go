package main

import (
	"fmt"
	"os"

	"github.com/theory-cloud/tabletheory/internal/expr"
	"github.com/theory-cloud/tabletheory/pkg/validation"
)

func main() {
	// Representative inputs that have historically been accepted by validators but can trigger conversion issues.
	// This list is intentionally small; fuzzing is handled by a separate verifier.
	cases := []struct { //nolint:govet // fieldalignment is irrelevant for a tiny, single-run harness
		name  string
		field string
		op    string
		val   any
	}{
		{name: "simple string", field: "Name", op: "=", val: "alice"},
		{name: "int", field: "Count", op: "=", val: 1},
		{name: "slice strings", field: "Tags", op: "IN", val: []string{"a", "b"}},
		{name: "map string->any", field: "Meta", op: "=", val: map[string]any{"k": "v"}},
		{name: "typed map int keys", field: "Meta", op: "=", val: map[int]string{1: "one"}},
		{name: "struct allowed by validation", field: "Payload", op: "=", val: struct{ A string }{A: "x"}},
	}

	b := expr.NewBuilder()
	for _, tc := range cases {
		if err := validation.ValidateFieldName(tc.field); err != nil {
			fmt.Fprintf(os.Stderr, "parity: unexpected field validation failure for %s: %v\n", tc.name, err)
			os.Exit(1)
		}
		if err := validation.ValidateOperator(tc.op); err != nil {
			fmt.Fprintf(os.Stderr, "parity: unexpected operator validation failure for %s: %v\n", tc.name, err)
			os.Exit(1)
		}
		if err := validation.ValidateValue(tc.val); err != nil {
			fmt.Fprintf(os.Stderr, "parity: unexpected value validation failure for %s: %v\n", tc.name, err)
			os.Exit(1)
		}

		// If this panics, we want the verifier to fail the rubric surface.
		if err := b.AddFilterCondition("AND", tc.field, tc.op, tc.val); err != nil {
			// Errors are acceptable; panics are not. Keep this visible for debugging.
			fmt.Fprintf(os.Stderr, "parity: builder returned error for %s: %v\n", tc.name, err)
		}
	}

	fmt.Println("validation-parity: ok (no panics in harness)")
}
