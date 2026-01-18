package query

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/pkg/core"
)

// Mock types for testing
type TestModel struct {
	ID        string    `theorydb:"pk"`
	SortKey   string    `theorydb:"sk"`
	UserID    string    `theorydb:"gsi:UserIndex:pk"`
	Timestamp time.Time `theorydb:"gsi:UserIndex:sk"`
	Status    string
	Name      string
	Count     int
}

// MockExecutor implements QueryExecutor for testing
type MockExecutor struct {
	QueryFunc func(input *core.CompiledQuery, dest any) error
	ScanFunc  func(input *core.CompiledQuery, dest any) error
}

func (m *MockExecutor) ExecuteQuery(input *core.CompiledQuery, dest any) error {
	if m.QueryFunc != nil {
		return m.QueryFunc(input, dest)
	}
	return nil
}

func (m *MockExecutor) ExecuteScan(input *core.CompiledQuery, dest any) error {
	if m.ScanFunc != nil {
		return m.ScanFunc(input, dest)
	}
	return nil
}

// MockMetadata implements core.ModelMetadata for testing
type MockMetadata struct{}

func (m *MockMetadata) TableName() string {
	return "test-table"
}

func (m *MockMetadata) PrimaryKey() core.KeySchema {
	return core.KeySchema{
		PartitionKey: "ID",
		SortKey:      "SortKey",
	}
}

func (m *MockMetadata) Indexes() []core.IndexSchema {
	return []core.IndexSchema{}
}

func (m *MockMetadata) AttributeMetadata(field string) *core.AttributeMetadata {
	return nil
}

func (m *MockMetadata) VersionFieldName() string {
	return ""
}

// Test metadata for TestModel
func testMetadata() core.ModelMetadata {
	return &MockMetadata{}
}

func TestNewOptimizer(t *testing.T) {
	t.Run("with default options", func(t *testing.T) {
		optimizer := NewOptimizer(nil)

		assert.NotNil(t, optimizer)
		assert.True(t, optimizer.enableAdaptive)
		assert.True(t, optimizer.enableParallel)
		assert.Greater(t, optimizer.maxParallelism, 0)
		assert.Equal(t, 1*time.Hour, optimizer.planCacheTTL)
	})

	t.Run("with custom options", func(t *testing.T) {
		opts := &OptimizationOptions{
			EnableAdaptive: false,
			EnableParallel: false,
			MaxParallelism: 2,
			PlanCacheTTL:   30 * time.Minute,
			MaxCacheSize:   500,
		}

		optimizer := NewOptimizer(opts)

		assert.False(t, optimizer.enableAdaptive)
		assert.False(t, optimizer.enableParallel)
		assert.Equal(t, 2, optimizer.maxParallelism)
		assert.Equal(t, 30*time.Minute, optimizer.planCacheTTL)
		assert.Equal(t, 500, optimizer.cacheSize)
	})
}

