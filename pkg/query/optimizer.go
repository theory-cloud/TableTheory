package query

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/theory-cloud/tabletheory/internal/numutil"
)

// QueryOptimizer provides query optimization capabilities
type QueryOptimizer struct {
	planCache         sync.Map
	queryStats        sync.Map
	planCacheTTL      time.Duration
	maxParallelism    int
	cacheSize         int
	cachePlanDuration time.Duration
	enableAdaptive    bool
	enableParallel    bool
}

// QueryPlan represents an optimized query execution plan
type QueryPlan struct {
	CachedAt          time.Time
	EstimatedCost     *CostEstimate
	Statistics        *QueryStatistics
	ID                string
	Operation         string
	IndexName         string
	Projections       []string
	OptimizationHints []string
	ParallelSegments  int
}

// CostEstimate represents the estimated cost of a query
type CostEstimate struct {
	ReadCapacityUnits  float64
	WriteCapacityUnits float64
	EstimatedItemCount int64
	EstimatedScanCount int64
	EstimatedDuration  time.Duration
	ConfidenceLevel    float64 // 0.0 to 1.0
}

// QueryStatistics tracks runtime statistics for queries
type QueryStatistics struct {
	LastExecuted      time.Time
	ExecutionCount    int64
	ErrorCount        int64
	TotalDuration     time.Duration
	AverageDuration   time.Duration
	MinDuration       time.Duration
	MaxDuration       time.Duration
	TotalItemsRead    int64
	TotalItemsScanned int64
	ErrorRate         float64
}

// OptimizationOptions configures the optimizer behavior
type OptimizationOptions struct {
	EnableAdaptive bool
	EnableParallel bool
	MaxParallelism int
	PlanCacheTTL   time.Duration
	MaxCacheSize   int
}

// NewOptimizer creates a new query optimizer
func NewOptimizer(opts *OptimizationOptions) *QueryOptimizer {
	if opts == nil {
		opts = &OptimizationOptions{
			EnableAdaptive: true,
			EnableParallel: true,
			MaxParallelism: runtime.NumCPU(),
			PlanCacheTTL:   1 * time.Hour,
			MaxCacheSize:   1000,
		}
	}

	return &QueryOptimizer{
		planCacheTTL:      opts.PlanCacheTTL,
		enableAdaptive:    opts.EnableAdaptive,
		enableParallel:    opts.EnableParallel,
		maxParallelism:    opts.MaxParallelism,
		cacheSize:         opts.MaxCacheSize,
		cachePlanDuration: opts.PlanCacheTTL,
	}
}

// OptimizeQuery analyzes a query and returns an optimized execution plan
func (o *QueryOptimizer) OptimizeQuery(q *Query) (*QueryPlan, error) {
	// Generate plan ID from query characteristics
	planID := o.generatePlanID(q)

	// Check cache first
	if cached, ok := o.planCache.Load(planID); ok {
		plan, ok := cached.(*QueryPlan)
		if !ok {
			o.planCache.Delete(planID)
		} else if time.Since(plan.CachedAt) < o.planCacheTTL {
			return plan, nil
		}
	}

	// Analyze query pattern
	plan := &QueryPlan{
		ID:                planID,
		OptimizationHints: []string{},
		CachedAt:          time.Now(),
	}

	// Determine operation type and analyze
	if len(q.conditions) > 0 {
		plan.Operation = operationQuery
		o.analyzeQueryConditions(q, plan)
	} else {
		plan.Operation = operationScan
		o.analyzeScanOperation(q, plan)
	}

	// Estimate costs
	plan.EstimatedCost = o.estimateCost(q, plan)

	// Apply adaptive optimizations if enabled
	if o.enableAdaptive {
		o.applyAdaptiveOptimizations(q, plan)
	}

	// Cache the plan
	o.planCache.Store(planID, plan)

	return plan, nil
}

