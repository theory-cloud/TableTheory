package index_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/index"
)

func TestNewSelector(t *testing.T) {
	indexes := []core.IndexSchema{
		{
			Name:         "test-index",
			Type:         "GSI",
			PartitionKey: "userId",
			SortKey:      "timestamp",
		},
	}

	selector := index.NewSelector(indexes)
	assert.NotNil(t, selector)
}

func TestAnalyzeConditions(t *testing.T) {
	tests := []struct {
		expected   index.RequiredKeys
		name       string
		conditions []index.Condition
	}{
		{
			name: "single equality condition",
			conditions: []index.Condition{
				{Field: "userId", Operator: "=", Value: "123"},
			},
			expected: index.RequiredKeys{
				PartitionKey: "userId",
				SortKey:      "",
				SortKeyOp:    "",
			},
		},
		{
			name: "partition and sort key conditions",
			conditions: []index.Condition{
				{Field: "userId", Operator: "=", Value: "123"},
				{Field: "timestamp", Operator: ">", Value: 1000},
			},
			expected: index.RequiredKeys{
				PartitionKey: "userId",
				SortKey:      "timestamp",
				SortKeyOp:    ">",
			},
		},
		{
			name: "no equality conditions",
			conditions: []index.Condition{
				{Field: "status", Operator: ">", Value: 1},
				{Field: "name", Operator: "CONTAINS", Value: "test"},
			},
			expected: index.RequiredKeys{
				PartitionKey: "",
				SortKey:      "",
				SortKeyOp:    "",
			},
		},
		{
			name: "multiple equality conditions",
			conditions: []index.Condition{
				{Field: "userId", Operator: "=", Value: "123"},
				{Field: "status", Operator: "=", Value: "active"},
			},
			expected: index.RequiredKeys{
				PartitionKey: "userId",
				SortKey:      "status",
				SortKeyOp:    "=",
			},
		},
		{
			name: "BETWEEN operator on sort key",
			conditions: []index.Condition{
				{Field: "userId", Operator: "=", Value: "123"},
				{Field: "timestamp", Operator: "BETWEEN", Value: []int{1000, 2000}},
			},
			expected: index.RequiredKeys{
				PartitionKey: "userId",
				SortKey:      "timestamp",
				SortKeyOp:    "between",
			},
		},
		{
			name: "BEGINS_WITH operator",
			conditions: []index.Condition{
				{Field: "pk", Operator: "=", Value: "USER#123"},
				{Field: "sk", Operator: "BEGINS_WITH", Value: "ORDER#"},
			},
			expected: index.RequiredKeys{
				PartitionKey: "pk",
				SortKey:      "sk",
				SortKeyOp:    "begins_with",
			},
		},
		{
			name: "case insensitive operators",
			conditions: []index.Condition{
				{Field: "id", Operator: "EQ", Value: "123"},
				{Field: "sort", Operator: "LT", Value: 100},
			},
			expected: index.RequiredKeys{
				PartitionKey: "id",
				SortKey:      "sort",
				SortKeyOp:    "<",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := index.AnalyzeConditions(tt.conditions)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSelectOptimal(t *testing.T) {
	tests := []struct {
		required     index.RequiredKeys
		name         string
		expectedName string
		expectedType string
		indexes      []core.IndexSchema
		expectNil    bool
	}{
		{
			name: "no partition key required",
			indexes: []core.IndexSchema{
				{
					Name:         "test-index",
					Type:         "GSI",
					PartitionKey: "userId",
				},
			},
			required: index.RequiredKeys{
				PartitionKey: "",
			},
			expectNil: true,
		},
		{
			name: "exact match on primary index",
			indexes: []core.IndexSchema{
				{
					Name:         "",
					Type:         "PRIMARY",
					PartitionKey: "id",
					SortKey:      "timestamp",
				},
			},
			required: index.RequiredKeys{
				PartitionKey: "id",
				SortKey:      "timestamp",
				SortKeyOp:    "=",
			},
			expectNil:    false,
			expectedName: "",
			expectedType: "PRIMARY",
		},
		{
			name: "prefer GSI over primary when both match",
			indexes: []core.IndexSchema{
				{
					Name:         "",
					Type:         "PRIMARY",
					PartitionKey: "id",
					SortKey:      "timestamp",
				},
				{
					Name:           "user-index",
					Type:           "GSI",
					PartitionKey:   "userId",
					SortKey:        "timestamp",
					ProjectionType: "ALL",
				},
			},
			required: index.RequiredKeys{
				PartitionKey: "userId",
				SortKey:      "timestamp",
				SortKeyOp:    "=",
			},
			expectNil:    false,
			expectedName: "user-index",
			expectedType: "GSI",
		},
		{
			name: "no matching index",
			indexes: []core.IndexSchema{
				{
					Name:         "user-index",
					Type:         "GSI",
					PartitionKey: "userId",
				},
				{
					Name:         "status-index",
					Type:         "GSI",
					PartitionKey: "status",
				},
			},
			required: index.RequiredKeys{
				PartitionKey: "email",
			},
			expectNil: true,
		},
		{
			name: "partial match - only partition key",
			indexes: []core.IndexSchema{
				{
					Name:         "user-index",
					Type:         "GSI",
					PartitionKey: "userId",
					SortKey:      "createdAt",
				},
			},
			required: index.RequiredKeys{
				PartitionKey: "userId",
				SortKey:      "",
			},
			expectNil:    false,
			expectedName: "user-index",
		},
		{
			name: "range query on sort key",
			indexes: []core.IndexSchema{
				{
					Name:         "",
					Type:         "PRIMARY",
					PartitionKey: "pk",
					SortKey:      "sk",
				},
			},
			required: index.RequiredKeys{
				PartitionKey: "pk",
				SortKey:      "sk",
				SortKeyOp:    "between",
			},
			expectNil:    false,
			expectedName: "",
		},
		{
			name: "prefer ALL projection",
			indexes: []core.IndexSchema{
				{
					Name:           "index1",
					Type:           "GSI",
					PartitionKey:   "userId",
					ProjectionType: "KEYS_ONLY",
				},
				{
					Name:           "index2",
					Type:           "GSI",
					PartitionKey:   "userId",
					ProjectionType: "ALL",
				},
			},
			required: index.RequiredKeys{
				PartitionKey: "userId",
			},
			expectNil:    false,
			expectedName: "index2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selector := index.NewSelector(tt.indexes)
			result, err := selector.SelectOptimal(tt.required, nil)

			require.NoError(t, err)

			if tt.expectNil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, tt.expectedName, result.Name)
				if tt.expectedType != "" {
					assert.Equal(t, tt.expectedType, result.Type)
				}
			}
		})
	}
}

func TestSelectOptimal_ComplexScenarios(t *testing.T) {
	// Test with multiple indexes and complex scoring
	indexes := []core.IndexSchema{
		{
			Name:           "",
			Type:           "PRIMARY",
			PartitionKey:   "id",
			SortKey:        "timestamp",
			ProjectionType: "ALL",
		},
		{
			Name:           "user-date-index",
			Type:           "GSI",
			PartitionKey:   "userId",
			SortKey:        "date",
			ProjectionType: "ALL",
		},
		{
			Name:           "user-status-index",
			Type:           "GSI",
			PartitionKey:   "userId",
			SortKey:        "status",
			ProjectionType: "KEYS_ONLY",
		},
		{
			Name:           "status-index",
			Type:           "GSI",
			PartitionKey:   "status",
			SortKey:        "timestamp",
			ProjectionType: "ALL",
		},
		{
			Name:           "email-index",
			Type:           "GSI",
			PartitionKey:   "email",
			ProjectionType: "ALL",
		},
	}

	selector := index.NewSelector(indexes)

	tests := []struct {
		name         string
		required     index.RequiredKeys
		expectedName string
		description  string
	}{
		{
			name: "exact match on primary key",
			required: index.RequiredKeys{
				PartitionKey: "id",
				SortKey:      "timestamp",
				SortKeyOp:    "=",
			},
			expectedName: "",
			description:  "Should use primary index for exact id+timestamp match",
		},
		{
			name: "GSI with sort key match",
			required: index.RequiredKeys{
				PartitionKey: "userId",
				SortKey:      "date",
				SortKeyOp:    ">",
			},
			expectedName: "user-date-index",
			description:  "Should prefer GSI with matching sort key",
		},
		{
			name: "GSI with exact sort key match",
			required: index.RequiredKeys{
				PartitionKey: "userId",
				SortKey:      "status",
				SortKeyOp:    "=",
			},
			expectedName: "user-status-index",
			description:  "Should use index with exact sort key match",
		},
		{
			name: "single field GSI",
			required: index.RequiredKeys{
				PartitionKey: "email",
			},
			expectedName: "email-index",
			description:  "Should use email index for email queries",
		},
		{
			name: "begins_with operation",
			required: index.RequiredKeys{
				PartitionKey: "status",
				SortKey:      "timestamp",
				SortKeyOp:    "begins_with",
			},
			expectedName: "status-index",
			description:  "Should handle begins_with on sort key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := selector.SelectOptimal(tt.required, nil)
			require.NoError(t, err)
			require.NotNil(t, result, tt.description)
			assert.Equal(t, tt.expectedName, result.Name, tt.description)
		})
	}
}

func TestNormalizeOperator(t *testing.T) {
	// This function is not exported, so we test it indirectly through AnalyzeConditions
	tests := []struct {
		operator string
		expected string
	}{
		{"=", "="},
		{"EQ", "="},
		{"eq", "="},
		{"<", "<"},
		{"LT", "<"},
		{"<=", "<="},
		{"LE", "<="},
		{">", ">"},
		{"GT", ">"},
		{">=", ">="},
		{"GE", ">="},
		{"BEGINS_WITH", "begins_with"},
		{"BETWEEN", "between"},
		{"CUSTOM_OP", "custom_op"},
	}

	for _, tt := range tests {
		t.Run(tt.operator, func(t *testing.T) {
			conditions := []index.Condition{
				{Field: "pk", Operator: "=", Value: "test"},
				{Field: "sk", Operator: tt.operator, Value: "value"},
			}
			result := index.AnalyzeConditions(conditions)
			assert.Equal(t, tt.expected, result.SortKeyOp)
		})
	}
}