func TestOptimizeQuery(t *testing.T) {
	optimizer := NewOptimizer(nil)
	executor := &MockExecutor{}
	metadata := testMetadata()

	t.Run("query with partition key", func(t *testing.T) {
		q := New(&TestModel{}, metadata, executor)
		q.Where("ID", "=", "123")

		plan, err := optimizer.OptimizeQuery(q)

		require.NoError(t, err)
		assert.Equal(t, "Query", plan.Operation)
		assert.NotEmpty(t, plan.ID)
		assert.NotNil(t, plan.EstimatedCost)

		// Should have optimization hint about Select()
		assert.Contains(t, plan.OptimizationHints, "TIP: Use Select() to project only needed attributes and reduce data transfer")
	})

	t.Run("query without partition key", func(t *testing.T) {
		q := New(&TestModel{}, metadata, executor)
		q.Where("Status", "=", "active")

		plan, err := optimizer.OptimizeQuery(q)

		require.NoError(t, err)
		assert.Equal(t, "Query", plan.Operation)

		// Should warn about full table scan
		found := false
		for _, hint := range plan.OptimizationHints {
			if hint == "WARNING: Query will result in full table scan. Consider adding a GSI." {
				found = true
				break
			}
		}
		assert.True(t, found, "Expected warning about full table scan")
	})

	t.Run("scan operation", func(t *testing.T) {
		q := New(&TestModel{}, metadata, executor)
		// No conditions means scan

		plan, err := optimizer.OptimizeQuery(q)

		require.NoError(t, err)
		assert.Equal(t, "Scan", plan.Operation)

		// Should warn about scan
		found := false
		for _, hint := range plan.OptimizationHints {
			if hint == "WARNING: Scan operation will read entire table. Consider using Query with appropriate conditions." {
				found = true
				break
			}
		}
		assert.True(t, found, "Expected warning about scan operation")

		// Should suggest parallel scan
		assert.Greater(t, plan.ParallelSegments, 1)
	})

	t.Run("query with sort key suggestion", func(t *testing.T) {
		q := New(&TestModel{}, metadata, executor)
		q.Where("ID", "=", "123")
		// No sort key condition

		plan, err := optimizer.OptimizeQuery(q)

		require.NoError(t, err)

		// Should suggest using sort key
		found := false
		for _, hint := range plan.OptimizationHints {
			if hint == "TIP: Consider adding sort key condition for more efficient queries" {
				found = true
				break
			}
		}
		assert.True(t, found, "Expected tip about sort key")
	})

	t.Run("query with inefficient operator", func(t *testing.T) {
		q := New(&TestModel{}, metadata, executor)
		q.Where("ID", "CONTAINS", "test")

		plan, err := optimizer.OptimizeQuery(q)

		require.NoError(t, err)

		// Should warn about inefficient operator
		found := false
		for _, hint := range plan.OptimizationHints {
			if hint == "WARNING: CONTAINS operator on partition key is inefficient" {
				found = true
				break
			}
		}
		assert.True(t, found, "Expected warning about inefficient operator")
	})
}

func TestPlanCaching(t *testing.T) {
	optimizer := NewOptimizer(&OptimizationOptions{
		PlanCacheTTL: 100 * time.Millisecond,
	})
	executor := &MockExecutor{}
	metadata := testMetadata()

	t.Run("cache hit", func(t *testing.T) {
		q := New(&TestModel{}, metadata, executor)
		q.Where("ID", "=", "123")

		// First call - cache miss
		plan1, err := optimizer.OptimizeQuery(q)
		require.NoError(t, err)

		// Second call - should hit cache
		plan2, err := optimizer.OptimizeQuery(q)
		require.NoError(t, err)

		assert.Equal(t, plan1.ID, plan2.ID)
		assert.Equal(t, plan1.CachedAt, plan2.CachedAt) // Same cached time
	})

	t.Run("cache expiration", func(t *testing.T) {
		q := New(&TestModel{}, metadata, executor)
		q.Where("ID", "=", "456")

		// First call
		plan1, err := optimizer.OptimizeQuery(q)
		require.NoError(t, err)

		// Wait for cache to expire
		time.Sleep(150 * time.Millisecond)

		// Second call - should regenerate
		plan2, err := optimizer.OptimizeQuery(q)
		require.NoError(t, err)

		assert.Equal(t, plan1.ID, plan2.ID)
		assert.NotEqual(t, plan1.CachedAt, plan2.CachedAt) // Different cached times
	})
}

