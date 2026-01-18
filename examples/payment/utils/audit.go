package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/theory-cloud/tabletheory/pkg/core"
)

// AuditLog represents an audit log entry
type AuditLog struct {
	Timestamp  time.Time      `theorydb:"created_at" json:"timestamp"`
	Before     map[string]any `theorydb:"json" json:"before,omitempty"`
	After      map[string]any `theorydb:"json" json:"after,omitempty"`
	Metadata   map[string]any `theorydb:"json" json:"metadata,omitempty"`
	ID         string         `theorydb:"pk" json:"id"`
	EntityType string         `theorydb:"index:gsi-entity,pk" json:"entity_type"`
	EntityID   string         `theorydb:"index:gsi-entity,sk" json:"entity_id"`
	Action     string         `json:"action"`
	UserID     string         `json:"user_id,omitempty"`
	MerchantID string         `theorydb:"index:gsi-merchant" json:"merchant_id"`
	IPAddress  string         `json:"ip_address,omitempty"`
	UserAgent  string         `json:"user_agent,omitempty"`
}

// AuditTracker provides audit trail functionality
type AuditTracker struct {
	db core.ExtendedDB
}

// NewAuditTracker creates a new audit tracker
func NewAuditTracker(db core.ExtendedDB) *AuditTracker {
	return &AuditTracker{db: db}
}

// Track records an audit event
func (a *AuditTracker) Track(action string, entityType string, metadata map[string]any) error {
	log := &AuditLog{
		ID:         uuid.New().String(),
		EntityType: entityType,
		Action:     action,
		Metadata:   metadata,
		Timestamp:  time.Now(),
	}

	// Extract specific fields from metadata if present
	if entityID, ok := metadata["entity_id"].(string); ok {
		log.EntityID = entityID
	}
	if merchantID, ok := metadata["merchant_id"].(string); ok {
		log.MerchantID = merchantID
	}
	if userID, ok := metadata["user_id"].(string); ok {
		log.UserID = userID
	}
	if ipAddress, ok := metadata["ip_address"].(string); ok {
		log.IPAddress = ipAddress
	}

	return a.db.Model(log).Create()
}

// TrackChange records changes to an entity
func (a *AuditTracker) TrackChange(ctx context.Context, req TrackChangeRequest) error {
	log := &AuditLog{
		ID:         uuid.New().String(),
		EntityType: req.EntityType,
		EntityID:   req.EntityID,
		Action:     req.Action,
		UserID:     req.UserID,
		MerchantID: req.MerchantID,
		IPAddress:  req.IPAddress,
		UserAgent:  req.UserAgent,
		Before:     req.Before,
		After:      req.After,
		Metadata:   req.Metadata,
		Timestamp:  time.Now(),
	}

	return a.db.Model(log).Create()
}

