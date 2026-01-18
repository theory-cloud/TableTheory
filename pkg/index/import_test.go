package index

import (
	"testing"

	"github.com/theory-cloud/tabletheory/pkg/core"
)

// TestCoreImport verifies that the core package can be imported and used
func TestCoreImport(t *testing.T) {
	// Create a test IndexSchema
	schema := core.IndexSchema{
		Name:           "test-index",
		Type:           "GSI",
		PartitionKey:   "testPK",
		SortKey:        "testSK",
		ProjectionType: "ALL",
	}

	// Create a selector with the schema
	selector := NewSelector([]core.IndexSchema{schema})

	// Verify selector was created
	if selector == nil {
		t.Fatal("Failed to create selector")
	}

	// Verify we can access the indexes
	if len(selector.indexes) != 1 {
		t.Errorf("Expected 1 index, got %d", len(selector.indexes))
	}

	// Verify the index data
	if selector.indexes[0].Name != "test-index" {
		t.Errorf("Expected index name 'test-index', got %s", selector.indexes[0].Name)
	}
}
