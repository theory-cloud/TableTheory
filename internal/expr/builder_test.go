package expr_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/internal/expr"
)

func TestNewBuilder(t *testing.T) {
	builder := expr.NewBuilder()
	assert.NotNil(t, builder)
}

func TestAddKeyCondition(t *testing.T) {
	tests := []struct {
		name          string
		field         string
		operator      string
		value         any
		expectedExpr  string
		expectedError bool
	}{
		{
			name:         "simple equality",
			field:        "id",
			operator:     "=",
			value:        "123",
			expectedExpr: "#n1 = :v1",
		},
		{
			name:         "sort key range",
			field:        "timestamp",
			operator:     ">",
			value:        1000,
			expectedExpr: "#TIMESTAMP > :v1",
		},
		{
			name:         "begins with",
			field:        "sk",
			operator:     "BEGINS_WITH",
			value:        "USER#",
			expectedExpr: "begins_with(#n1, :v1)",
		},
		{
			name:         "between",
			field:        "timestamp",
			operator:     "BETWEEN",
			value:        []any{1000, 2000},
			expectedExpr: "#TIMESTAMP BETWEEN :v1 AND :v2",
		},
		{
			name:          "invalid operator for key condition",
			field:         "id",
			operator:      "INVALID_OPERATOR",
			value:         "test",
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := expr.NewBuilder()
			err := builder.AddKeyCondition(tt.field, tt.operator, tt.value)

			if tt.expectedError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			components := builder.Build()
			assert.Equal(t, tt.expectedExpr, components.KeyConditionExpression)
		})
	}
}

func TestAddFilterCondition(t *testing.T) {
	type filterCondition struct {
		value     any
		logicalOp string
		field     string
		operator  string
	}

	tests := []struct {
		name         string
		expectedExpr string
		conditions   []filterCondition
	}{
		{
			name: "single filter",
			conditions: []filterCondition{
				{logicalOp: "AND", field: "status", operator: "=", value: "active"},
			},
			expectedExpr: "#STATUS = :v1",
		},
		{
			name: "multiple AND filters",
			conditions: []filterCondition{
				{logicalOp: "AND", field: "status", operator: "=", value: "active"},
				{logicalOp: "AND", field: "age", operator: ">", value: 18},
			},
			expectedExpr: "#STATUS = :v1 AND #n2 > :v2",
		},
		{
			name: "OR filters",
			conditions: []filterCondition{
				{logicalOp: "AND", field: "status", operator: "=", value: "active"},
				{logicalOp: "OR", field: "status", operator: "=", value: "pending"},
			},
			expectedExpr: "#STATUS = :v1 OR #STATUS = :v2",
		},
		{
			name: "IN operator",
			conditions: []filterCondition{
				{logicalOp: "AND", field: "status", operator: "IN", value: []string{"active", "pending", "completed"}},
			},
			expectedExpr: "#STATUS IN (:v1, :v2, :v3)",
		},
		{
			name: "EXISTS operator",
			conditions: []filterCondition{
				{logicalOp: "AND", field: "email", operator: "EXISTS", value: nil},
			},
			expectedExpr: "attribute_exists(#n1)",
		},
		{
			name: "NOT_EXISTS operator",
			conditions: []filterCondition{
				{logicalOp: "AND", field: "deletedAt", operator: "NOT_EXISTS", value: nil},
			},
			expectedExpr: "attribute_not_exists(#n1)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := expr.NewBuilder()

			for _, cond := range tt.conditions {
				err := builder.AddFilterCondition(cond.logicalOp, cond.field, cond.operator, cond.value)
				require.NoError(t, err)
			}

			components := builder.Build()
			assert.Equal(t, tt.expectedExpr, components.FilterExpression)
		})
	}
}

func TestReservedWords(t *testing.T) {
	builder := expr.NewBuilder()

	// Test reserved words that should be escaped
	reservedWords := []string{"status", "size", "name", "type", "count", "timestamp"}

	for _, word := range reservedWords {
		err := builder.AddFilterCondition("AND", word, "=", "test")
		require.NoError(t, err)
	}

	components := builder.Build()

	// Check that all reserved words are properly escaped
	for _, word := range reservedWords {
		upperWord := "#" + strings.ToUpper(word)
		assert.Contains(t, components.ExpressionAttributeNames, upperWord)
		assert.Equal(t, word, components.ExpressionAttributeNames[upperWord])
	}
}

