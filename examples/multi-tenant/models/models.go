package models

import (
	"time"
)

// Organization represents a tenant in the multi-tenant system
type Organization struct {
	ID             string      `theorydb:"pk" json:"id"`
	Slug           string      `theorydb:"index:gsi-slug,pk,sparse" json:"slug"`
	Name           string      `json:"name"`
	Domain         string      `theorydb:"index:gsi-domain,pk,sparse,omitempty" json:"domain,omitempty"`
	Plan           string      `json:"plan"`   // free, starter, pro, enterprise
	Status         string      `json:"status"` // active, suspended, cancelled
	OwnerID        string      `json:"owner_id"`
	Settings       OrgSettings `theorydb:"json" json:"settings"`
	Limits         PlanLimits  `theorydb:"json" json:"limits"`
	BillingInfo    BillingInfo `theorydb:"json" json:"billing_info"`
	UserCount      int         `json:"user_count"`
	ProjectCount   int         `json:"project_count"`
	StorageUsed    int64       `json:"storage_used"` // bytes
	TrialEndsAt    time.Time   `json:"trial_ends_at,omitempty"`
	SubscriptionID string      `json:"subscription_id,omitempty"`
	CreatedAt      time.Time   `theorydb:"created_at" json:"created_at"`
	UpdatedAt      time.Time   `theorydb:"updated_at" json:"updated_at"`
	Version        int         `theorydb:"version" json:"version"`
}

// OrgSettings contains organization-specific settings
type OrgSettings struct {
	AllowSignup    bool     `json:"allow_signup"`
	RequireMFA     bool     `json:"require_mfa"`
	AllowedDomains []string `json:"allowed_domains,omitempty"`
	DefaultRole    string   `json:"default_role"`
	SessionTimeout int      `json:"session_timeout"` // minutes
	IPWhitelist    []string `json:"ip_whitelist,omitempty"`
	WebhookURL     string   `json:"webhook_url,omitempty"`
	WebhookSecret  string   `json:"webhook_secret,omitempty"`
}

// PlanLimits defines resource limits for the organization's plan
type PlanLimits struct {
	MaxUsers       int    `json:"max_users"`
	MaxProjects    int    `json:"max_projects"`
	MaxStorage     int64  `json:"max_storage"`      // bytes
	MaxAPIRequests int    `json:"max_api_requests"` // per month
	MaxComputeTime int    `json:"max_compute_time"` // minutes per month
	CustomDomain   bool   `json:"custom_domain"`
	SSO            bool   `json:"sso"`
	AuditLog       bool   `json:"audit_log"`
	Support        string `json:"support"` // community, email, priority, dedicated
}

// BillingInfo contains billing information
type BillingInfo struct {
	CustomerID      string  `json:"customer_id"` // Stripe customer ID
	PaymentMethodID string  `json:"payment_method_id,omitempty"`
	BillingEmail    string  `json:"billing_email"`
	CompanyName     string  `json:"company_name,omitempty"`
	TaxID           string  `json:"tax_id,omitempty"`
	Address         Address `json:"address"`
}

// User represents a user within an organization
type User struct {
	ID              string    `theorydb:"pk" json:"id"`
	OrgID           string    `theorydb:"" json:"org_id"`
	UserID          string    `theorydb:"" json:"user_id"`
	Email           string    `theorydb:"index:gsi-email,pk" json:"email"`
	OrgEmail        string    `theorydb:"index:gsi-email,sk" json:"org_email"`
	Username        string    `json:"username"`
	FirstName       string    `json:"first_name"`
	LastName        string    `json:"last_name"`
	Avatar          string    `json:"avatar,omitempty"`
	Role            string    `json:"role"`   // owner, admin, member, viewer
	Status          string    `json:"status"` // active, invited, suspended
	Permissions     []string  `theorydb:"set,omitempty" json:"permissions"`
	Projects        []string  `theorydb:"set,omitempty" json:"projects"` // Project IDs user has access to
	MFAEnabled      bool      `json:"mfa_enabled"`
	MFASecret       string    `json:"-"`
	LastLoginAt     time.Time `json:"last_login_at,omitempty"`
	InvitedBy       string    `json:"invited_by,omitempty"`
	InviteToken     string    `json:"invite_token,omitempty"`
	InviteExpiresAt time.Time `json:"invite_expires_at,omitempty"`
	CreatedAt       time.Time `theorydb:"created_at" json:"created_at"`
	UpdatedAt       time.Time `theorydb:"updated_at" json:"updated_at"`
}

