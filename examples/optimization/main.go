package main

import (
	"fmt"
	"time"

	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/query"
)

// UserActivity represents user activity tracking
type UserActivity struct {
	ActivityTime time.Time `theorydb:"sk"`
	Details      map[string]string
	UserID       string `theorydb:"pk"`
	ActivityType string `theorydb:"gsi:TypeIndex:pk"`
	SessionID    string `theorydb:"gsi:SessionIndex:pk"`
	Tags         []string
	Duration     int
}

// MockMetadata implements core.ModelMetadata for testing
type MockMetadata struct{}

func (m *MockMetadata) TableName() string {
	return "user-activities"
}

func (m *MockMetadata) PrimaryKey() core.KeySchema {
	return core.KeySchema{
		PartitionKey: "UserID",
		SortKey:      "ActivityTime",
	}
}

func (m *MockMetadata) Indexes() []core.IndexSchema {
	return []core.IndexSchema{
		{
			Name:           "TypeIndex",
			Type:           "GSI",
			PartitionKey:   "ActivityType",
			ProjectionType: "ALL",
		},
		{
			Name:           "SessionIndex",
			Type:           "GSI",
			PartitionKey:   "SessionID",
			SortKey:        "ActivityTime",
			ProjectionType: "ALL",
		},
	}
}

func (m *MockMetadata) AttributeMetadata(field string) *core.AttributeMetadata {
	return nil
}

func (m *MockMetadata) VersionFieldName() string {
	return ""
}

// MockExecutor implements query.QueryExecutor for testing
type MockExecutor struct{}

func (m *MockExecutor) ExecuteQuery(input *core.CompiledQuery, dest any) error {
	return nil
}

func (m *MockExecutor) ExecuteScan(input *core.CompiledQuery, dest any) error {
	return nil
}

func main() {
	// Create the query optimizer with custom options
	optimizer := query.NewOptimizer(&query.OptimizationOptions{
		EnableAdaptive: true,
		EnableParallel: true,
		MaxParallelism: 4,
		PlanCacheTTL:   30 * time.Minute,
		MaxCacheSize:   500,
	})

	// Create mock metadata and executor
	metadata := &MockMetadata{}
	executor := &MockExecutor{}

	fmt.Println("=== TableTheory Query Optimizer Examples ===")

	// Example 1: Efficient query with partition key
	fmt.Println("Example 1: Efficient Query with Partition Key")
	q1 := query.New(&UserActivity{}, metadata, executor)
	q1.Where("UserID", "=", "user123").
		Where("ActivityTime", ">", time.Now().Add(-24*time.Hour))
	explainQuery(q1, optimizer)

	// Example 2: Inefficient query without partition key
	fmt.Println("\nExample 2: Inefficient Query (Missing Partition Key)")
	q2 := query.New(&UserActivity{}, metadata, executor)
	q2.Where("ActivityType", "=", "login")
	explainQuery(q2, optimizer)

	// Example 3: Scan operation
	fmt.Println("\nExample 3: Table Scan")
	q3 := query.New(&UserActivity{}, metadata, executor)
	q3.Filter("Duration", ">", 300)
	explainQuery(q3, optimizer)

	// Example 4: Query with projection
	fmt.Println("\nExample 4: Query with Projection")
	q4 := query.New(&UserActivity{}, metadata, executor)
	q4.Where("UserID", "=", "user456").
		Select("UserID", "ActivityTime", "ActivityType")
	explainQuery(q4, optimizer)

	// Example 5: Query with limit
	fmt.Println("\nExample 5: Limited Query")
	q5 := query.New(&UserActivity{}, metadata, executor)
	q5.Where("UserID", "=", "user789").
		Limit(10).
		OrderBy("ActivityTime", "desc")
	explainQuery(q5, optimizer)

	// Example 6: Query missing sort key
	fmt.Println("\nExample 6: Query Missing Sort Key")
	q6 := query.New(&UserActivity{}, metadata, executor)
	q6.Where("UserID", "=", "user999")
	explainQuery(q6, optimizer)

	// Example 7: Inefficient operator usage
	fmt.Println("\nExample 7: Inefficient Operator Usage")
	q7 := query.New(&UserActivity{}, metadata, executor)
	q7.Where("UserID", "CONTAINS", "user")
	explainQuery(q7, optimizer)

	// Example 8: Simulating adaptive optimization
	fmt.Println("\nExample 8: Adaptive Optimization Demo")
	simulateAdaptiveOptimization(metadata, executor, optimizer)
}