func TestAddProjection(t *testing.T) {
	builder := expr.NewBuilder()

	fields := []string{"id", "name", "email", "status"}
	builder.AddProjection(fields...)

	components := builder.Build()

	// Check the exact projection expression
	// name and status are reserved words
	assert.Equal(t, "#n1, #NAME, #n3, #STATUS", components.ProjectionExpression)

	// Check reserved word handling in projection
	assert.Contains(t, components.ExpressionAttributeNames, "#NAME")
	assert.Contains(t, components.ExpressionAttributeNames, "#STATUS")
}

func TestUpdateExpressions(t *testing.T) {
	t.Run("SET expressions", func(t *testing.T) {
		builder := expr.NewBuilder()

		require.NoError(t, builder.AddUpdateSet("name", "John Doe"))
		require.NoError(t, builder.AddUpdateSet("age", 30))
		require.NoError(t, builder.AddUpdateSet("email", "john@example.com"))

		components := builder.Build()

		assert.Contains(t, components.UpdateExpression, "SET")
		assert.Contains(t, components.UpdateExpression, "#NAME = :v1")
		assert.Contains(t, components.UpdateExpression, "#n2 = :v2")
		assert.Contains(t, components.UpdateExpression, "#n3 = :v3")
	})

	t.Run("ADD expressions", func(t *testing.T) {
		builder := expr.NewBuilder()

		require.NoError(t, builder.AddUpdateAdd("loginCount", 1))
		require.NoError(t, builder.AddUpdateAdd("points", 10))

		components := builder.Build()

		assert.Contains(t, components.UpdateExpression, "ADD")
		assert.Contains(t, components.UpdateExpression, "#n1 :v1")
		assert.Contains(t, components.UpdateExpression, "#n2 :v2")
	})

	t.Run("REMOVE expressions", func(t *testing.T) {
		builder := expr.NewBuilder()

		require.NoError(t, builder.AddUpdateRemove("tempField"))
		require.NoError(t, builder.AddUpdateRemove("oldData"))

		components := builder.Build()

		assert.Contains(t, components.UpdateExpression, "REMOVE")
		assert.Contains(t, components.UpdateExpression, "#n1")
		assert.Contains(t, components.UpdateExpression, "#n2")
	})

	t.Run("DELETE expressions", func(t *testing.T) {
		builder := expr.NewBuilder()

		require.NoError(t, builder.AddUpdateDelete("tags", []string{"old", "deprecated"}))

		components := builder.Build()

		assert.Contains(t, components.UpdateExpression, "DELETE")
		assert.Contains(t, components.UpdateExpression, "#n1 :v1")
	})

	t.Run("mixed update expressions", func(t *testing.T) {
		builder := expr.NewBuilder()

		require.NoError(t, builder.AddUpdateSet("name", "Jane Doe"))
		require.NoError(t, builder.AddUpdateAdd("version", 1))
		require.NoError(t, builder.AddUpdateRemove("tempData"))
		require.NoError(t, builder.AddUpdateDelete("tags", []string{"temp"}))

		components := builder.Build()

		// Should have all update types in the correct order
		assert.Contains(t, components.UpdateExpression, "SET")
		assert.Contains(t, components.UpdateExpression, "ADD")
		assert.Contains(t, components.UpdateExpression, "REMOVE")
		assert.Contains(t, components.UpdateExpression, "DELETE")
	})

	t.Run("list index update expressions", func(t *testing.T) {
		t.Run("SET list element", func(t *testing.T) {
			builder := expr.NewBuilder()

			require.NoError(t, builder.AddUpdateSet("items[0]", "value"))

			components := builder.Build()

			assert.Contains(t, components.UpdateExpression, "SET")
			assert.Contains(t, components.UpdateExpression, "[0] = :v1")

			for _, attrName := range components.ExpressionAttributeNames {
				assert.NotContains(t, attrName, "[")
				assert.NotContains(t, attrName, "]")
			}
		})

		t.Run("REMOVE list element", func(t *testing.T) {
			builder := expr.NewBuilder()

			require.NoError(t, builder.AddUpdateRemove("items[2]"))

			components := builder.Build()

			assert.Contains(t, components.UpdateExpression, "REMOVE")
			assert.Contains(t, components.UpdateExpression, "[2]")
		})

		t.Run("reject list index injection", func(t *testing.T) {
			builder := expr.NewBuilder()

			require.Error(t, builder.AddUpdateSet("items[0] = :v2, other = :v3, items[1]", "value"))
			require.Error(t, builder.AddUpdateSet("items[-1]", "value"))
			require.Error(t, builder.AddUpdateRemove("items[0], other]"))
		})
	})
}

