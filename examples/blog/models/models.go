package models

import (
	"fmt"
	"strings"
	"time"
)

// Post represents a blog post
type Post struct {
	PublishedAt   time.Time         `theorydb:"index:gsi-status-date,sk" json:"published_at,omitempty"`
	UpdatedAt     time.Time         `theorydb:"updated_at" json:"updated_at"`
	CreatedAt     time.Time         `theorydb:"created_at" json:"created_at"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	SEO           SEOMetadata       `json:"seo,omitempty"`
	Content       string            `json:"content"`
	Status        string            `theorydb:"index:gsi-status-date,pk" json:"status"`
	Excerpt       string            `json:"excerpt,omitempty"`
	CategoryID    string            `theorydb:"index:gsi-category,pk" json:"category_id,omitempty"`
	ID            string            `theorydb:"pk" json:"id"`
	FeaturedImage string            `json:"featured_image,omitempty"`
	Title         string            `json:"title"`
	AuthorID      string            `theorydb:"index:gsi-author,pk" json:"author_id"`
	Slug          string            `theorydb:"index:gsi-slug,pk" json:"slug"`
	Tags          []string          `theorydb:"set" json:"tags,omitempty"`
	ViewCount     int               `json:"view_count"`
	CommentCount  int               `json:"comment_count"`
	Version       int               `theorydb:"version" json:"version"`
}

// PostStatus constants
const (
	PostStatusDraft     = "draft"
	PostStatusPublished = "published"
	PostStatusArchived  = "archived"
)

// Comment represents a comment on a post
type Comment struct {
	CreatedAt   time.Time `theorydb:"created_at" json:"created_at"`
	UpdatedAt   time.Time `theorydb:"updated_at" json:"updated_at"`
	ID          string    `theorydb:"pk" json:"id"`
	PostID      string    `theorydb:"index:gsi-post,pk" json:"post_id"`
	ParentID    string    `theorydb:"index:gsi-post,sk,prefix:parent" json:"parent_id,omitempty"`
	AuthorID    string    `json:"author_id"`
	AuthorName  string    `json:"author_name"`
	AuthorEmail string    `json:"author_email"`
	Content     string    `json:"content"`
	Status      string    `json:"status"`
	IPAddress   string    `json:"ip_address,omitempty"`
	UserAgent   string    `json:"user_agent,omitempty"`
}

// CommentStatus constants
const (
	CommentStatusApproved = "approved"
	CommentStatusPending  = "pending"
	CommentStatusSpam     = "spam"
)

// Author represents a blog author
type Author struct {
	LastLoginAt time.Time `json:"last_login_at,omitempty"`
	CreatedAt   time.Time `theorydb:"created_at" json:"created_at"`
	UpdatedAt   time.Time `theorydb:"updated_at" json:"updated_at"`
	ID          string    `theorydb:"pk" json:"id"`
	Email       string    `theorydb:"index:gsi-email,pk" json:"email"`
	Username    string    `theorydb:"index:gsi-username,pk" json:"username"`
	Name        string    `json:"name"`
	Bio         string    `json:"bio,omitempty"`
	Avatar      string    `json:"avatar,omitempty"`
	Role        string    `json:"role"`
	PostCount   int       `json:"post_count"`
	Active      bool      `json:"active"`
}

// AuthorRole constants
const (
	RoleAdmin  = "admin"
	RoleEditor = "editor"
	RoleAuthor = "author"
)

// Category represents a blog category
type Category struct {
	CreatedAt   time.Time `theorydb:"created_at" json:"created_at"`
	UpdatedAt   time.Time `theorydb:"updated_at" json:"updated_at"`
	ID          string    `theorydb:"pk" json:"id"`
	Slug        string    `theorydb:"index:gsi-slug,pk" json:"slug"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	ParentID    string    `json:"parent_id,omitempty"`
	PostCount   int       `json:"post_count"`
}

// Tag represents a blog tag
type Tag struct {
	CreatedAt time.Time `theorydb:"created_at" json:"created_at"`
	ID        string    `theorydb:"pk" json:"id"`
	Name      string    `theorydb:"index:gsi-name,pk" json:"name"`
	Slug      string    `theorydb:"index:gsi-slug,pk" json:"slug"`
	PostCount int       `json:"post_count"`
}

// Subscriber represents an email subscriber
type Subscriber struct {
	VerifiedAt       time.Time `json:"verified_at,omitempty"`
	CreatedAt        time.Time `theorydb:"created_at" json:"created_at"`
	UpdatedAt        time.Time `theorydb:"updated_at" json:"updated_at"`
	ID               string    `theorydb:"pk" json:"id"`
	Email            string    `theorydb:"index:gsi-email,pk" json:"email"`
	Name             string    `json:"name,omitempty"`
	Status           string    `json:"status"`
	UnsubscribeToken string    `json:"-"`
	Categories       []string  `theorydb:"set" json:"categories,omitempty"`
	Tags             []string  `theorydb:"set" json:"tags,omitempty"`
}

