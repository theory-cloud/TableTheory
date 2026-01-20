package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"github.com/theory-cloud/tabletheory/examples/multi-tenant/models"
	"github.com/theory-cloud/tabletheory/pkg/core"
	derrors "github.com/theory-cloud/tabletheory/pkg/errors"
)

// ResourceHandler handles resource tracking and usage reporting
type ResourceHandler struct {
	db core.ExtendedDB
}

// NewResourceHandler creates a new resource handler
func NewResourceHandler(db core.ExtendedDB) *ResourceHandler {
	return &ResourceHandler{db: db}
}

// RecordUsage records resource usage (typically called by API key authenticated requests)
func (h *ResourceHandler) RecordUsage(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	orgID := vars["org_id"]

	// API key validation should be done in middleware
	apiKeyID := r.Context().Value("api_key_id").(string)

	// Ensure proper org ID format
	if !strings.HasPrefix(orgID, "org#") {
		orgID = fmt.Sprintf("org#%s", orgID)
	}

	var req struct {
		ProjectID string            `json:"project_id"`
		Type      string            `json:"type"`
		Quantity  float64           `json:"quantity"`
		Metadata  map[string]string `json:"metadata"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.ProjectID == "" || req.Type == "" || req.Quantity <= 0 {
		http.Error(w, "project_id, type, and positive quantity are required", http.StatusBadRequest)
		return
	}

	// Ensure proper project ID format
	if !strings.HasPrefix(req.ProjectID, "project#") {
		req.ProjectID = fmt.Sprintf("project#%s", req.ProjectID)
	}

	// Verify project belongs to organization
	var project models.Project
	if err := h.db.Model(&models.Project{}).
		Where("ID", "=", fmt.Sprintf("%s#%s", orgID, req.ProjectID)).
		First(&project); err != nil {
		if err == derrors.ErrItemNotFound {
			http.Error(w, "project not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Check resource limits
	if err := h.checkResourceLimits(orgID, req.ProjectID, req.Type, req.Quantity); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	// Calculate cost based on type and organization plan
	cost := h.calculateCost(orgID, req.Type, req.Quantity)

	// Create resource record
	resource := &models.Resource{
		ID:           fmt.Sprintf("%s#resource#%s", orgID, uuid.New().String()),
		OrgID:        orgID,
		ResourceID:   fmt.Sprintf("resource#%s", uuid.New().String()),
		ProjectID:    req.ProjectID,
		Timestamp:    time.Now(),
		Type:         req.Type,
		Name:         fmt.Sprintf("%s usage", req.Type),
		Quantity:     req.Quantity,
		Unit:         getResourceUnit(req.Type),
		Cost:         cost,
		Metadata:     req.Metadata,
		BillingCycle: time.Now().Format("2006-01"),
	}

	// Add API key to metadata
	if resource.Metadata == nil {
		resource.Metadata = make(map[string]string)
	}
	resource.Metadata["api_key_id"] = apiKeyID

	// Save resource record
	if err := h.db.Model(resource).Create(); err != nil {
		http.Error(w, "failed to record usage", http.StatusInternalServerError)
		return
	}

	// Update project resource usage
	h.updateProjectResourceUsage(&project, req.Type, req.Quantity)

	// Log audit event
	h.logAuditEvent(orgID, apiKeyID, "record_usage", "resource", resource.ResourceID, nil, true, "")

	response := map[string]any{
		"resource_id": resource.ResourceID,
		"quantity":    resource.Quantity,
		"unit":        resource.Unit,
		"cost":        resource.Cost,
		"timestamp":   resource.Timestamp,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetUsageReport returns usage report for an organization
func (h *ResourceHandler) GetUsageReport(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	orgID := vars["org_id"]

	// Ensure proper org ID format
	if !strings.HasPrefix(orgID, "org#") {
		orgID = fmt.Sprintf("org#%s", orgID)
	}

	// Get billing cycle from query params
	billingCycle := r.URL.Query().Get("billing_cycle")
	if billingCycle == "" {
		billingCycle = time.Now().Format("2006-01")
	}

	// Check if cached report exists
	var report models.UsageReport
	err := h.db.Model(&models.UsageReport{}).
		Where("ID", "=", fmt.Sprintf("%s#%s", orgID, billingCycle)).
		First(&report)

	if err == derrors.ErrItemNotFound {
		// Generate new report
		report = h.generateUsageReport(orgID, billingCycle)

		// Save report for caching
		if err := h.db.Model(&report).Create(); err != nil {
			// Log error but continue - report generation is more important
			fmt.Printf("Failed to cache usage report: %v\n", err)
		}
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Add real-time data for current month
	if billingCycle == time.Now().Format("2006-01") {
		h.updateReportWithRealTimeData(&report)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(report)
}

// GetProjectUsage returns usage for a specific project
func (h *ResourceHandler) GetProjectUsage(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	orgID := vars["org_id"]
	projectID := vars["project_id"]

	// Ensure proper ID formats
	if !strings.HasPrefix(orgID, "org#") {
		orgID = fmt.Sprintf("org#%s", orgID)
	}
	if !strings.HasPrefix(projectID, "project#") {
		projectID = fmt.Sprintf("project#%s", projectID)
	}

	// Get date range from query params
	startDate := r.URL.Query().Get("start_date")
	endDate := r.URL.Query().Get("end_date")

	if startDate == "" {
		// Default to current month
		startDate = time.Now().Format("2006-01-02")
		startDate = startDate[:8] + "01"
	}
	if endDate == "" {
		endDate = time.Now().Format("2006-01-02")
	}

	start, _ := time.Parse("2006-01-02", startDate)
	end, _ := time.Parse("2006-01-02", endDate)

	// Query resources
	var resources []models.Resource
	if err := h.db.Model(&models.Resource{}).
		Where("OrgID", "=", orgID).
		Where("ProjectID", "=", projectID).
		Where("Timestamp", ">=", start).
		Where("Timestamp", "<=", end).
		All(&resources); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Aggregate by type
	usage := make(map[string]struct {
		Quantity float64 `json:"quantity"`
		Unit     string  `json:"unit"`
		Cost     int     `json:"cost"`
		Count    int     `json:"count"`
	})

	for _, resource := range resources {
		u := usage[resource.Type]
		u.Quantity += resource.Quantity
		u.Unit = resource.Unit
		u.Cost += resource.Cost
		u.Count++
		usage[resource.Type] = u
	}

	response := map[string]any{
		"project_id": projectID,
		"start_date": startDate,
		"end_date":   endDate,
		"usage":      usage,
		"total_cost": calculateTotalCost(resources),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Helper functions

func (h *ResourceHandler) checkResourceLimits(orgID, projectID, resourceType string, quantity float64) error {
	// Get organization to check plan limits
	var org models.Organization
	if err := h.db.Model(&models.Organization{}).
		Where("ID", "=", orgID).
		First(&org); err != nil {
		return fmt.Errorf("failed to get organization")
	}

	// Get current month usage
	billingCycle := time.Now().Format("2006-01")
	var currentUsage float64

	var resources []models.Resource
	h.db.Model(&models.Resource{}).
		Where("OrgID", "=", orgID).
		Where("BillingCycle", "=", billingCycle).
		Where("Type", "=", resourceType).
		All(&resources)

	for _, r := range resources {
		currentUsage += r.Quantity
	}

	// Check limits based on resource type
	switch resourceType {
	case models.ResourceTypeAPI:
		if org.Limits.MaxAPIRequests > 0 && int(currentUsage+quantity) > org.Limits.MaxAPIRequests {
			return fmt.Errorf("API request limit exceeded (%d/%d)", int(currentUsage+quantity), org.Limits.MaxAPIRequests)
		}
	case models.ResourceTypeCompute:
		if org.Limits.MaxComputeTime > 0 && int(currentUsage+quantity) > org.Limits.MaxComputeTime {
			return fmt.Errorf("compute time limit exceeded (%d/%d minutes)", int(currentUsage+quantity), org.Limits.MaxComputeTime)
		}
	case models.ResourceTypeStorage:
		// Check project-level storage limit
		var project models.Project
		h.db.Model(&models.Project{}).
			Where("ID", "=", fmt.Sprintf("%s#%s", orgID, projectID)).
			First(&project)

		if project.Resources.StorageLimit > 0 && project.Resources.StorageUsed+int64(quantity) > project.Resources.StorageLimit {
			return fmt.Errorf("project storage limit exceeded")
		}
	}

	return nil
}

func (h *ResourceHandler) calculateCost(orgID, resourceType string, quantity float64) int {
	// Get organization plan
	var org models.Organization
	h.db.Model(&models.Organization{}).
		Where("ID", "=", orgID).
		First(&org)

	// Free plan has no costs
	if org.Plan == models.PlanFree {
		return 0
	}

	// Calculate based on resource type and plan
	var rate int // cents per unit

	switch resourceType {
	case models.ResourceTypeAPI:
		// $0.50 per 1000 requests
		rate = 50
		quantity = quantity / 1000
	case models.ResourceTypeCompute:
		// $0.10 per minute
		rate = 10
	case models.ResourceTypeStorage:
		// $0.10 per GB per month
		rate = 10
		quantity = quantity / (1024 * 1024 * 1024) // Convert to GB
	case models.ResourceTypeBandwidth:
		// $0.08 per GB
		rate = 8
		quantity = quantity / (1024 * 1024 * 1024) // Convert to GB
	}

	// Apply plan discounts
	switch org.Plan {
	case models.PlanStarter:
		rate = rate * 9 / 10 // 10% discount
	case models.PlanPro:
		rate = rate * 8 / 10 // 20% discount
	case models.PlanEnterprise:
		rate = rate * 6 / 10 // 40% discount
	}

	return int(quantity * float64(rate))
}

func (h *ResourceHandler) generateUsageReport(orgID, billingCycle string) models.UsageReport {
	// Get all resources for the billing cycle
	var resources []models.Resource
	h.db.Model(&models.Resource{}).
		Where("OrgID", "=", orgID).
		Where("BillingCycle", "=", billingCycle).
		All(&resources)

	// Get organization details
	var org models.Organization
	h.db.Model(&models.Organization{}).
		Where("ID", "=", orgID).
		First(&org)

	// Aggregate usage
	breakdown := make(map[string]models.UsageBreakdown)

	for _, resource := range resources {
		b, exists := breakdown[resource.Type]
		if !exists {
			b = models.UsageBreakdown{
				Type: resource.Type,
				Unit: resource.Unit,
			}
		}
		b.Quantity += resource.Quantity
		b.Cost += resource.Cost
		breakdown[resource.Type] = b
	}

	// Convert map to slice and calculate rates
	breakdownSlice := []models.UsageBreakdown{}
	totalCost := 0

	for _, b := range breakdown {
		if b.Quantity > 0 {
			b.Rate = int(float64(b.Cost) / b.Quantity)
		}
		totalCost += b.Cost
		breakdownSlice = append(breakdownSlice, b)
	}

	// Count unique projects and calculate storage
	projectMap := make(map[string]bool)
	var storageGB float64
	var bandwidthGB float64
	var apiRequests int
	var computeMinutes int

	for _, resource := range resources {
		projectMap[resource.ProjectID] = true

		switch resource.Type {
		case models.ResourceTypeAPI:
			apiRequests += int(resource.Quantity)
		case models.ResourceTypeCompute:
			computeMinutes += int(resource.Quantity)
		case models.ResourceTypeStorage:
			storageGB = resource.Quantity / (1024 * 1024 * 1024) // Latest storage value
		case models.ResourceTypeBandwidth:
			bandwidthGB += resource.Quantity / (1024 * 1024 * 1024)
		}
	}

	return models.UsageReport{
		ID:             fmt.Sprintf("%s#%s", orgID, billingCycle),
		OrgID:          orgID,
		BillingCycle:   billingCycle,
		Plan:           org.Plan,
		UserCount:      org.UserCount,
		ProjectCount:   len(projectMap),
		APIRequests:    apiRequests,
		ComputeMinutes: computeMinutes,
		StorageGB:      storageGB,
		BandwidthGB:    bandwidthGB,
		TotalCost:      totalCost,
		Breakdown:      breakdownSlice,
		PaymentStatus:  "pending",
		GeneratedAt:    time.Now(),
	}
}

func (h *ResourceHandler) updateReportWithRealTimeData(report *models.UsageReport) {
	// Get resources since report generation
	var resources []models.Resource
	h.db.Model(&models.Resource{}).
		Where("OrgID", "=", report.OrgID).
		Where("BillingCycle", "=", report.BillingCycle).
		Where("Timestamp", ">", report.GeneratedAt).
		All(&resources)

	// Update totals
	for _, resource := range resources {
		switch resource.Type {
		case models.ResourceTypeAPI:
			report.APIRequests += int(resource.Quantity)
		case models.ResourceTypeCompute:
			report.ComputeMinutes += int(resource.Quantity)
		case models.ResourceTypeStorage:
			report.StorageGB = resource.Quantity / (1024 * 1024 * 1024)
		case models.ResourceTypeBandwidth:
			report.BandwidthGB += resource.Quantity / (1024 * 1024 * 1024)
		}

		report.TotalCost += resource.Cost

		// Update breakdown
		found := false
		for i, b := range report.Breakdown {
			if b.Type == resource.Type {
				report.Breakdown[i].Quantity += resource.Quantity
				report.Breakdown[i].Cost += resource.Cost
				found = true
				break
			}
		}
		if !found {
			report.Breakdown = append(report.Breakdown, models.UsageBreakdown{
				Type:     resource.Type,
				Quantity: resource.Quantity,
				Unit:     resource.Unit,
				Rate:     int(float64(resource.Cost) / resource.Quantity),
				Cost:     resource.Cost,
			})
		}
	}
}

func (h *ResourceHandler) updateProjectResourceUsage(project *models.Project, resourceType string, quantity float64) {
	switch resourceType {
	case models.ResourceTypeCompute:
		project.Resources.CPUUsed += int(quantity)
	case models.ResourceTypeStorage:
		project.Resources.StorageUsed = int64(quantity)
	case models.ResourceTypeBandwidth:
		project.Resources.BandwidthUsed += int64(quantity)
	}

	h.db.Model(project).Update()
}

func getResourceUnit(resourceType string) string {
	switch resourceType {
	case models.ResourceTypeAPI:
		return "requests"
	case models.ResourceTypeCompute:
		return "minutes"
	case models.ResourceTypeStorage:
		return "bytes"
	case models.ResourceTypeBandwidth:
		return "bytes"
	default:
		return "units"
	}
}

func calculateTotalCost(resources []models.Resource) int {
	total := 0
	for _, r := range resources {
		total += r.Cost
	}
	return total
}

func (h *ResourceHandler) logAuditEvent(orgID, userID, action, resourceType, resourceID string, changes map[string]models.Change, success bool, errorMsg string) {
	audit := &models.AuditLog{
		ID:           fmt.Sprintf("%s#%s#%s", orgID, time.Now().Format(time.RFC3339Nano), uuid.New().String()),
		OrgID:        orgID,
		Timestamp:    time.Now(),
		EventID:      uuid.New().String(),
		UserID:       userID,
		UserTime:     time.Now(),
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Changes:      changes,
		Success:      success,
		ErrorMessage: errorMsg,
		TTL:          time.Now().AddDate(0, 3, 0).Unix(), // 90 days retention
	}

	// Best effort - don't fail the main operation if audit fails
	h.db.Model(audit).Create()
}
