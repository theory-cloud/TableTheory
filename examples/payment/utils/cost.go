package utils

import (
	"fmt"
	"math"
	"time"
)

// CostEstimator estimates AWS DynamoDB costs
type CostEstimator struct {
	// Pricing per region (using us-east-1 as default)
	readCostPerUnit     float64 // Cost per RCU per month
	writeCostPerUnit    float64 // Cost per WCU per month
	storagePerGB        float64 // Cost per GB per month
	onDemandReadCost    float64 // Cost per million read requests
	onDemandWriteCost   float64 // Cost per million write requests
	gsiReadCostPerUnit  float64 // GSI read cost per RCU
	gsiWriteCostPerUnit float64 // GSI write cost per WCU
	streamsCost         float64 // DynamoDB Streams per million requests
	backupCostPerGB     float64 // On-demand backup cost per GB
}

// NewCostEstimator creates a new cost estimator with default pricing
func NewCostEstimator() *CostEstimator {
	return &CostEstimator{
		// AWS DynamoDB pricing for us-east-1 (as of 2024)
		readCostPerUnit:     0.00013, // per RCU per hour
		writeCostPerUnit:    0.00065, // per WCU per hour
		storagePerGB:        0.25,    // per month
		onDemandReadCost:    0.25,    // per million requests
		onDemandWriteCost:   1.25,    // per million requests
		gsiReadCostPerUnit:  0.00013, // per RCU per hour
		gsiWriteCostPerUnit: 0.00065, // per WCU per hour
		streamsCost:         0.02,    // per 100,000 stream read requests
		backupCostPerGB:     0.10,    // per month
	}
}

// Metrics represents usage metrics for cost calculation
type Metrics struct {
	GSIReadCapacityUnits  int
	GSIWriteCapacityUnits int
	StorageGB             float64
	ItemCount             int64
	AverageItemSizeKB     float64
	MonthlyReadRequests   int64
	MonthlyWriteRequests  int64
	GSICount              int
	WriteCapacityUnits    int
	RegionCount           int
	ReadCapacityUnits     int
	StreamReadRequests    int64
	PeakHoursPerDay       int
	BackupStorageGB       float64
	BackupEnabled         bool
	IsMultiRegion         bool
	StreamsEnabled        bool
}

// CostBreakdown provides detailed cost breakdown
type CostBreakdown struct {
	Details          map[string]float64 `json:"details"`
	ReadCost         float64            `json:"read_cost"`
	WriteCost        float64            `json:"write_cost"`
	StorageCost      float64            `json:"storage_cost"`
	GSICost          float64            `json:"gsi_cost"`
	StreamsCost      float64            `json:"streams_cost"`
	BackupCost       float64            `json:"backup_cost"`
	TotalMonthlyCost float64            `json:"total_monthly_cost"`
	TotalYearlyCost  float64            `json:"total_yearly_cost"`
	CostPerItem      float64            `json:"cost_per_item"`
	CostPerRequest   float64            `json:"cost_per_request"`
}

// EstimateMonthly calculates monthly DynamoDB costs
func (c *CostEstimator) EstimateMonthly(metrics Metrics) *CostBreakdown {
	breakdown := &CostBreakdown{
		Details: make(map[string]float64),
	}

	// Calculate based on billing mode
	if metrics.ReadCapacityUnits > 0 || metrics.WriteCapacityUnits > 0 {
		// Provisioned capacity mode
		breakdown.ReadCost = c.calculateProvisionedReadCost(metrics)
		breakdown.WriteCost = c.calculateProvisionedWriteCost(metrics)
		breakdown.Details["provisioned_mode"] = 1
	} else {
		// On-demand mode
		breakdown.ReadCost = c.calculateOnDemandReadCost(metrics)
		breakdown.WriteCost = c.calculateOnDemandWriteCost(metrics)
		breakdown.Details["on_demand_mode"] = 1
	}

	// Storage cost
	breakdown.StorageCost = metrics.StorageGB * c.storagePerGB
	breakdown.Details["storage_gb"] = metrics.StorageGB

	// GSI cost
	if metrics.GSICount > 0 {
		breakdown.GSICost = c.calculateGSICost(metrics)
		breakdown.Details["gsi_count"] = float64(metrics.GSICount)
	}

	// Streams cost
	if metrics.StreamsEnabled {
		breakdown.StreamsCost = c.calculateStreamsCost(metrics)
		breakdown.Details["streams_requests"] = float64(metrics.StreamReadRequests)
	}

	// Backup cost
	if metrics.BackupEnabled {
		breakdown.BackupCost = metrics.BackupStorageGB * c.backupCostPerGB
		breakdown.Details["backup_gb"] = metrics.BackupStorageGB
	}

	// Multi-region costs
	regionMultiplier := 1.0
	if metrics.IsMultiRegion && metrics.RegionCount > 1 {
		regionMultiplier = float64(metrics.RegionCount)
		breakdown.Details["region_count"] = float64(metrics.RegionCount)
	}

	// Calculate totals
	breakdown.TotalMonthlyCost = (breakdown.ReadCost + breakdown.WriteCost +
		breakdown.StorageCost + breakdown.GSICost +
		breakdown.StreamsCost + breakdown.BackupCost) * regionMultiplier

	breakdown.TotalYearlyCost = breakdown.TotalMonthlyCost * 12

	// Per-item and per-request costs
	if metrics.ItemCount > 0 {
		breakdown.CostPerItem = breakdown.TotalMonthlyCost / float64(metrics.ItemCount)
	}

	totalRequests := metrics.MonthlyReadRequests + metrics.MonthlyWriteRequests
	if totalRequests > 0 {
		breakdown.CostPerRequest = breakdown.TotalMonthlyCost / float64(totalRequests)
	}

	return breakdown
}