// SearchIndex represents a search index entry for full-text search
type SearchIndex struct {
	UpdatedAt   time.Time `json:"updated_at"`
	ID          string    `theorydb:"pk" json:"id"`
	ContentType string    `theorydb:"index:gsi-search,pk" json:"content_type"`
	SearchTerms string    `theorydb:"index:gsi-search,sk" json:"search_terms"`
	PostID      string    `json:"post_id"`
	Title       string    `json:"title"`
	Excerpt     string    `json:"excerpt"`
	Tags        []string  `theorydb:"set" json:"tags"`
}

// Analytics represents page view analytics
type Analytics struct {
	UpdatedAt   time.Time      `json:"updated_at"`
	Countries   map[string]int `json:"countries"`
	Referrers   map[string]int `json:"referrers"`
	ID          string         `theorydb:"pk" json:"id"`
	PostID      string         `json:"post_id"`
	Date        string         `json:"date"`
	Views       int            `json:"views"`
	UniqueViews int            `json:"unique_views"`
}

// Helper methods for Analytics composite key
func (a *Analytics) SetCompositeKey() {
	a.ID = fmt.Sprintf("%s#%s", a.PostID, a.Date)
}

func (a *Analytics) ParseCompositeKey() error {
	parts := strings.Split(a.ID, "#")
	if len(parts) != 2 {
		return fmt.Errorf("invalid composite key format: %s", a.ID)
	}
	a.PostID = parts[0]
	a.Date = parts[1]
	return nil
}

// Session represents a user session for tracking unique views
type Session struct {
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `theorydb:"ttl" json:"expires_at"`
	ID          string    `theorydb:"pk" json:"id"`
	IPAddress   string    `json:"ip_address"`
	UserAgent   string    `json:"user_agent"`
	PostsViewed []string  `theorydb:"set" json:"posts_viewed"`
}

// PostView tracks individual post views for analytics
type PostView struct {
	Timestamp time.Time `json:"timestamp"`
	TTL       time.Time `theorydb:"ttl" json:"ttl"`
	ID        string    `theorydb:"pk" json:"id"`
	PostID    string    `json:"post_id"`
	SessionID string    `json:"session_id"`
	Country   string    `json:"country,omitempty"`
	Referrer  string    `json:"referrer,omitempty"`
}

// Helper methods for PostView composite key
func (p *PostView) SetCompositeKey() {
	p.ID = fmt.Sprintf("%s#%s", p.PostID, p.Timestamp.Format(time.RFC3339Nano))
}

func (p *PostView) ParseCompositeKey() error {
	parts := strings.Split(p.ID, "#")
	if len(parts) != 2 {
		return fmt.Errorf("invalid composite key format: %s", p.ID)
	}
	p.PostID = parts[0]
	timestamp, err := time.Parse(time.RFC3339Nano, parts[1])
	if err != nil {
		return fmt.Errorf("invalid timestamp format: %w", err)
	}
	p.Timestamp = timestamp
	return nil
}

// RelatedPost represents a many-to-many relationship between posts
type RelatedPost struct {
	CreatedAt time.Time `json:"created_at"`
	ID        string    `theorydb:"pk,composite:post_id,related_id" json:"id"`
	PostID    string    `theorydb:"extract:post_id" json:"post_id"`
	RelatedID string    `theorydb:"extract:related_id" json:"related_id"`
	Score     float64   `json:"score"`
}

// ContentBlock represents reusable content blocks
type ContentBlock struct {
	CreatedAt time.Time         `theorydb:"created_at" json:"created_at"`
	UpdatedAt time.Time         `theorydb:"updated_at" json:"updated_at"`
	Variables map[string]string `theorydb:"json" json:"variables,omitempty"`
	ID        string            `theorydb:"pk" json:"id"`
	Name      string            `theorydb:"index:gsi-name,unique" json:"name"`
	Type      string            `json:"type"`
	Content   string            `json:"content"`
	Version   int               `theorydb:"version" json:"version"`
}

// Page represents a static page
type Page struct {
	PublishedAt time.Time         `json:"published_at,omitempty"`
	UpdatedAt   time.Time         `theorydb:"updated_at" json:"updated_at"`
	CreatedAt   time.Time         `theorydb:"created_at" json:"created_at"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	SEO         SEOMetadata       `json:"seo,omitempty"`
	Title       string            `json:"title"`
	Status      string            `json:"status"`
	Template    string            `json:"template,omitempty"`
	Content     string            `json:"content"`
	ID          string            `theorydb:"pk" json:"id"`
	ParentID    string            `json:"parent_id,omitempty"`
	Slug        string            `theorydb:"index:gsi-slug,pk" json:"slug"`
	Order       int               `json:"order"`
	Version     int               `theorydb:"version" json:"version"`
}

// EmailTemplate represents an email template
type EmailTemplate struct {
	CreatedAt time.Time         `theorydb:"created_at" json:"created_at"`
	UpdatedAt time.Time         `theorydb:"updated_at" json:"updated_at"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	ID        string            `theorydb:"pk" json:"id"`
	Name      string            `theorydb:"index:gsi-name,pk" json:"name"`
	Subject   string            `json:"subject"`
	Body      string            `json:"body"`
	Type      string            `json:"type"`
	Variables []string          `json:"variables"`
	Active    bool              `json:"active"`
}

// SEOMetadata represents SEO information for posts and pages
type SEOMetadata struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Keywords    string `json:"keywords,omitempty"`
	OGImage     string `json:"og_image,omitempty"`
}
