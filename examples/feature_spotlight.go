// Package examples contains focused snippets for new TableTheory capabilities.
package examples

import (
	"context"
	"fmt"
	"time"

	"github.com/theory-cloud/tabletheory/v6"
	core "github.com/theory-cloud/tabletheory/v6/pkg/core"
)

// Bookmark models a bookmark stored in a single-table design.
type Bookmark struct {
	Created  time.Time `json:"created_at"`
	PK       string    `theorydb:"pk" json:"pk"`
	SK       string    `theorydb:"sk" json:"sk"`
	UserID   string    `json:"user_id"`
	URL      string    `json:"url"`
	Category string    `json:"category"`
}

// TableName keeps this example's table distinct when examples share one local
// DynamoDB instance.
func (Bookmark) TableName() string { return "example_conditional_bookmarks" }

// BookmarkQuota tracks how many writes a user can make in a window.
type BookmarkQuota struct {
	PK        string `theorydb:"pk" json:"pk"`
	SK        string `theorydb:"sk" json:"sk"`
	Remaining int    `json:"remaining"`
}

// TableName keeps this example's table distinct when examples share one local
// DynamoDB instance.
func (BookmarkQuota) TableName() string { return "example_conditional_bookmark_quotas" }

// BookmarkAudit is written alongside bookmark mutations.
type BookmarkAudit struct {
	CreatedAt time.Time `json:"created_at"`
	PK        string    `theorydb:"pk" json:"pk"`
	SK        string    `theorydb:"sk" json:"sk"`
	Action    string    `json:"action"`
	ActorID   string    `json:"actor_id"`
}

// TableName keeps this example's table distinct when examples share one local
// DynamoDB instance.
func (BookmarkAudit) TableName() string { return "example_conditional_bookmark_audits" }

// Session demonstrates optimistic updates with conditional helpers.
type Session struct {
	LastSeen time.Time `json:"last_seen"`
	PK       string    `theorydb:"pk" json:"pk"`
	SK       string    `theorydb:"sk" json:"sk"`
	Status   string    `json:"status"`
	Version  int64     `json:"version"`
}

// TableName keeps this example's table distinct when examples share one local
// DynamoDB instance.
func (Session) TableName() string { return "example_conditional_sessions" }

// Invoice is used for batch retrieval demos.
type Invoice struct {
	PK      string `theorydb:"pk" json:"pk"`
	SK      string `theorydb:"sk" json:"sk"`
	Status  string `json:"status"`
	Total   int64  `json:"total"`
	Balance int64  `json:"balance"`
}

// TableName keeps this example's table distinct when examples share one local
// DynamoDB instance.
func (Invoice) TableName() string { return "example_conditional_invoices" }

// DemonstrateConditionalHelpers shows insert-only creates, optimistic updates, and guarded deletes.
func DemonstrateConditionalHelpers(db core.ExtendedDB) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	session := &Session{
		PK:       "USER#123",
		SK:       "SESSION#web",
		Status:   "active",
		LastSeen: time.Now(),
		Version:  1,
	}

	// Insert-only create
	if err := db.WithContext(ctx).Model(session).IfNotExists().Create(); err != nil {
		return fmt.Errorf("insert-only create: %w", err)
	}

	// Optimistic update guarded by status + version
	session.LastSeen = time.Now()
	session.Version++
	if err := db.WithContext(ctx).Model(session).
		WithCondition("Status", "=", "active").
		WithCondition("Version", "=", session.Version-1).
		Update("LastSeen", "Version"); err != nil {
		return fmt.Errorf("conditional update: %w", err)
	}

	// Guarded delete via raw expression. The raw expression uses the stored
	// attribute name, which is the lower-camel "version" here.
	if err := db.WithContext(ctx).Model(session).
		WithConditionExpression("attribute_exists(PK) AND version = :v", map[string]any{
			":v": session.Version,
		}).
		Delete(); err != nil {
		return fmt.Errorf("conditional delete: %w", err)
	}

	return nil
}

// DemonstrateTransactionBuilder composes a dual-write with quota checks.
func DemonstrateTransactionBuilder(db core.ExtendedDB) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	bookmark := &Bookmark{
		PK:      "BOOKMARK#123",
		SK:      "USER#42",
		UserID:  "user-42",
		URL:     "https://paytheory.com",
		Created: time.Now(),
	}
	quota := &BookmarkQuota{
		PK:        "QUOTA#user-42",
		SK:        "WINDOW#2024-06",
		Remaining: 3,
	}

	err := db.Transact().
		Create(bookmark, tabletheory.IfNotExists()).
		UpdateWithBuilder(quota, func(ub core.UpdateBuilder) error {
			ub.Decrement("Remaining")
			return nil
		}, tabletheory.Condition("Remaining", ">", 0)).
		Put(&BookmarkAudit{
			PK:        bookmark.PK,
			SK:        fmt.Sprintf("AUDIT#%d", time.Now().Unix()),
			Action:    "create",
			ActorID:   bookmark.UserID,
			CreatedAt: time.Now(),
		}).
		Execute()
	if err != nil {
		return fmt.Errorf("transaction builder: %w", err)
	}

	// Context-aware helper automatically wires Execute.
	err = db.TransactWrite(ctx, func(tx core.TransactionBuilder) error {
		tx.Delete(bookmark, tabletheory.IfExists())
		tx.ConditionCheck(quota, tabletheory.Condition("Remaining", ">=", 0))
		return nil
	})
	if err != nil {
		return fmt.Errorf("transaction cleanup: %w", err)
	}

	return nil
}

// DemonstrateBatchGetBuilder fetches invoices with chunking, retries, and callbacks.
func DemonstrateBatchGetBuilder(db core.ExtendedDB) ([]Invoice, error) {
	keys := []any{
		tabletheory.NewKeyPair("ORG#42", "INVOICE#2024-01"),
		tabletheory.NewKeyPair("ORG#42", "INVOICE#2024-02"),
		tabletheory.NewKeyPair("ORG#42", "INVOICE#2024-03"),
		tabletheory.NewKeyPair("ORG#42", "INVOICE#2024-04"),
	}

	policy := &core.RetryPolicy{
		MaxRetries:    5,
		InitialDelay:  75 * time.Millisecond,
		MaxDelay:      2 * time.Second,
		BackoffFactor: 1.8,
		Jitter:        0.35,
	}

	var invoices []Invoice
	err := db.Model(&Invoice{}).
		BatchGetBuilder().
		Keys(keys).
		Select("PK", "SK", "Status", "Balance").
		Parallel(4).
		WithRetry(policy).
		OnProgress(func(done, total int) {
			fmt.Printf("batch get progress: %d/%d\n", done, total)
		}).
		OnError(func(chunk []any, chunkErr error) error {
			return fmt.Errorf("chunk of %d keys failed: %w", len(chunk), chunkErr)
		}).
		Execute(&invoices)
	if err != nil {
		return nil, fmt.Errorf("batch get builder: %w", err)
	}

	return invoices, nil
}