// Project represents a project within an organization
type Project struct {
	ID           string          `theorydb:"pk" json:"id"`
	OrgID        string          `theorydb:"" json:"org_id"`
	ProjectID    string          `theorydb:"" json:"project_id"`
	Name         string          `theorydb:"index:gsi-org-projects,pk" json:"name"`
	ProjectName  string          `theorydb:"index:gsi-org-projects,sk" json:"project_name"`
	Slug         string          `json:"slug"`
	Description  string          `json:"description,omitempty"`
	Type         string          `json:"type"`        // web, mobile, api, ml
	Status       string          `json:"status"`      // active, archived, deleted
	Environment  string          `json:"environment"` // development, staging, production
	Settings     ProjectSettings `theorydb:"json" json:"settings"`
	Team         []TeamMember    `theorydb:"json" json:"team"`
	Resources    ResourceQuota   `theorydb:"json" json:"resources"`
	Tags         []string        `theorydb:"set,omitempty" json:"tags"`
	Repository   string          `json:"repository,omitempty"`
	DeploymentID string          `json:"deployment_id,omitempty"`
	CreatedBy    string          `json:"created_by"`
	CreatedAt    time.Time       `theorydb:"created_at" json:"created_at"`
	UpdatedAt    time.Time       `theorydb:"updated_at" json:"updated_at"`
	Version      int             `theorydb:"version" json:"version"`
}

// ProjectSettings contains project-specific settings
type ProjectSettings struct {
	AutoDeploy      bool              `json:"auto_deploy"`
	BuildCommand    string            `json:"build_command,omitempty"`
	OutputDirectory string            `json:"output_directory,omitempty"`
	EnvVars         map[string]string `json:"env_vars,omitempty"`
	Domains         []string          `json:"domains,omitempty"`
	Framework       string            `json:"framework,omitempty"`
	NodeVersion     string            `json:"node_version,omitempty"`
}

// TeamMember represents a user's role in a project
type TeamMember struct {
	UserID  string    `json:"user_id"`
	Role    string    `json:"role"` // lead, developer, viewer
	AddedAt time.Time `json:"added_at"`
	AddedBy string    `json:"added_by"`
}

// ResourceQuota tracks resource usage for a project
type ResourceQuota struct {
	CPULimit      int   `json:"cpu_limit"`     // millicores
	MemoryLimit   int   `json:"memory_limit"`  // MB
	StorageLimit  int64 `json:"storage_limit"` // bytes
	CPUUsed       int   `json:"cpu_used"`
	MemoryUsed    int   `json:"memory_used"`
	StorageUsed   int64 `json:"storage_used"`
	BuildMinutes  int   `json:"build_minutes"`
	BandwidthUsed int64 `json:"bandwidth_used"` // bytes
}

// Resource represents a billable resource (API calls, storage, compute)
type Resource struct {
	ID           string            `theorydb:"pk" json:"id"`
	OrgID        string            `theorydb:"" json:"org_id"`
	ResourceID   string            `theorydb:"" json:"resource_id"`
	ProjectID    string            `theorydb:"index:gsi-project-resources,pk" json:"project_id"`
	Timestamp    time.Time         `theorydb:"index:gsi-project-resources,sk" json:"timestamp"`
	Type         string            `json:"type"` // api_call, storage, compute, bandwidth
	Name         string            `json:"name"`
	Quantity     float64           `json:"quantity"`
	Unit         string            `json:"unit"` // requests, bytes, minutes, etc
	Cost         int               `json:"cost"` // in cents
	UserID       string            `json:"user_id,omitempty"`
	Metadata     map[string]string `theorydb:"json" json:"metadata,omitempty"`
	BillingCycle string            `json:"billing_cycle"` // YYYY-MM
	CreatedAt    time.Time         `theorydb:"created_at" json:"created_at"`
}

// APIKey represents an API key for programmatic access
type APIKey struct {
	ID         string    `theorydb:"pk" json:"id"`
	OrgID      string    `theorydb:"" json:"org_id"`
	KeyID      string    `theorydb:"" json:"key_id"`
	Name       string    `json:"name"`
	KeyHash    string    `json:"-"`          // Hashed API key
	KeyPrefix  string    `json:"key_prefix"` // First 8 chars for identification
	ProjectID  string    `json:"project_id,omitempty"`
	Scopes     []string  `theorydb:"set,omitempty" json:"scopes"`
	RateLimit  int       `json:"rate_limit"` // requests per hour
	LastUsedAt time.Time `json:"last_used_at,omitempty"`
	LastUsedIP string    `json:"last_used_ip,omitempty"`
	ExpiresAt  time.Time `json:"expires_at,omitempty"`
	CreatedBy  string    `json:"created_by"`
	CreatedAt  time.Time `theorydb:"created_at" json:"created_at"`
	Active     bool      `json:"active"`
}