func TestCostEstimation(t *testing.T) {
	optimizer := NewOptimizer(nil)
	executor := &MockExecutor{}
	metadata := testMetadata()

	t.Run("query cost estimation", func(t *testing.T) {
		q := New(&TestModel{}, metadata, executor)
		q.Where("ID", "=", "123").Limit(10)

		plan, err := optimizer.OptimizeQuery(q)
		require.NoError(t, err)

		cost := plan.EstimatedCost
		assert.NotNil(t, cost)
		assert.Greater(t, cost.ReadCapacityUnits, 0.0)
		assert.Equal(t, int64(10), cost.EstimatedItemCount) // Limited to 10
		assert.Greater(t, cost.EstimatedDuration, time.Duration(0))
		assert.Equal(t, 0.5, cost.ConfidenceLevel) // Default confidence without history
	})

	t.Run("scan cost estimation", func(t *testing.T) {
		q := New(&TestModel{}, metadata, executor)
		// No conditions - will scan

		plan, err := optimizer.OptimizeQuery(q)
		require.NoError(t, err)

		cost := plan.EstimatedCost
		assert.NotNil(t, cost)
		assert.Greater(t, cost.ReadCapacityUnits, 0.0)
		assert.Greater(t, cost.EstimatedItemCount, int64(0))
		assert.Greater(t, cost.EstimatedScanCount, cost.EstimatedItemCount) // Scan reads more
	})
}

func TestAdaptiveOptimization(t *testing.T) {
	executor := &MockExecutor{}
	metadata := testMetadata()

	t.Run("record execution statistics", func(t *testing.T) {
		optimizer := NewOptimizer(&OptimizationOptions{
			EnableAdaptive: true,
		})
		q := New(&TestModel{}, metadata, executor)
		q.Where("ID", "=", "123")

		// Record multiple executions
		for i := 0; i < 5; i++ {
			result := &QueryExecutionResult{
				Duration:      time.Duration(50+i*10) * time.Millisecond,
				ItemsReturned: 10,
				ItemsScanned:  15,
				Error:         nil,
			}
			optimizer.RecordExecution(q, result)
		}

		// Get statistics
		stats, ok := optimizer.GetStatistics(q)
		require.True(t, ok)

		assert.Equal(t, int64(5), stats.ExecutionCount)
		assert.Equal(t, int64(50), stats.TotalItemsRead)
		assert.Equal(t, int64(75), stats.TotalItemsScanned)
		assert.Equal(t, 50*time.Millisecond, stats.MinDuration)
		assert.Equal(t, 90*time.Millisecond, stats.MaxDuration)
		assert.Equal(t, 0.0, stats.ErrorRate)
	})

	t.Run("adaptive optimization with history", func(t *testing.T) {
		optimizer := NewOptimizer(&OptimizationOptions{
			EnableAdaptive: true,
		})
		q := New(&TestModel{}, metadata, executor)
		q.Where("ID", "=", "789")

		// Record enough executions to build history
		for i := 0; i < 15; i++ {
			result := &QueryExecutionResult{
				Duration:      100 * time.Millisecond,
				ItemsReturned: 5,
				ItemsScanned:  100, // Low efficiency
				Error:         nil,
			}
			optimizer.RecordExecution(q, result)
		}

		// Optimize query - should use historical data
		plan, err := optimizer.OptimizeQuery(q)
		require.NoError(t, err)

		// Should have higher confidence with history
		assert.Greater(t, plan.EstimatedCost.ConfidenceLevel, 0.7)

		// Should warn about low efficiency
		found := false
		for _, hint := range plan.OptimizationHints {
			if strings.Contains(hint, "Low scan efficiency") {
				found = true
				break
			}
		}
		assert.True(t, found, "Expected warning about low scan efficiency")
	})

	t.Run("high error rate detection", func(t *testing.T) {
		optimizer := NewOptimizer(&OptimizationOptions{
			EnableAdaptive: true,
		})
		q := New(&TestModel{}, metadata, executor)
		q.Where("ID", "=", "error-test")

		// Record executions with errors
		for i := 0; i < 10; i++ {
			result := &QueryExecutionResult{
				Duration:      50 * time.Millisecond,
				ItemsReturned: 0,
				ItemsScanned:  0,
				Error:         nil,
			}
			if i%5 == 0 { // 20% error rate
				result.Error = assert.AnError
			}
			optimizer.RecordExecution(q, result)
		}

		// Optimize query
		plan, err := optimizer.OptimizeQuery(q)
		require.NoError(t, err)

		// Should warn about high error rate
		found := false
		for _, hint := range plan.OptimizationHints {
			if hint == "WARNING: High error rate detected. Consider reviewing query conditions." {
				found = true
				break
			}
		}
		assert.True(t, found, "Expected warning about high error rate")
	})
}

