package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/theory-cloud/tabletheory/internal/expr"
)

func main() {
	// Positive case: list index SET should be allowed and should not leak bracket syntax into ExpressionAttributeNames.
	{
		b := expr.NewBuilder()
		if err := b.AddUpdateSet("items[0]", "value"); err != nil {
			fmt.Fprintf(os.Stderr, "expression-hardening: unexpected error for list index SET\n")
			os.Exit(1)
		}

		components := b.Build()
		if !strings.Contains(components.UpdateExpression, "[0] = ") {
			fmt.Fprintf(os.Stderr, "expression-hardening: expected list index assignment in UpdateExpression\n")
			os.Exit(1)
		}
		for _, attrName := range components.ExpressionAttributeNames {
			if strings.Contains(attrName, "[") || strings.Contains(attrName, "]") {
				fmt.Fprintf(os.Stderr, "expression-hardening: ExpressionAttributeNames must not contain bracket syntax\n")
				os.Exit(1)
			}
		}
	}

	// Negative cases: injection or invalid indices must be rejected.
	{
		b := expr.NewBuilder()

		// Attempts to inject additional SET clauses via the list index position.
		if err := b.AddUpdateSet("items[0] = :v2, other = :v3, items[1]", "value"); err == nil {
			fmt.Fprintf(os.Stderr, "expression-hardening: expected injection attempt to be rejected (SET)\n")
			os.Exit(1)
		}

		// Negative index must be rejected.
		if err := b.AddUpdateSet("items[-1]", "value"); err == nil {
			fmt.Fprintf(os.Stderr, "expression-hardening: expected negative list index to be rejected (SET)\n")
			os.Exit(1)
		}

		// Attempts to inject additional REMOVE clauses via the list index position.
		if err := b.AddUpdateRemove("items[0], other]"); err == nil {
			fmt.Fprintf(os.Stderr, "expression-hardening: expected injection attempt to be rejected (REMOVE)\n")
			os.Exit(1)
		}
	}

	fmt.Println("expression-hardening: ok")
}