// analyzeQueryConditions analyzes query conditions and suggests optimizations
func (o *QueryOptimizer) analyzeQueryConditions(q *Query, plan *QueryPlan) {
	// Check if query uses the partition key
	primaryKey := q.metadata.PrimaryKey()
	hasPartitionKey := false
	hasSortKey := false

	for _, cond := range q.conditions {
		if cond.Field == primaryKey.PartitionKey {
			hasPartitionKey = true
		}
		if primaryKey.SortKey != "" && cond.Field == primaryKey.SortKey {
			hasSortKey = true
		}
	}

	// Analyze index usage
	if !hasPartitionKey {
		// Check if any GSI could be used
		indexSuggestions := o.suggestIndexes(q)
		if len(indexSuggestions) > 0 {
			plan.OptimizationHints = append(plan.OptimizationHints,
				fmt.Sprintf("Consider using index: %s", indexSuggestions[0]))
			plan.IndexName = indexSuggestions[0]
		} else {
			plan.OptimizationHints = append(plan.OptimizationHints,
				"WARNING: Query will result in full table scan. Consider adding a GSI.")
		}
	}

	// Check for sort key usage
	if hasPartitionKey && primaryKey.SortKey != "" && !hasSortKey {
		plan.OptimizationHints = append(plan.OptimizationHints,
			"TIP: Consider adding sort key condition for more efficient queries")
	}

	// Check for inefficient operators
	for _, cond := range q.conditions {
		if cond.Operator == "CONTAINS" && cond.Field == primaryKey.PartitionKey {
			plan.OptimizationHints = append(plan.OptimizationHints,
				"WARNING: CONTAINS operator on partition key is inefficient")
		}
	}

	// Suggest projection optimization
	if len(q.projection) == 0 {
		plan.OptimizationHints = append(plan.OptimizationHints,
			"TIP: Use Select() to project only needed attributes and reduce data transfer")
	}
}

// analyzeScanOperation analyzes scan operations and suggests optimizations
func (o *QueryOptimizer) analyzeScanOperation(q *Query, plan *QueryPlan) {
	// For scans, always suggest using queries if possible
	plan.OptimizationHints = append(plan.OptimizationHints,
		"WARNING: Scan operation will read entire table. Consider using Query with appropriate conditions.")

	// Suggest parallel scan for large tables
	if o.enableParallel {
		plan.ParallelSegments = o.calculateOptimalSegments(q)
		if plan.ParallelSegments > 1 {
			plan.OptimizationHints = append(plan.OptimizationHints,
				fmt.Sprintf("TIP: Use parallel scan with %d segments for better performance", plan.ParallelSegments))
		}
	}

	// Check filters efficiency
	if q.builder != nil && len(q.builder.Build().FilterExpression) > 0 {
		plan.OptimizationHints = append(plan.OptimizationHints,
			"INFO: Filters are applied after data retrieval. Consider using Query conditions instead.")
	}
}

// estimateCost estimates the cost of executing a query
func (o *QueryOptimizer) estimateCost(q *Query, plan *QueryPlan) *CostEstimate {
	estimate := &CostEstimate{
		ConfidenceLevel: 0.5, // Default medium confidence
	}

	// Get historical statistics if available
	statsKey := o.generateStatsKey(q)
	if stats, ok := o.queryStats.Load(statsKey); ok {
		queryStats, ok := stats.(*QueryStatistics)
		if !ok {
			o.queryStats.Delete(statsKey)
		} else if queryStats.ExecutionCount > 10 {
			// Use historical data for more accurate estimates
			estimate.ConfidenceLevel = 0.8
			estimate.EstimatedDuration = queryStats.AverageDuration
			avgItemsPerExecution := queryStats.TotalItemsRead / queryStats.ExecutionCount
			estimate.EstimatedItemCount = avgItemsPerExecution
			avgScannedPerExecution := queryStats.TotalItemsScanned / queryStats.ExecutionCount
			estimate.EstimatedScanCount = avgScannedPerExecution
		}
	}

	// If no historical data, use heuristics first
	if estimate.EstimatedItemCount == 0 {
		o.applyHeuristicEstimates(q, plan, estimate)
	}

	// Calculate capacity units
	if plan.Operation == operationQuery {
		// Queries are more efficient
		estimate.ReadCapacityUnits = float64(estimate.EstimatedItemCount) * 0.5 / 4.0 // 4KB per RCU
	} else {
		// Scans consume more capacity
		estimate.ReadCapacityUnits = float64(estimate.EstimatedScanCount) * 0.5 / 4.0
	}

	return estimate
}

// applyHeuristicEstimates applies rule-based estimates when no historical data exists
func (o *QueryOptimizer) applyHeuristicEstimates(q *Query, plan *QueryPlan, estimate *CostEstimate) {
	// Base estimates
	if plan.Operation == operationQuery {
		estimate.EstimatedItemCount = 100
		estimate.EstimatedScanCount = 100
		estimate.EstimatedDuration = 50 * time.Millisecond
	} else {
		estimate.EstimatedItemCount = 1000
		estimate.EstimatedScanCount = 10000
		estimate.EstimatedDuration = 500 * time.Millisecond
	}

	// Adjust for limit
	if q.limit > 0 && int64(q.limit) < estimate.EstimatedItemCount {
		ratio := float64(q.limit) / float64(estimate.EstimatedItemCount)
		estimate.EstimatedItemCount = int64(q.limit)
		estimate.EstimatedDuration = time.Duration(float64(estimate.EstimatedDuration) * ratio)
	}

	// Adjust for parallel scan
	if plan.ParallelSegments > 1 {
		estimate.EstimatedDuration /= time.Duration(plan.ParallelSegments)
	}
}