// AuditLog represents an audit trail entry
type AuditLog struct {
	ID           string            `theorydb:"pk" json:"id"`
	OrgID        string            `theorydb:"" json:"org_id"`
	Timestamp    time.Time         `theorydb:"" json:"timestamp"`
	EventID      string            `theorydb:"" json:"event_id"`
	UserID       string            `theorydb:"index:gsi-user-audit,pk" json:"user_id"`
	UserTime     time.Time         `theorydb:"index:gsi-user-audit,sk" json:"user_time"`
	Action       string            `json:"action"`        // create, update, delete, login, etc
	ResourceType string            `json:"resource_type"` // user, project, apikey, etc
	ResourceID   string            `json:"resource_id"`
	Changes      map[string]Change `theorydb:"json" json:"changes,omitempty"`
	IPAddress    string            `json:"ip_address"`
	UserAgent    string            `json:"user_agent,omitempty"`
	Success      bool              `json:"success"`
	ErrorMessage string            `json:"error_message,omitempty"`
	TTL          int64             `theorydb:"ttl" json:"ttl"` // 90 days retention
}

// Change represents a field change in audit log
type Change struct {
	From any `json:"from"`
	To   any `json:"to"`
}

// Invitation represents a pending user invitation
type Invitation struct {
	ID        string    `theorydb:"pk" json:"id"`
	OrgID     string    `theorydb:"" json:"org_id"`
	InviteID  string    `theorydb:"" json:"invite_id"`
	Email     string    `theorydb:"index:gsi-invite-email" json:"email"`
	Role      string    `json:"role"`
	Projects  []string  `theorydb:"set,omitempty" json:"projects,omitempty"`
	InvitedBy string    `json:"invited_by"`
	Token     string    `json:"-"`
	ExpiresAt int64     `theorydb:"ttl" json:"expires_at"`
	CreatedAt time.Time `theorydb:"created_at" json:"created_at"`
}

// UsageReport represents monthly usage for billing
type UsageReport struct {
	ID             string           `theorydb:"pk" json:"id"`
	OrgID          string           `theorydb:"" json:"org_id"`
	BillingCycle   string           `theorydb:"" json:"billing_cycle"` // YYYY-MM
	Plan           string           `json:"plan"`
	UserCount      int              `json:"user_count"`
	ProjectCount   int              `json:"project_count"`
	APIRequests    int              `json:"api_requests"`
	ComputeMinutes int              `json:"compute_minutes"`
	StorageGB      float64          `json:"storage_gb"`
	BandwidthGB    float64          `json:"bandwidth_gb"`
	TotalCost      int              `json:"total_cost"` // in cents
	Breakdown      []UsageBreakdown `theorydb:"json" json:"breakdown"`
	InvoiceID      string           `json:"invoice_id,omitempty"`
	PaymentStatus  string           `json:"payment_status"` // pending, paid, failed
	GeneratedAt    time.Time        `json:"generated_at"`
}

// UsageBreakdown represents cost breakdown by resource type
type UsageBreakdown struct {
	Type     string  `json:"type"`
	Quantity float64 `json:"quantity"`
	Unit     string  `json:"unit"`
	Rate     int     `json:"rate"` // cents per unit
	Cost     int     `json:"cost"` // total in cents
}

// Address represents a billing address
type Address struct {
	Line1      string `json:"line1"`
	Line2      string `json:"line2,omitempty"`
	City       string `json:"city"`
	State      string `json:"state"`
	PostalCode string `json:"postal_code"`
	Country    string `json:"country"`
}

// Constants for roles and permissions
const (
	// Organization roles
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
	RoleViewer = "viewer"

	// Project roles
	ProjectRoleLead      = "lead"
	ProjectRoleDeveloper = "developer"
	ProjectRoleViewer    = "viewer"

	// Plans
	PlanFree       = "free"
	PlanStarter    = "starter"
	PlanPro        = "pro"
	PlanEnterprise = "enterprise"

	// Resource types
	ResourceTypeAPI       = "api_call"
	ResourceTypeStorage   = "storage"
	ResourceTypeCompute   = "compute"
	ResourceTypeBandwidth = "bandwidth"
)