func TestAddConditionExpression(t *testing.T) {
	builder := expr.NewBuilder()

	// Add multiple conditions
	err := builder.AddConditionExpression("version", "=", 1)
	require.NoError(t, err)

	err = builder.AddConditionExpression("status", "!=", "deleted")
	require.NoError(t, err)

	components := builder.Build()

	assert.Contains(t, components.ConditionExpression, "#n1 = :v1")
	assert.Contains(t, components.ConditionExpression, "AND")
	assert.Contains(t, components.ConditionExpression, "#STATUS <> :v2")
}

func TestComplexExpressions(t *testing.T) {
	t.Run("nested attributes", func(t *testing.T) {
		builder := expr.NewBuilder()

		err := builder.AddFilterCondition("AND", "address.city", "=", "New York")
		require.NoError(t, err)

		err = builder.AddFilterCondition("AND", "metadata.tags", "CONTAINS", "important")
		require.NoError(t, err)

		components := builder.Build()

		// Should handle nested paths
		assert.Contains(t, components.FilterExpression, "#n1.#n2")
		assert.Contains(t, components.FilterExpression, "contains(")
	})

	t.Run("all operators", func(t *testing.T) {
		operators := map[string]any{
			"=":           "value",
			"!=":          "value",
			"<":           10,
			"<=":          20,
			">":           30,
			">=":          40,
			"BETWEEN":     []any{1, 10},
			"IN":          []string{"a", "b", "c"},
			"BEGINS_WITH": "prefix",
			"CONTAINS":    "substring",
			"EXISTS":      nil,
			"NOT_EXISTS":  nil,
		}

		for op, value := range operators {
			t.Run(op, func(t *testing.T) {
				builder := expr.NewBuilder()
				err := builder.AddFilterCondition("AND", "field", op, value)

				if op == "INVALID" {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
					components := builder.Build()
					assert.NotEmpty(t, components.FilterExpression)
				}
			})
		}
	})
}

func TestAddGroupFilter(t *testing.T) {
	mainBuilder := expr.NewBuilder()

	// Add a regular filter
	require.NoError(t, mainBuilder.AddFilterCondition("AND", "status", "=", "active"))

	// Create a sub-group
	subBuilder := expr.NewBuilder()
	require.NoError(t, subBuilder.AddFilterCondition("AND", "age", ">", 18))
	require.NoError(t, subBuilder.AddFilterCondition("OR", "role", "=", "admin"))

	subComponents := subBuilder.Build()
	mainBuilder.AddGroupFilter("AND", subComponents)

	// Add another regular filter
	require.NoError(t, mainBuilder.AddFilterCondition("AND", "verified", "=", true))

	components := mainBuilder.Build()

	// Should have grouped expression with parentheses
	assert.Contains(t, components.FilterExpression, "(")
	assert.Contains(t, components.FilterExpression, ")")
	assert.Contains(t, components.FilterExpression, "#STATUS = :v1")
	assert.Contains(t, components.FilterExpression, "#n2 = :v2") // verified - corrected placeholder
}

func TestConvertToSlice(t *testing.T) {
	builder := expr.NewBuilder()

	// Test with []string
	err := builder.AddFilterCondition("AND", "status", "IN", []string{"active", "pending"})
	assert.NoError(t, err)

	// Test with []int
	err = builder.AddFilterCondition("AND", "age", "IN", []int{18, 21, 25})
	assert.NoError(t, err)

	// Test with []any
	err = builder.AddFilterCondition("AND", "mixed", "IN", []any{"string", 123, true})
	assert.NoError(t, err)

	// Test with invalid type (not a slice)
	err = builder.AddFilterCondition("AND", "invalid", "IN", "not-a-slice")
	assert.Error(t, err)

	// Test with too many values (>100)
	largeSlice := make([]string, 101)
	for i := range largeSlice {
		largeSlice[i] = fmt.Sprintf("value%d", i)
	}
	err = builder.AddFilterCondition("AND", "toomany", "IN", largeSlice)
	assert.Error(t, err)
}