// suggestIndexes suggests appropriate indexes for the query
func (o *QueryOptimizer) suggestIndexes(_ *Query) []string {
	suggestions := []string{}

	// Get available indexes from metadata
	// This would need to be extended with actual index metadata
	// For now, return empty suggestions

	return suggestions
}

// calculateOptimalSegments calculates optimal number of parallel segments
func (o *QueryOptimizer) calculateOptimalSegments(q *Query) int {
	// Start with CPU count
	segments := o.maxParallelism

	// Adjust based on limit
	if q.limit > 0 {
		// For small result sets, reduce parallelism
		if q.limit < 100 {
			segments = 1
		} else if q.limit < 1000 {
			segments = minInt(segments, 4)
		}
	}

	return segments
}

// applyAdaptiveOptimizations applies runtime-based optimizations
func (o *QueryOptimizer) applyAdaptiveOptimizations(q *Query, plan *QueryPlan) {
	statsKey := o.generateStatsKey(q)
	if stats, ok := o.queryStats.Load(statsKey); ok {
		queryStats, ok := stats.(*QueryStatistics)
		if !ok {
			o.queryStats.Delete(statsKey)
			return
		}

		// If error rate is high, suggest different approach
		if queryStats.ErrorRate > 0.1 {
			plan.OptimizationHints = append(plan.OptimizationHints,
				"WARNING: High error rate detected. Consider reviewing query conditions.")
		}

		// If scan efficiency is low, suggest optimization
		if queryStats.TotalItemsScanned > 0 {
			efficiency := float64(queryStats.TotalItemsRead) / float64(queryStats.TotalItemsScanned)
			if efficiency < 0.1 {
				plan.OptimizationHints = append(plan.OptimizationHints,
					fmt.Sprintf("WARNING: Low scan efficiency (%.1f%%). Consider adding more selective filters.", efficiency*100))
			}
		}
	}
}

// RecordExecution records the execution statistics for adaptive optimization
func (o *QueryOptimizer) RecordExecution(q *Query, result *QueryExecutionResult) {
	if !o.enableAdaptive {
		return
	}

	statsKey := o.generateStatsKey(q)

	// Load or create statistics
	var stats *QueryStatistics
	if existing, ok := o.queryStats.Load(statsKey); ok {
		existingStats, ok := existing.(*QueryStatistics)
		if !ok {
			o.queryStats.Delete(statsKey)
		} else if existingStats.MinDuration == 0 {
			existingStats.MinDuration = result.Duration
		}
		stats = existingStats
	}

	if stats == nil {
		stats = &QueryStatistics{
			MinDuration: result.Duration,
		}
	}

	// Update statistics
	stats.ExecutionCount++
	stats.TotalDuration += result.Duration
	stats.AverageDuration = stats.TotalDuration / time.Duration(stats.ExecutionCount)
	stats.TotalItemsRead += result.ItemsReturned
	stats.TotalItemsScanned += result.ItemsScanned
	stats.LastExecuted = time.Now()

	if result.Duration < stats.MinDuration {
		stats.MinDuration = result.Duration
	}
	if result.Duration > stats.MaxDuration {
		stats.MaxDuration = result.Duration
	}

	if result.Error != nil {
		stats.ErrorCount++
	}
	stats.ErrorRate = float64(stats.ErrorCount) / float64(stats.ExecutionCount)

	// Store updated statistics
	o.queryStats.Store(statsKey, stats)
}

// GetQueryPlan returns the cached query plan if available
func (o *QueryOptimizer) GetQueryPlan(q *Query) (*QueryPlan, bool) {
	planID := o.generatePlanID(q)
	if cached, ok := o.planCache.Load(planID); ok {
		plan, ok := cached.(*QueryPlan)
		if !ok {
			o.planCache.Delete(planID)
		} else if time.Since(plan.CachedAt) < o.planCacheTTL {
			return plan, true
		}
	}
	return nil, false
}

// ClearCache clears the query plan cache
func (o *QueryOptimizer) ClearCache() {
	o.planCache = sync.Map{}
}

// GetStatistics returns query execution statistics
func (o *QueryOptimizer) GetStatistics(q *Query) (*QueryStatistics, bool) {
	statsKey := o.generateStatsKey(q)
	if stats, ok := o.queryStats.Load(statsKey); ok {
		queryStats, ok := stats.(*QueryStatistics)
		if !ok {
			o.queryStats.Delete(statsKey)
			return nil, false
		}
		return queryStats, true
	}
	return nil, false
}

