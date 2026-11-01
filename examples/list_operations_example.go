// Package examples demonstrates list operations with TableTheory UpdateBuilder
package examples

import (
	"fmt"

	"github.com/theory-cloud/tabletheory/v6/pkg/core"
)

// Product represents a product with list and set attributes.
type Product struct {
	ID          string `theorydb:"pk"`
	Name        string
	Description string `theorydb:"omitempty"`
	Tags        []string
	Categories  []string
	Labels      []string `theorydb:"set"`
	ViewCount   int
}

// TableName keeps this example's table distinct when examples share one local
// DynamoDB instance.
func (Product) TableName() string { return "example_list_operation_products" }

// DemonstrateListOperations shows how to use list append/prepend operations
// against an existing product row.
func DemonstrateListOperations(db core.ExtendedDB) error {
	// Example 1: Append to a list
	// This will generate: SET Tags = list_append(Tags, :v1)
	if err := db.Model(&Product{}).
		Where("ID", "=", "prod123").
		UpdateBuilder().
		AppendToList("Tags", []string{"new-tag", "another-tag"}).
		Execute(); err != nil {
		return fmt.Errorf("append tags: %w", err)
	}

	// Example 2: Prepend to a list
	// This will generate: SET Tags = list_append(:v1, Tags)
	if err := db.Model(&Product{}).
		Where("ID", "=", "prod123").
		UpdateBuilder().
		PrependToList("Tags", []string{"featured", "sale"}).
		Execute(); err != nil {
		return fmt.Errorf("prepend tags: %w", err)
	}

	// Example 3: Combined operations
	if err := db.Model(&Product{}).
		Where("ID", "=", "prod123").
		UpdateBuilder().
		Set("Name", "Updated Product Name").
		AppendToList("Tags", []string{"updated"}).
		PrependToList("Categories", []string{"electronics"}).
		SetIfNotExists("Description", nil, "Default product description").
		Increment("ViewCount").
		Execute(); err != nil {
		return fmt.Errorf("combined update: %w", err)
	}

	// Example 4: Remove from a set (using the DELETE action). DELETE applies to
	// set attributes, so this uses Labels rather than the Tags list.
	if err := db.Model(&Product{}).
		Where("ID", "=", "prod123").
		UpdateBuilder().
		Delete("Labels", []string{"obsolete-tag"}).
		Execute(); err != nil {
		return fmt.Errorf("delete from set: %w", err)
	}

	// Example 5: List element operations
	if err := db.Model(&Product{}).
		Where("ID", "=", "prod123").
		UpdateBuilder().
		SetListElement("Tags", 0, "first-tag").
		RemoveFromListAt("Tags", 5).
		Execute(); err != nil {
		return fmt.Errorf("update list elements: %w", err)
	}

	fmt.Println("List operations demonstrated successfully!")
	return nil
}

// The generated DynamoDB UpdateExpression will look like:
// SET #Tags = list_append(#Tags, :v1), #Name = :v2
// With proper placeholders for reserved words and values