// TrackChangeRequest contains details for tracking a change
type TrackChangeRequest struct {
	Before     map[string]any `json:"before,omitempty"`
	After      map[string]any `json:"after,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	EntityType string         `json:"entity_type"`
	EntityID   string         `json:"entity_id"`
	Action     string         `json:"action"`
	UserID     string         `json:"user_id,omitempty"`
	MerchantID string         `json:"merchant_id"`
	IPAddress  string         `json:"ip_address,omitempty"`
	UserAgent  string         `json:"user_agent,omitempty"`
}

// GetAuditHistory retrieves audit history for an entity
func (a *AuditTracker) GetAuditHistory(ctx context.Context, entityType, entityID string, limit int) ([]*AuditLog, error) {
	var logs []*AuditLog

	query := a.db.Model(&AuditLog{}).
		Index("gsi-entity").
		Where("EntityType", "=", entityType).
		Where("EntityID", "=", entityID).
		OrderBy("Timestamp", "DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.All(&logs)
	return logs, err
}

// GetMerchantAuditLogs retrieves audit logs for a merchant
func (a *AuditTracker) GetMerchantAuditLogs(ctx context.Context, merchantID string, startTime, endTime time.Time) ([]*AuditLog, error) {
	var logs []*AuditLog

	err := a.db.Model(&AuditLog{}).
		Index("gsi-merchant").
		Where("MerchantID", "=", merchantID).
		Where("Timestamp", ">=", startTime).
		Where("Timestamp", "<=", endTime).
		OrderBy("Timestamp", "DESC").
		All(&logs)

	return logs, err
}

// ComplianceReport generates a compliance report
type ComplianceReport struct {
	StartDate    time.Time           `json:"start_date"`
	EndDate      time.Time           `json:"end_date"`
	Generated    time.Time           `json:"generated"`
	EventsByType map[string]int      `json:"events_by_type"`
	UserActivity map[string]int      `json:"user_activity"`
	MerchantID   string              `json:"merchant_id"`
	Anomalies    []ComplianceAnomaly `json:"anomalies,omitempty"`
	TotalEvents  int                 `json:"total_events"`
}

// ComplianceAnomaly represents a potential compliance issue
type ComplianceAnomaly struct {
	Type        string    `json:"type"`
	Description string    `json:"description"`
	Severity    string    `json:"severity"`
	Timestamp   time.Time `json:"timestamp"`
	EntityID    string    `json:"entity_id,omitempty"`
}

// GenerateComplianceReport creates a compliance report
func (a *AuditTracker) GenerateComplianceReport(ctx context.Context, merchantID string, startDate, endDate time.Time) (*ComplianceReport, error) {
	logs, err := a.GetMerchantAuditLogs(ctx, merchantID, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve audit logs: %w", err)
	}

	report := &ComplianceReport{
		MerchantID:   merchantID,
		StartDate:    startDate,
		EndDate:      endDate,
		TotalEvents:  len(logs),
		EventsByType: make(map[string]int),
		UserActivity: make(map[string]int),
		Anomalies:    []ComplianceAnomaly{},
		Generated:    time.Now(),
	}

	// Analyze logs
	for _, log := range logs {
		// Count events by type
		report.EventsByType[log.Action]++

		// Count user activity
		if log.UserID != "" {
			report.UserActivity[log.UserID]++
		}

		// Check for anomalies
		if anomaly := a.checkForAnomaly(log); anomaly != nil {
			report.Anomalies = append(report.Anomalies, *anomaly)
		}
	}

	return report, nil
}

// checkForAnomaly checks if an audit log indicates a potential issue
func (a *AuditTracker) checkForAnomaly(log *AuditLog) *ComplianceAnomaly {
	// Example anomaly detection rules

	// Check for failed authentication attempts
	if log.Action == "authentication_failed" {
		count := 0
		if metadata, ok := log.Metadata["attempt_count"].(float64); ok {
			count = int(metadata)
		}
		if count > 5 {
			return &ComplianceAnomaly{
				Type:        "excessive_failed_auth",
				Description: fmt.Sprintf("Excessive failed authentication attempts: %d", count),
				Severity:    "high",
				Timestamp:   log.Timestamp,
				EntityID:    log.EntityID,
			}
		}
	}

	// Check for unusual transaction amounts
	if log.Action == "payment_processed" {
		if amount, ok := log.Metadata["amount"].(float64); ok && amount > 100000 {
			return &ComplianceAnomaly{
				Type:        "high_value_transaction",
				Description: fmt.Sprintf("High value transaction: $%.2f", amount/100),
				Severity:    "medium",
				Timestamp:   log.Timestamp,
				EntityID:    log.EntityID,
			}
		}
	}

	// Check for data export events
	if log.Action == "data_exported" {
		return &ComplianceAnomaly{
			Type:        "data_export",
			Description: "Sensitive data was exported",
			Severity:    "low",
			Timestamp:   log.Timestamp,
			EntityID:    log.EntityID,
		}
	}

	return nil
}

// ExportAuditLogs exports audit logs in a specific format
func (a *AuditTracker) ExportAuditLogs(ctx context.Context, merchantID string, format string) ([]byte, error) {
	logs, err := a.GetMerchantAuditLogs(ctx, merchantID, time.Now().AddDate(0, -1, 0), time.Now())
	if err != nil {
		return nil, err
	}

	switch format {
	case "json":
		return json.MarshalIndent(logs, "", "  ")
	case "csv":
		// Simplified CSV export
		csv := "timestamp,action,entity_type,entity_id,user_id\n"
		for _, log := range logs {
			csv += fmt.Sprintf("%s,%s,%s,%s,%s\n",
				log.Timestamp.Format(time.RFC3339),
				log.Action,
				log.EntityType,
				log.EntityID,
				log.UserID,
			)
		}
		return []byte(csv), nil
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}