// generatePlanID generates a unique ID for a query plan
func (o *QueryOptimizer) generatePlanID(q *Query) string {
	parts := []string{
		q.metadata.TableName(),
		q.index,
	}

	// Add conditions
	for _, cond := range q.conditions {
		parts = append(parts, fmt.Sprintf("%s:%s", cond.Field, cond.Operator))
	}

	// Add projections
	if len(q.projection) > 0 {
		parts = append(parts, "proj:"+strings.Join(q.projection, ","))
	}

	return strings.Join(parts, "|")
}

// generateStatsKey generates a key for storing query statistics
func (o *QueryOptimizer) generateStatsKey(q *Query) string {
	// Similar to plan ID but without specific values
	parts := make([]string, 0, 3)
	parts = append(parts, q.metadata.TableName(), q.index)

	// Add condition fields only (not values)
	condFields := make([]string, 0, len(q.conditions))
	for _, cond := range q.conditions {
		condFields = append(condFields, cond.Field)
	}
	parts = append(parts, "fields:"+strings.Join(condFields, ","))

	return strings.Join(parts, "|")
}

// QueryExecutionResult represents the result of a query execution
type QueryExecutionResult struct {
	Error         error
	Duration      time.Duration
	ItemsReturned int64
	ItemsScanned  int64
}

// OptimizedQuery wraps a Query with optimization capabilities
type OptimizedQuery struct {
	*Query
	optimizer *QueryOptimizer
	plan      *QueryPlan
}

// WithOptimizer creates an optimized query wrapper
func (q *Query) WithOptimizer(optimizer *QueryOptimizer) (*OptimizedQuery, error) {
	plan, err := optimizer.OptimizeQuery(q)
	if err != nil {
		return nil, err
	}

	return &OptimizedQuery{
		Query:     q,
		optimizer: optimizer,
		plan:      plan,
	}, nil
}

// Execute executes the query with optimizations applied
func (oq *OptimizedQuery) Execute(dest any) error {
	start := time.Now()

	// Apply optimization hints
	if oq.plan.IndexName != "" && oq.plan.IndexName != oq.index {
		oq.Index(oq.plan.IndexName)
	}

	// Execute based on operation type
	var err error
	var itemsReturned, itemsScanned int64

	switch oq.plan.Operation {
	case operationQuery:
		err = oq.All(dest)
	case operationScan:
		if oq.plan.ParallelSegments > 1 {
			err = oq.ScanAllSegments(dest, numutil.ClampIntToInt32(oq.plan.ParallelSegments))
		} else {
			err = oq.Scan(dest)
		}
	default:
		err = fmt.Errorf("unsupported operation: %s", oq.plan.Operation)
	}

	// Record execution statistics
	duration := time.Since(start)
	result := &QueryExecutionResult{
		Duration:      duration,
		ItemsReturned: itemsReturned,
		ItemsScanned:  itemsScanned,
		Error:         err,
	}

	oq.optimizer.RecordExecution(oq.Query, result)

	return err
}

// GetPlan returns the optimization plan
func (oq *OptimizedQuery) GetPlan() *QueryPlan {
	return oq.plan
}

// ExplainPlan returns a human-readable explanation of the query plan
func (oq *OptimizedQuery) ExplainPlan() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Query Plan for %s:\n", oq.plan.ID))
	sb.WriteString(fmt.Sprintf("  Operation: %s\n", oq.plan.Operation))

	if oq.plan.IndexName != "" {
		sb.WriteString(fmt.Sprintf("  Index: %s\n", oq.plan.IndexName))
	}

	if oq.plan.ParallelSegments > 1 {
		sb.WriteString(fmt.Sprintf("  Parallel Segments: %d\n", oq.plan.ParallelSegments))
	}

	if oq.plan.EstimatedCost != nil {
		sb.WriteString("\n  Cost Estimates:\n")
		sb.WriteString(fmt.Sprintf("    Read Capacity Units: %.2f\n", oq.plan.EstimatedCost.ReadCapacityUnits))
		sb.WriteString(fmt.Sprintf("    Estimated Items: %d\n", oq.plan.EstimatedCost.EstimatedItemCount))
		sb.WriteString(fmt.Sprintf("    Estimated Duration: %v\n", oq.plan.EstimatedCost.EstimatedDuration))
		sb.WriteString(fmt.Sprintf("    Confidence: %.0f%%\n", oq.plan.EstimatedCost.ConfidenceLevel*100))
	}

	if len(oq.plan.OptimizationHints) > 0 {
		sb.WriteString("\n  Optimization Hints:\n")
		for _, hint := range oq.plan.OptimizationHints {
			sb.WriteString(fmt.Sprintf("    - %s\n", hint))
		}
	}

	return sb.String()
}

// Helper function for min
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
