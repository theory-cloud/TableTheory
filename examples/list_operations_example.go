// Package examples demonstrates list operations with TableTheory UpdateBuilder
package examples

import (
	"fmt"
	"log"

	"github.com/theory-cloud/tabletheory"
)

// Product represents a product with tags
type Product struct {
	ID          string `theorydb:"pk"`
	Name        string
	Description string
	Tags        []string
	Categories  []string
}

// DemonstrateListOperations shows how to use list append/prepend operations
func DemonstrateListOperations() {
	// Initialize TableTheory
	db, err := theorydb.New(theorydb.Config{
		Region: "us-east-1",
	})
	if err != nil {
		log.Fatal(err)
	}

	// Example 1: Append to a list
	// This will generate: SET Tags = list_append(Tags, :v1)
	err = db.Model(&Product{}).
		Where("ID", "=", "prod123").
		UpdateBuilder().
		AppendToList("Tags", []string{"new-tag", "another-tag"}).
		Execute()
	if err != nil {
		log.Printf("Failed to append tags: %v", err)
	}

	// Example 2: Prepend to a list
	// This will generate: SET Tags = list_append(:v1, Tags)
	err = db.Model(&Product{}).
		Where("ID", "=", "prod123").
		UpdateBuilder().
		PrependToList("Tags", []string{"featured", "sale"}).
		Execute()
	if err != nil {
		log.Printf("Failed to prepend tags: %v", err)
	}

	// Example 3: Combined operations
	err = db.Model(&Product{}).
		Where("ID", "=", "prod123").
		UpdateBuilder().
		Set("Name", "Updated Product Name").
		AppendToList("Tags", []string{"updated"}).
		PrependToList("Categories", []string{"electronics"}).
		SetIfNotExists("Description", nil, "Default product description").
		Increment("ViewCount").
		Execute()
	if err != nil {
		log.Printf("Failed to perform combined update: %v", err)
	}

	// Example 4: Remove from set (using DELETE action)
	// This is for set types, not lists
	err = db.Model(&Product{}).
		Where("ID", "=", "prod123").
		UpdateBuilder().
		Delete("Tags", []string{"obsolete-tag"}). // Removes specific values from a set
		Execute()
	if err != nil {
		log.Printf("Failed to delete from set: %v", err)
	}

	// Example 5: List element operations
	err = db.Model(&Product{}).
		Where("ID", "=", "prod123").
		UpdateBuilder().
		SetListElement("Tags", 0, "first-tag"). // Set first element
		RemoveFromListAt("Tags", 5).            // Remove element at index 5
		Execute()
	if err != nil {
		log.Printf("Failed to update list elements: %v", err)
	}

	fmt.Println("List operations demonstrated successfully!")
}

// The generated DynamoDB UpdateExpression will look like:
// SET #Tags = list_append(#Tags, :v1), #Name = :v2
// With proper placeholders for reserved words and values
