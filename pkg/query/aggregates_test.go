package query

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/core"
)

// Test model for aggregates
type AggregateTestItem struct {
	ID       string  `dynamodb:"id"`
	Category string  `dynamodb:"category"`
	Status   string  `dynamodb:"status"`
	Price    float64 `dynamodb:"price"`
	Quantity int     `dynamodb:"quantity"`
}

// Mock executor for aggregate tests
type mockAggregateExecutor struct {
	err   error
	items []any
}

func (m *mockAggregateExecutor) ExecuteQuery(input *core.CompiledQuery, dest any) error {
	if m.err != nil {
		return m.err
	}

	// Use reflection to populate dest with test items
	destValue := reflect.ValueOf(dest).Elem()
	itemsValue := reflect.ValueOf(m.items)

	// Create a new slice of the correct type
	newSlice := reflect.MakeSlice(destValue.Type(), itemsValue.Len(), itemsValue.Len())

	// Copy each item
	for i := 0; i < itemsValue.Len(); i++ {
		newSlice.Index(i).Set(reflect.ValueOf(itemsValue.Index(i).Interface()))
	}

	destValue.Set(newSlice)
	return nil
}

func (m *mockAggregateExecutor) ExecuteScan(input *core.CompiledQuery, dest any) error {
	return m.ExecuteQuery(input, dest)
}

func TestSum(t *testing.T) {
	tests := []struct {
		name     string
		field    string
		items    []any
		expected float64
		wantErr  bool
	}{
		{
			name: "sum of prices",
			items: []any{
				AggregateTestItem{ID: "1", Price: 10.5, Quantity: 2},
				AggregateTestItem{ID: "2", Price: 20.0, Quantity: 1},
				AggregateTestItem{ID: "3", Price: 15.25, Quantity: 3},
			},
			field:    "Price",
			expected: 45.75,
		},
		{
			name: "sum of quantities",
			items: []any{
				AggregateTestItem{ID: "1", Price: 10.5, Quantity: 2},
				AggregateTestItem{ID: "2", Price: 20.0, Quantity: 1},
				AggregateTestItem{ID: "3", Price: 15.25, Quantity: 3},
			},
			field:    "Quantity",
			expected: 6.0,
		},
		{
			name:     "empty result set",
			items:    []any{},
			field:    "Price",
			expected: 0,
		},
		{
			name: "non-numeric field",
			items: []any{
				AggregateTestItem{ID: "1", Status: "active"},
			},
			field:    "Status",
			expected: 0, // Should skip non-numeric values
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := &Query{
				model:    AggregateTestItem{},
				metadata: &TestMetadata{},
				executor: &mockAggregateExecutor{items: tt.items},
				ctx:      context.Background(),
			}

			result, err := q.Sum(tt.field)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.InDelta(t, tt.expected, result, 0.001)
			}
		})
	}
}

func TestAverage(t *testing.T) {
	tests := []struct {
		name     string
		field    string
		items    []any
		expected float64
		wantErr  bool
	}{
		{
			name: "average of prices",
			items: []any{
				AggregateTestItem{ID: "1", Price: 10.0},
				AggregateTestItem{ID: "2", Price: 20.0},
				AggregateTestItem{ID: "3", Price: 30.0},
			},
			field:    "Price",
			expected: 20.0,
		},
		{
			name:     "empty result set",
			items:    []any{},
			field:    "Price",
			expected: 0,
		},
		{
			name: "mixed valid and invalid values",
			items: []any{
				AggregateTestItem{ID: "1", Price: 10.0},
				AggregateTestItem{ID: "2", Price: 20.0},
				AggregateTestItem{ID: "3", Status: "active"}, // No price (zero value)
			},
			field:    "Price",
			expected: 10.0, // Average of 10, 20, and 0 (zero value counted)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := &Query{
				model:    AggregateTestItem{},
				metadata: &TestMetadata{},
				executor: &mockAggregateExecutor{items: tt.items},
				ctx:      context.Background(),
			}

			result, err := q.Average(tt.field)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.InDelta(t, tt.expected, result, 0.001)
			}
		})
	}
}