// explainQuery runs the optimizer on a query and explains the plan
func explainQuery(q *query.Query, optimizer *query.QueryOptimizer) {
	optimizedQuery, err := q.WithOptimizer(optimizer)
	if err != nil {
		fmt.Printf("Error optimizing query: %v\n", err)
		return
	}

	fmt.Println(optimizedQuery.ExplainPlan())
}

// simulateAdaptiveOptimization demonstrates how the optimizer learns from execution history
func simulateAdaptiveOptimization(metadata core.ModelMetadata, executor query.QueryExecutor, optimizer *query.QueryOptimizer) {
	q := query.New(&UserActivity{}, metadata, executor)
	q.Where("UserID", "=", "adaptive-test")

	// First optimization - no history
	fmt.Println("Initial optimization (no history):")
	optimized1, _ := q.WithOptimizer(optimizer)
	fmt.Println(optimized1.ExplainPlan())

	// Simulate multiple executions with varying performance
	fmt.Println("\nSimulating 20 query executions...")
	for i := 0; i < 20; i++ {
		result := &query.QueryExecutionResult{
			Duration:      time.Duration(50+i*5) * time.Millisecond,
			ItemsReturned: 10,
			ItemsScanned:  100, // Inefficient scan ratio
			Error:         nil,
		}

		// Record execution
		optimizer.RecordExecution(q, result)

		if i == 9 {
			// Simulate an error
			result.Error = fmt.Errorf("throttled")
		}
	}

	// Get statistics
	stats, ok := optimizer.GetStatistics(q)
	if ok {
		fmt.Printf("\nQuery Statistics:\n")
		fmt.Printf("  Executions: %d\n", stats.ExecutionCount)
		fmt.Printf("  Average Duration: %v\n", stats.AverageDuration)
		fmt.Printf("  Min Duration: %v\n", stats.MinDuration)
		fmt.Printf("  Max Duration: %v\n", stats.MaxDuration)
		fmt.Printf("  Error Rate: %.2f%%\n", stats.ErrorRate*100)
		fmt.Printf("  Scan Efficiency: %.2f%%\n",
			float64(stats.TotalItemsRead)/float64(stats.TotalItemsScanned)*100)
	}

	// Re-optimize with history
	fmt.Println("\nRe-optimization with execution history:")
	optimized2, _ := q.WithOptimizer(optimizer)
	fmt.Println(optimized2.ExplainPlan())

	// Clear cache and re-optimize
	fmt.Println("\nAfter clearing cache:")
	optimizer.ClearCache()
	optimized3, _ := q.WithOptimizer(optimizer)
	fmt.Println(optimized3.ExplainPlan())
}

// Expected output would show optimization hints like:
//
// Query Plan for user-activities||UserID:=:
//   Operation: Query
//
//   Cost Estimates:
//     Read Capacity Units: 12.50
//     Estimated Items: 100
//     Estimated Duration: 50ms
//     Confidence: 50%
//
//   Optimization Hints:
//     - TIP: Consider adding sort key condition for more efficient queries
//     - TIP: Use Select() to project only needed attributes and reduce data transfer
//
// After simulating executions with historical data:
//
//   Cost Estimates:
//     Read Capacity Units: 1.25
//     Estimated Items: 10
//     Estimated Duration: 70ms
//     Confidence: 80%
//
//   Optimization Hints:
//     - WARNING: Low scan efficiency (10.0%). Consider adding more selective filters.
//     - TIP: Consider adding sort key condition for more efficient queries
//     - TIP: Use Select() to project only needed attributes and reduce data transfer