func TestAddAdvancedFunction(t *testing.T) {
	tests := []struct {
		name         string
		function     string
		field        string
		expectedExpr string
		args         []any
		expectedErr  bool
	}{
		{
			name:         "size function",
			function:     "size",
			field:        "tags",
			args:         []any{},
			expectedExpr: "size(#n1)",
		},
		{
			name:         "attribute_type function",
			function:     "attribute_type",
			field:        "data",
			args:         []any{"M"},
			expectedExpr: "attribute_type(#DATA, :v1)", // data is a reserved word
		},
		{
			name:         "attribute_exists function",
			function:     "attribute_exists",
			field:        "email",
			args:         []any{},
			expectedExpr: "attribute_exists(#n1)",
		},
		{
			name:         "attribute_not_exists function",
			function:     "attribute_not_exists",
			field:        "deletedAt",
			args:         []any{},
			expectedExpr: "attribute_not_exists(#n1)",
		},
		{
			name:         "list_append function",
			function:     "list_append",
			field:        "items",
			args:         []any{[]string{"new1", "new2"}},
			expectedExpr: "list_append(#ITEMS, :v1)", // items is a reserved word
		},
		{
			name:        "attribute_type missing argument",
			function:    "attribute_type",
			field:       "data",
			args:        []any{},
			expectedErr: true,
		},
		{
			name:        "list_append missing argument",
			function:    "list_append",
			field:       "items",
			args:        []any{},
			expectedErr: true,
		},
		{
			name:        "unknown function",
			function:    "unknown_func",
			field:       "field",
			args:        []any{},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := expr.NewBuilder()
			expr, err := builder.AddAdvancedFunction(tt.function, tt.field, tt.args...)

			if tt.expectedErr {
				assert.Error(t, err)
				assert.Empty(t, expr)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedExpr, expr)
			}
		})
	}
}

func TestAddUpdateFunction(t *testing.T) {
	tests := []struct {
		name        string
		field       string
		function    string
		args        []any
		expectedErr bool
	}{
		{
			name:     "if_not_exists function",
			field:    "views",
			function: "if_not_exists",
			args:     []any{"views", 0},
		},
		{
			name:     "list_append function",
			field:    "history",
			function: "list_append",
			args:     []any{"history", []string{"new_event"}},
		},
		{
			name:     "list_append prepend",
			field:    "history",
			function: "list_append",
			args:     []any{[]string{"new_event"}, "history"},
		},
		{
			name:     "list_append merge lists",
			field:    "history",
			function: "list_append",
			args:     []any{[]string{"a"}, []string{"b"}},
		},
		{
			name:        "if_not_exists missing args",
			field:       "views",
			function:    "if_not_exists",
			args:        []any{},
			expectedErr: true,
		},
		{
			name:        "list_append missing args",
			field:       "history",
			function:    "list_append",
			args:        []any{},
			expectedErr: true,
		},
		{
			name:        "unknown function",
			field:       "field",
			function:    "unknown_func",
			args:        []any{"arg"},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := expr.NewBuilder()
			err := builder.AddUpdateFunction(tt.field, tt.function, tt.args...)

			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				components := builder.Build()
				assert.NotEmpty(t, components.UpdateExpression)
				assert.Contains(t, components.UpdateExpression, "SET")
			}
		})
	}
}

func TestAddKeyCondition_BetweenRequiresTwoValues(t *testing.T) {
	builder := expr.NewBuilder()
	err := builder.AddKeyCondition("timestamp", "BETWEEN", []any{1000})
	require.Error(t, err)
}

func TestAddFilterCondition_IN_RequiresSlice(t *testing.T) {
	builder := expr.NewBuilder()
	err := builder.AddFilterCondition("AND", "status", "IN", "not-a-slice")
	require.Error(t, err)
}

func TestBuildCompleteExpression(t *testing.T) {
	builder := expr.NewBuilder()

	// Add key conditions
	require.NoError(t, builder.AddKeyCondition("pk", "=", "USER#123"))
	require.NoError(t, builder.AddKeyCondition("sk", "BEGINS_WITH", "ORDER#"))

	// Add filters
	require.NoError(t, builder.AddFilterCondition("AND", "status", "=", "active"))
	require.NoError(t, builder.AddFilterCondition("AND", "amount", ">", 100))

	// Add projection
	builder.AddProjection("id", "status", "amount", "createdAt")

	// Add update expressions
	require.NoError(t, builder.AddUpdateSet("status", "completed"))
	require.NoError(t, builder.AddUpdateAdd("totalAmount", 100))

	// Add condition
	require.NoError(t, builder.AddConditionExpression("version", "=", 1))

	components := builder.Build()

	// Verify all components are present
	assert.NotEmpty(t, components.KeyConditionExpression)
	assert.NotEmpty(t, components.FilterExpression)
	assert.NotEmpty(t, components.ProjectionExpression)
	assert.NotEmpty(t, components.UpdateExpression)
	assert.NotEmpty(t, components.ConditionExpression)
	assert.NotEmpty(t, components.ExpressionAttributeNames)
	assert.NotEmpty(t, components.ExpressionAttributeValues)

	// Verify structure
	assert.Contains(t, components.KeyConditionExpression, "AND")
	assert.Contains(t, components.FilterExpression, "AND")
	assert.Contains(t, components.UpdateExpression, "SET")
	assert.Contains(t, components.UpdateExpression, "ADD")
}