// EstimatePaymentPlatformCosts estimates costs for a payment platform
func (c *CostEstimator) EstimatePaymentPlatformCosts(
	monthlyTransactions int64,
	averageQueriesPerTransaction float64,
	retentionDays int,
) *CostBreakdown {
	// Calculate metrics based on payment platform patterns
	metrics := Metrics{
		// Use on-demand for payment platforms (better for variable load)
		MonthlyReadRequests:  int64(float64(monthlyTransactions) * averageQueriesPerTransaction),
		MonthlyWriteRequests: monthlyTransactions * 3, // payment + transaction + audit

		// Storage calculation
		ItemCount:         monthlyTransactions * 3, // payments, transactions, audits
		AverageItemSizeKB: 2.0,                     // Average payment record size

		// GSIs for payment queries
		GSICount: 3, // merchant, customer, idempotency

		// Enable streams for webhooks
		StreamsEnabled:     true,
		StreamReadRequests: monthlyTransactions, // One webhook per transaction

		// Backup for compliance
		BackupEnabled: true,
	}

	// Calculate storage
	monthlyDataGB := float64(metrics.ItemCount) * metrics.AverageItemSizeKB / 1024 / 1024
	metrics.StorageGB = monthlyDataGB * float64(retentionDays) / 30
	metrics.BackupStorageGB = metrics.StorageGB * 0.5 // Assume 50% backup size

	return c.EstimateMonthly(metrics)
}

// Helper methods

func (c *CostEstimator) calculateProvisionedReadCost(metrics Metrics) float64 {
	hoursPerMonth := 24 * 30
	return float64(metrics.ReadCapacityUnits) * c.readCostPerUnit * float64(hoursPerMonth)
}

func (c *CostEstimator) calculateProvisionedWriteCost(metrics Metrics) float64 {
	hoursPerMonth := 24 * 30
	return float64(metrics.WriteCapacityUnits) * c.writeCostPerUnit * float64(hoursPerMonth)
}

func (c *CostEstimator) calculateOnDemandReadCost(metrics Metrics) float64 {
	millionRequests := float64(metrics.MonthlyReadRequests) / 1_000_000
	return millionRequests * c.onDemandReadCost
}

func (c *CostEstimator) calculateOnDemandWriteCost(metrics Metrics) float64 {
	millionRequests := float64(metrics.MonthlyWriteRequests) / 1_000_000
	return millionRequests * c.onDemandWriteCost
}

func (c *CostEstimator) calculateGSICost(metrics Metrics) float64 {
	hoursPerMonth := 24 * 30
	readCost := float64(metrics.GSIReadCapacityUnits) * c.gsiReadCostPerUnit * float64(hoursPerMonth)
	writeCost := float64(metrics.GSIWriteCapacityUnits) * c.gsiWriteCostPerUnit * float64(hoursPerMonth)

	// For on-demand, estimate GSI costs as 1.25x table costs
	if metrics.ReadCapacityUnits == 0 {
		gsiMultiplier := 1.25 * float64(metrics.GSICount)
		return (c.calculateOnDemandReadCost(metrics) + c.calculateOnDemandWriteCost(metrics)) * gsiMultiplier * 0.2
	}

	return readCost + writeCost
}

func (c *CostEstimator) calculateStreamsCost(metrics Metrics) float64 {
	hundredThousandRequests := float64(metrics.StreamReadRequests) / 100_000
	return hundredThousandRequests * c.streamsCost
}

// OptimizationRecommendations provides cost optimization suggestions
type OptimizationRecommendations struct {
	Recommendations  []Recommendation `json:"recommendations"`
	PotentialSavings float64          `json:"potential_savings"`
}

// Recommendation represents a cost optimization suggestion
type Recommendation struct {
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Impact      string  `json:"impact"`
	Effort      string  `json:"effort"`
	Savings     float64 `json:"estimated_savings"`
}