func TestMin(t *testing.T) {
	tests := []struct {
		expected any
		name     string
		field    string
		errMsg   string
		items    []any
		wantErr  bool
	}{
		{
			name: "min of numeric field",
			items: []any{
				AggregateTestItem{ID: "1", Price: 30.0},
				AggregateTestItem{ID: "2", Price: 10.0},
				AggregateTestItem{ID: "3", Price: 20.0},
			},
			field:    "Price",
			expected: 10.0,
		},
		{
			name: "min of string field",
			items: []any{
				AggregateTestItem{ID: "1", Status: "pending"},
				AggregateTestItem{ID: "2", Status: "active"},
				AggregateTestItem{ID: "3", Status: "completed"},
			},
			field:    "Status",
			expected: "active",
		},
		{
			name:    "empty result set",
			items:   []any{},
			field:   "Price",
			wantErr: true,
			errMsg:  "no items found",
		},
		{
			name: "all nil values",
			items: []any{
				AggregateTestItem{ID: "1"},
				AggregateTestItem{ID: "2"},
			},
			field:   "Price",
			wantErr: true,
			errMsg:  "no valid values found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := &Query{
				model:    AggregateTestItem{},
				metadata: &TestMetadata{},
				executor: &mockAggregateExecutor{items: tt.items},
				ctx:      context.Background(),
			}

			result, err := q.Min(tt.field)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestMax(t *testing.T) {
	tests := []struct {
		expected any
		name     string
		field    string
		errMsg   string
		items    []any
		wantErr  bool
	}{
		{
			name: "max of numeric field",
			items: []any{
				AggregateTestItem{ID: "1", Price: 30.0},
				AggregateTestItem{ID: "2", Price: 10.0},
				AggregateTestItem{ID: "3", Price: 20.0},
			},
			field:    "Price",
			expected: 30.0,
		},
		{
			name: "max of string field",
			items: []any{
				AggregateTestItem{ID: "1", Status: "pending"},
				AggregateTestItem{ID: "2", Status: "active"},
				AggregateTestItem{ID: "3", Status: "completed"},
			},
			field:    "Status",
			expected: "pending",
		},
		{
			name:    "empty result set",
			items:   []any{},
			field:   "Price",
			wantErr: true,
			errMsg:  "no items found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := &Query{
				model:    AggregateTestItem{},
				metadata: &TestMetadata{},
				executor: &mockAggregateExecutor{items: tt.items},
				ctx:      context.Background(),
			}

			result, err := q.Max(tt.field)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestAggregate(t *testing.T) {
	items := []any{
		AggregateTestItem{ID: "1", Price: 10.0, Quantity: 2},
		AggregateTestItem{ID: "2", Price: 20.0, Quantity: 1},
		AggregateTestItem{ID: "3", Price: 30.0, Quantity: 3},
	}

	q := &Query{
		model:    AggregateTestItem{},
		metadata: &TestMetadata{},
		executor: &mockAggregateExecutor{items: items},
		ctx:      context.Background(),
	}

	t.Run("aggregate with field", func(t *testing.T) {
		result, err := q.Aggregate("Price")

		require.NoError(t, err)
		assert.Equal(t, int64(3), result.Count)
		assert.InDelta(t, 60.0, result.Sum, 0.001)
		assert.InDelta(t, 20.0, result.Average, 0.001)
		assert.Equal(t, 10.0, result.Min)
		assert.Equal(t, 30.0, result.Max)
	})

	t.Run("aggregate without fields", func(t *testing.T) {
		result, err := q.Aggregate()

		require.NoError(t, err)
		assert.Equal(t, int64(3), result.Count)
		assert.Equal(t, 0.0, result.Sum)
		assert.Equal(t, 0.0, result.Average)
		assert.Nil(t, result.Min)
		assert.Nil(t, result.Max)
	})
}

func TestGroupBy(t *testing.T) {
	items := []any{
		AggregateTestItem{ID: "1", Category: "electronics", Price: 100.0},
		AggregateTestItem{ID: "2", Category: "electronics", Price: 200.0},
		AggregateTestItem{ID: "3", Category: "books", Price: 20.0},
		AggregateTestItem{ID: "4", Category: "books", Price: 30.0},
		AggregateTestItem{ID: "5", Category: "electronics", Price: 150.0},
	}

	q := &Query{
		model:    AggregateTestItem{},
		metadata: &TestMetadata{},
		executor: &mockAggregateExecutor{items: items},
		ctx:      context.Background(),
	}

	results, err := q.GroupBy("Category").Execute()
	require.NoError(t, err)

	// Should have 2 groups
	assert.Len(t, results, 2)

	// Check group counts
	for _, group := range results {
		switch group.Key {
		case "electronics":
			assert.Equal(t, int64(3), group.Count)
			assert.Len(t, group.Items, 3)
		case "books":
			assert.Equal(t, int64(2), group.Count)
			assert.Len(t, group.Items, 2)
		default:
			t.Errorf("Unexpected group key: %v", group.Key)
		}
	}
}

func TestGroupByWithAggregates(t *testing.T) {
	items := []any{
		AggregateTestItem{ID: "1", Category: "electronics", Price: 100.0, Quantity: 2},
		AggregateTestItem{ID: "2", Category: "electronics", Price: 200.0, Quantity: 1},
		AggregateTestItem{ID: "3", Category: "books", Price: 20.0, Quantity: 5},
		AggregateTestItem{ID: "4", Category: "books", Price: 30.0, Quantity: 3},
		AggregateTestItem{ID: "5", Category: "electronics", Price: 150.0, Quantity: 2},
	}

	q := &Query{
		model:    AggregateTestItem{},
		metadata: &TestMetadata{},
		executor: &mockAggregateExecutor{items: items},
		ctx:      context.Background(),
	}

	results, err := q.GroupBy("Category").
		Count("item_count").
		Sum("Price", "total_price").
		Avg("Price", "avg_price").
		Min("Price", "min_price").
		Max("Price", "max_price").
		Sum("Quantity", "total_quantity").
		Execute()

	require.NoError(t, err)
	assert.Len(t, results, 2)

	// Check aggregates for each group
	for _, group := range results {
		switch group.Key {
		case "electronics":
			assert.Equal(t, int64(3), group.Aggregates["item_count"].Count)
			assert.InDelta(t, 450.0, group.Aggregates["total_price"].Sum, 0.001)
			assert.InDelta(t, 150.0, group.Aggregates["avg_price"].Average, 0.001)
			assert.Equal(t, 100.0, group.Aggregates["min_price"].Min)
			assert.Equal(t, 200.0, group.Aggregates["max_price"].Max)
			assert.InDelta(t, 5.0, group.Aggregates["total_quantity"].Sum, 0.001)
		case "books":
			assert.Equal(t, int64(2), group.Aggregates["item_count"].Count)
			assert.InDelta(t, 50.0, group.Aggregates["total_price"].Sum, 0.001)
			assert.InDelta(t, 25.0, group.Aggregates["avg_price"].Average, 0.001)
			assert.Equal(t, 20.0, group.Aggregates["min_price"].Min)
			assert.Equal(t, 30.0, group.Aggregates["max_price"].Max)
			assert.InDelta(t, 8.0, group.Aggregates["total_quantity"].Sum, 0.001)
		default:
			t.Errorf("Unexpected group key: %v", group.Key)
		}
	}
}

func TestGroupByWithHaving(t *testing.T) {
	items := []any{
		AggregateTestItem{ID: "1", Category: "electronics", Price: 100.0},
		AggregateTestItem{ID: "2", Category: "electronics", Price: 200.0},
		AggregateTestItem{ID: "3", Category: "books", Price: 20.0},
		AggregateTestItem{ID: "4", Category: "books", Price: 30.0},
		AggregateTestItem{ID: "5", Category: "electronics", Price: 150.0},
		AggregateTestItem{ID: "6", Category: "toys", Price: 10.0},
	}

	q := &Query{
		model:    AggregateTestItem{},
		metadata: &TestMetadata{},
		executor: &mockAggregateExecutor{items: items},
		ctx:      context.Background(),
	}

	// Test HAVING with COUNT
	results, err := q.GroupBy("Category").
		Count("count").
		Having("COUNT(*)", ">", 2).
		Execute()

	require.NoError(t, err)
	assert.Len(t, results, 1) // Only electronics has more than 2 items
	assert.Equal(t, "electronics", results[0].Key)

	// Test HAVING with SUM
	results, err = q.GroupBy("Category").
		Sum("Price", "total").
		Having("total", ">", 100).
		Execute()

	require.NoError(t, err)
	assert.Len(t, results, 1) // Only electronics has sum > 100
	assert.Equal(t, "electronics", results[0].Key)

	// Test multiple HAVING clauses
	results, err = q.GroupBy("Category").
		Count("count").
		Sum("Price", "total").
		Having("COUNT(*)", ">=", 2).
		Having("total", "<", 100).
		Execute()

	require.NoError(t, err)
	assert.Len(t, results, 1) // Only books matches both conditions
	assert.Equal(t, "books", results[0].Key)
}

func TestExtractNumericValue(t *testing.T) {
	tests := []struct {
		name     string
		item     any
		field    string
		expected float64
		wantErr  bool
	}{
		{
			name:     "int field",
			item:     AggregateTestItem{Quantity: 42},
			field:    "Quantity",
			expected: 42.0,
		},
		{
			name:     "float64 field",
			item:     AggregateTestItem{Price: 99.99},
			field:    "Price",
			expected: 99.99,
		},
		{
			name:     "pointer to struct",
			item:     &AggregateTestItem{Quantity: 10},
			field:    "Quantity",
			expected: 10.0,
		},
		{
			name:    "non-numeric field",
			item:    AggregateTestItem{Status: "active"},
			field:   "Status",
			wantErr: true,
		},
		{
			name:    "non-existent field",
			item:    AggregateTestItem{},
			field:   "NonExistent",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := extractNumericValue(tt.item, tt.field)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.InDelta(t, tt.expected, result, 0.001)
			}
		})
	}
}

func TestExtractFieldValue(t *testing.T) {
	tests := []struct {
		item     any
		expected any
		name     string
		field    string
		isNil    bool
	}{
		{
			name:     "string field",
			item:     AggregateTestItem{Status: "active"},
			field:    "Status",
			expected: "active",
		},
		{
			name:     "numeric field",
			item:     AggregateTestItem{Price: 42.5},
			field:    "Price",
			expected: 42.5,
		},
		{
			name:  "zero value",
			item:  AggregateTestItem{},
			field: "Price",
			isNil: true,
		},
		{
			name:  "non-existent field",
			item:  AggregateTestItem{},
			field: "NonExistent",
			isNil: true,
		},
		{
			name:     "pointer to struct",
			item:     &AggregateTestItem{Status: "test"},
			field:    "Status",
			expected: "test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractFieldValue(tt.item, tt.field)

			if tt.isNil {
				assert.Nil(t, result)
			} else {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestCompareValues(t *testing.T) {
	tests := []struct {
		a        any
		b        any
		name     string
		expected int
	}{
		{
			name:     "numeric comparison - less",
			a:        10,
			b:        20,
			expected: -1,
		},
		{
			name:     "numeric comparison - equal",
			a:        15.5,
			b:        15.5,
			expected: 0,
		},
		{
			name:     "numeric comparison - greater",
			a:        30.0,
			b:        20.0,
			expected: 1,
		},
		{
			name:     "string comparison - less",
			a:        "apple",
			b:        "banana",
			expected: -1,
		},
		{
			name:     "string comparison - equal",
			a:        "test",
			b:        "test",
			expected: 0,
		},
		{
			name:     "string comparison - greater",
			a:        "zebra",
			b:        "apple",
			expected: 1,
		},
		{
			name:     "mixed types - convert to string",
			a:        true,
			b:        false,
			expected: 1, // "true" > "false"
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compareValues(tt.a, tt.b)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestToFloat64(t *testing.T) {
	tests := []struct {
		value    any
		name     string
		expected float64
		wantErr  bool
	}{
		{name: "int", value: 42, expected: 42.0},
		{name: "int8", value: int8(42), expected: 42.0},
		{name: "int16", value: int16(42), expected: 42.0},
		{name: "int32", value: int32(42), expected: 42.0},
		{name: "int64", value: int64(42), expected: 42.0},
		{name: "uint", value: uint(42), expected: 42.0},
		{name: "uint8", value: uint8(42), expected: 42.0},
		{name: "uint16", value: uint16(42), expected: 42.0},
		{name: "uint32", value: uint32(42), expected: 42.0},
		{name: "uint64", value: uint64(42), expected: 42.0},
		{name: "float32", value: float32(42.5), expected: 42.5},
		{name: "float64", value: 42.5, expected: 42.5},
		{name: "string", value: "not a number", wantErr: true},
		{name: "bool", value: true, wantErr: true},
		{name: "struct", value: struct{}{}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := toFloat64(tt.value)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.InDelta(t, tt.expected, result, 0.001)
			}
		})
	}
}

func TestCountDistinct(t *testing.T) {
	items := []any{
		AggregateTestItem{ID: "1", Category: "electronics", Status: "active"},
		AggregateTestItem{ID: "2", Category: "electronics", Status: "pending"},
		AggregateTestItem{ID: "3", Category: "books", Status: "active"},
		AggregateTestItem{ID: "4", Category: "books", Status: "active"},
		AggregateTestItem{ID: "5", Category: "electronics", Status: "completed"},
	}

	q := &Query{
		model:    AggregateTestItem{},
		metadata: &TestMetadata{},
		executor: &mockAggregateExecutor{items: items},
		ctx:      context.Background(),
	}

	tests := []struct {
		name     string
		field    string
		expected int64
	}{
		{
			name:     "distinct categories",
			field:    "Category",
			expected: 2, // electronics, books
		},
		{
			name:     "distinct statuses",
			field:    "Status",
			expected: 3, // active, pending, completed
		},
		{
			name:     "distinct IDs",
			field:    "ID",
			expected: 5, // all unique
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := q.CountDistinct(tt.field)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHaving(t *testing.T) {
	q := &Query{
		model:    AggregateTestItem{},
		metadata: &TestMetadata{},
		ctx:      context.Background(),
	}

	// Having is not fully implemented, just returns the query
	result := q.Having("count > ?", 5)
	assert.Equal(t, q, result)
}

func TestGetAllItems_Error(t *testing.T) {
	q := &Query{
		model:    AggregateTestItem{},
		metadata: &TestMetadata{},
		executor: &mockAggregateExecutor{err: errors.New("query failed")},
		ctx:      context.Background(),
	}

	// Test that errors are properly propagated
	_, err := q.Sum("Price")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "query failed")
}
