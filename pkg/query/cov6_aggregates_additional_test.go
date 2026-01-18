package query

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAggregateResultValue_CoversConversionAndDefaults_COV6(t *testing.T) {
	t.Run("min conversion failure", func(t *testing.T) {
		value, ok := aggregateResultValue(&AggregateResult{Min: "not-a-number"})
		require.False(t, ok)
		require.Zero(t, value)
	})

	t.Run("max conversion failure", func(t *testing.T) {
		value, ok := aggregateResultValue(&AggregateResult{Max: "not-a-number"})
		require.False(t, ok)
		require.Zero(t, value)
	})

	t.Run("count prefers non-zero", func(t *testing.T) {
		value, ok := aggregateResultValue(&AggregateResult{Count: 2})
		require.True(t, ok)
		require.Equal(t, float64(2), value)
	})

	t.Run("sum prefers non-zero", func(t *testing.T) {
		value, ok := aggregateResultValue(&AggregateResult{Sum: 1.25})
		require.True(t, ok)
		require.Equal(t, 1.25, value)
	})

	t.Run("average prefers non-zero", func(t *testing.T) {
		value, ok := aggregateResultValue(&AggregateResult{Average: 3.5})
		require.True(t, ok)
		require.Equal(t, 3.5, value)
	})

	t.Run("all-zero yields ok zero", func(t *testing.T) {
		value, ok := aggregateResultValue(&AggregateResult{})
		require.True(t, ok)
		require.Zero(t, value)
	})
}

func TestGroupByQuery_EvaluateHaving_ErrorBranches_COV6(t *testing.T) {
	group := &GroupedResult{
		Count: 2,
		Aggregates: map[string]*AggregateResult{
			"bad": {Min: "not-a-number"},
		},
	}

	g := &GroupByQuery{
		havingClauses: []havingClause{
			{aggregate: "missing", operator: ">", value: 1},
		},
	}
	require.False(t, g.evaluateHaving(group))

	g.havingClauses = []havingClause{
		{aggregate: "bad", operator: ">", value: 1},
	}
	require.False(t, g.evaluateHaving(group))

	g.havingClauses = []havingClause{
		{aggregate: "COUNT(*)", operator: ">", value: "not-a-number"},
	}
	require.False(t, g.evaluateHaving(group))
}

func TestCompareHaving_Operators_COV6(t *testing.T) {
	require.True(t, compareHaving(1, "=", 1))
	require.True(t, compareHaving(2, ">=", 1))
	require.True(t, compareHaving(1, "<=", 1))
	require.True(t, compareHaving(1, "!=", 2))
	require.True(t, compareHaving(1, "unknown", 2))
}