// GetOptimizationRecommendations analyzes metrics and provides recommendations
func (c *CostEstimator) GetOptimizationRecommendations(metrics Metrics, breakdown *CostBreakdown) *OptimizationRecommendations {
	recommendations := &OptimizationRecommendations{
		Recommendations: []Recommendation{},
	}

	// Check if on-demand would be cheaper than provisioned
	if metrics.ReadCapacityUnits > 0 {
		onDemandMetrics := metrics
		onDemandMetrics.ReadCapacityUnits = 0
		onDemandMetrics.WriteCapacityUnits = 0
		onDemandBreakdown := c.EstimateMonthly(onDemandMetrics)

		if onDemandBreakdown.TotalMonthlyCost < breakdown.TotalMonthlyCost*0.8 {
			savings := breakdown.TotalMonthlyCost - onDemandBreakdown.TotalMonthlyCost
			recommendations.Recommendations = append(recommendations.Recommendations, Recommendation{
				Title:       "Switch to On-Demand Billing",
				Description: "Your usage pattern suggests on-demand billing would be more cost-effective",
				Impact:      "high",
				Savings:     savings,
				Effort:      "low",
			})
			recommendations.PotentialSavings += savings
		}
	}

	// Check for over-provisioned capacity
	if metrics.ReadCapacityUnits > 0 && metrics.MonthlyReadRequests > 0 {
		requiredRCU := int(math.Ceil(float64(metrics.MonthlyReadRequests) / (30 * 24 * 3600) * 2)) // 2KB items
		if requiredRCU < metrics.ReadCapacityUnits/2 {
			savings := breakdown.ReadCost * 0.5
			recommendations.Recommendations = append(recommendations.Recommendations, Recommendation{
				Title:       "Reduce Read Capacity",
				Description: fmt.Sprintf("Current RCU: %d, Required: %d", metrics.ReadCapacityUnits, requiredRCU),
				Impact:      "medium",
				Savings:     savings,
				Effort:      "low",
			})
			recommendations.PotentialSavings += savings
		}
	}

	// Check for expensive GSIs
	if metrics.GSICount > 2 && breakdown.GSICost > breakdown.TotalMonthlyCost*0.3 {
		savings := breakdown.GSICost * 0.3
		recommendations.Recommendations = append(recommendations.Recommendations, Recommendation{
			Title:       "Optimize Global Secondary Indexes",
			Description: "GSI costs are over 30% of total. Consider consolidating or removing unused indexes",
			Impact:      "high",
			Savings:     savings,
			Effort:      "medium",
		})
		recommendations.PotentialSavings += savings
	}

	// Check backup strategy
	if metrics.BackupEnabled && metrics.BackupStorageGB > metrics.StorageGB*2 {
		savings := breakdown.BackupCost * 0.5
		recommendations.Recommendations = append(recommendations.Recommendations, Recommendation{
			Title:       "Optimize Backup Retention",
			Description: "Backup storage exceeds 2x table size. Consider reducing retention period",
			Impact:      "low",
			Savings:     savings,
			Effort:      "low",
		})
		recommendations.PotentialSavings += savings
	}

	// TTL recommendation for old data
	if metrics.ItemCount > 1_000_000 {
		savings := breakdown.StorageCost * 0.2
		recommendations.Recommendations = append(recommendations.Recommendations, Recommendation{
			Title:       "Implement TTL for Old Records",
			Description: "Use DynamoDB TTL to automatically delete old payment records",
			Impact:      "medium",
			Savings:     savings,
			Effort:      "medium",
		})
		recommendations.PotentialSavings += savings
	}

	return recommendations
}

// FormatCostReport generates a human-readable cost report
func FormatCostReport(breakdown *CostBreakdown, recommendations *OptimizationRecommendations) string {
	report := fmt.Sprintf(`
DynamoDB Cost Analysis Report
=============================
Generated: %s

Monthly Cost Breakdown:
----------------------
Read Operations:    $%.2f
Write Operations:   $%.2f
Storage:            $%.2f
GSI:                $%.2f
Streams:            $%.2f
Backup:             $%.2f
----------------------
Total Monthly:      $%.2f
Total Yearly:       $%.2f

Cost Metrics:
-------------
Cost per Item:      $%.6f
Cost per Request:   $%.6f

`,
		time.Now().Format("2006-01-02 15:04:05"),
		breakdown.ReadCost,
		breakdown.WriteCost,
		breakdown.StorageCost,
		breakdown.GSICost,
		breakdown.StreamsCost,
		breakdown.BackupCost,
		breakdown.TotalMonthlyCost,
		breakdown.TotalYearlyCost,
		breakdown.CostPerItem,
		breakdown.CostPerRequest,
	)

	if recommendations != nil && len(recommendations.Recommendations) > 0 {
		report += fmt.Sprintf(`
Optimization Recommendations:
----------------------------
Potential Monthly Savings: $%.2f

`, recommendations.PotentialSavings)

		for i, rec := range recommendations.Recommendations {
			report += fmt.Sprintf(`
%d. %s
   %s
   Impact: %s | Effort: %s | Savings: $%.2f/month

`, i+1, rec.Title, rec.Description, rec.Impact, rec.Effort, rec.Savings)
		}
	}

	return report
}