func TestOptimizedQueryExecution(t *testing.T) {
	optimizer := NewOptimizer(nil)
	executor := &MockExecutor{
		QueryFunc: func(input *core.CompiledQuery, dest any) error {
			// Mock successful query execution
			return nil
		},
		ScanFunc: func(input *core.CompiledQuery, dest any) error {
			// Mock successful scan execution
			return nil
		},
	}
	metadata := testMetadata()

	t.Run("execute optimized query", func(t *testing.T) {
		q := New(&TestModel{}, metadata, executor)
		q.Where("ID", "=", "123")

		optimizedQuery, err := q.WithOptimizer(optimizer)
		require.NoError(t, err)

		var results []TestModel
		err = optimizedQuery.Execute(&results)
		assert.NoError(t, err)

		// Should have a plan
		plan := optimizedQuery.GetPlan()
		assert.NotNil(t, plan)
		assert.Equal(t, "Query", plan.Operation)
	})

	t.Run("explain plan", func(t *testing.T) {
		q := New(&TestModel{}, metadata, executor)
		q.Where("ID", "=", "123").Select("ID", "Name")

		optimizedQuery, err := q.WithOptimizer(optimizer)
		require.NoError(t, err)

		explanation := optimizedQuery.ExplainPlan()

		assert.Contains(t, explanation, "Query Plan")
		assert.Contains(t, explanation, "Operation: Query")
		assert.Contains(t, explanation, "Cost Estimates")
		assert.Contains(t, explanation, "Optimization Hints")
	})
}

func TestParallelScanOptimization(t *testing.T) {
	optimizer := NewOptimizer(&OptimizationOptions{
		EnableParallel: true,
		MaxParallelism: 4,
	})
	executor := &MockExecutor{}
	metadata := testMetadata()

	t.Run("large result set", func(t *testing.T) {
		q := New(&TestModel{}, metadata, executor)
		// No conditions - will scan

		plan, err := optimizer.OptimizeQuery(q)
		require.NoError(t, err)

		assert.Equal(t, 4, plan.ParallelSegments)
	})

	t.Run("small result set with limit", func(t *testing.T) {
		q := New(&TestModel{}, metadata, executor)
		q.Limit(50) // Small limit

		plan, err := optimizer.OptimizeQuery(q)
		require.NoError(t, err)

		assert.Equal(t, 1, plan.ParallelSegments) // No parallelism for small results
	})

	t.Run("medium result set", func(t *testing.T) {
		q := New(&TestModel{}, metadata, executor)
		q.Limit(500)

		plan, err := optimizer.OptimizeQuery(q)
		require.NoError(t, err)

		assert.LessOrEqual(t, plan.ParallelSegments, 4)
		assert.Greater(t, plan.ParallelSegments, 1)
	})
}

func TestCacheClear(t *testing.T) {
	optimizer := NewOptimizer(nil)
	executor := &MockExecutor{}
	metadata := testMetadata()

	// Create and cache a plan
	q := New(&TestModel{}, metadata, executor)
	q.Where("ID", "=", "123")

	_, err := optimizer.OptimizeQuery(q)
	require.NoError(t, err)

	// Verify plan is cached
	cached, ok := optimizer.GetQueryPlan(q)
	assert.True(t, ok)
	assert.NotNil(t, cached)

	// Clear cache
	optimizer.ClearCache()

	// Verify plan is no longer cached
	cached, ok = optimizer.GetQueryPlan(q)
	assert.False(t, ok)
	assert.Nil(t, cached)
}
