// Package examples contains focused snippets for new TableTheory capabilities.
package examples

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/theory-cloud/tabletheory"
	core "github.com/theory-cloud/tabletheory/pkg/core"
	customerrors "github.com/theory-cloud/tabletheory/pkg/errors"
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

// BookmarkQuota tracks how many writes a user can make in a window.
type BookmarkQuota struct {
	PK        string `theorydb:"pk" json:"pk"`
	SK        string `theorydb:"sk" json:"sk"`
	Remaining int    `json:"remaining"`
}

// BookmarkAudit is written alongside bookmark mutations.
type BookmarkAudit struct {
	CreatedAt time.Time `json:"created_at"`
	PK        string    `theorydb:"pk" json:"pk"`
	SK        string    `theorydb:"sk" json:"sk"`
	Action    string    `json:"action"`
	ActorID   string    `json:"actor_id"`
}

// Session demonstrates optimistic updates with conditional helpers.
type Session struct {
	LastSeen time.Time `json:"last_seen"`
	PK       string    `theorydb:"pk" json:"pk"`
	SK       string    `theorydb:"sk" json:"sk"`
	Status   string    `json:"status"`
	Version  int64     `json:"version"`
}

// Invoice is used for batch retrieval demos.
type Invoice struct {
	PK      string `theorydb:"pk" json:"pk"`
	SK      string `theorydb:"sk" json:"sk"`
	Status  string `json:"status"`
	Total   int64  `json:"total"`
	Balance int64  `json:"balance"`
}

// DemonstrateConditionalHelpers shows insert-only creates, optimistic updates, and guarded deletes.
func DemonstrateConditionalHelpers() {
	db, err := theorydb.New(theorydb.Config{
		Region: "us-east-1",
	})
	if err != nil {
		log.Fatalf("init theorydb: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
		if errors.Is(err, customerrors.ErrConditionFailed) {
			log.Println("session already exists, skipping")
		} else {
			log.Printf("create session failed: %v", err)
		}
	}

	// Optimistic update guarded by status + version
	session.LastSeen = time.Now()
	session.Version++
	if err := db.Model(session).
		WithCondition("Status", "=", "active").
		WithCondition("Version", "=", session.Version-1).
		Update("LastSeen", "Version"); err != nil {
		if errors.Is(err, customerrors.ErrConditionFailed) {
			log.Println("session mutated concurrently, retrying fetch")
		} else {
			log.Printf("conditional update failed: %v", err)
		}
	}

	// Guarded delete via raw expression
	if err := db.Model(session).
		WithConditionExpression("attribute_exists(PK) AND Version = :v", map[string]any{
			":v": session.Version,
		}).
		Delete(); err != nil && !errors.Is(err, customerrors.ErrConditionFailed) {
		log.Printf("conditional delete failed: %v", err)
	}
}

// DemonstrateTransactionBuilder composes a dual-write with quota checks.
func DemonstrateTransactionBuilder() {
	db, err := theorydb.New(theorydb.Config{
		Region: "us-east-1",
	})
	if err != nil {
		log.Fatalf("init theorydb: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
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

	err = db.Transact().
		Create(bookmark, theorydb.IfNotExists()).
		UpdateWithBuilder(quota, func(ub core.UpdateBuilder) error {
			ub.Decrement("Remaining")
			return nil
		}, theorydb.Condition("Remaining", ">", 0)).
		Put(&BookmarkAudit{
			PK:        bookmark.PK,
			SK:        fmt.Sprintf("AUDIT#%d", time.Now().Unix()),
			Action:    "create",
			ActorID:   bookmark.UserID,
			CreatedAt: time.Now(),
		}).
		Execute()
	if err != nil {
		var txErr *customerrors.TransactionError
		if errors.As(err, &txErr) {
			log.Printf("transaction canceled at op %d (%s): %s", txErr.OperationIndex, txErr.Operation, txErr.Reason)
		}
		if errors.Is(err, customerrors.ErrConditionFailed) {
			log.Println("bookmark already exists or quota exhausted")
		} else {
			log.Printf("transact builder failed: %v", err)
		}
	}

	// Context-aware helper automatically wires Execute.
	err = db.TransactWrite(ctx, func(tx core.TransactionBuilder) error {
		tx.Delete(bookmark, theorydb.IfExists())
		tx.ConditionCheck(quota, theorydb.Condition("Remaining", ">=", 0))
		return nil
	})
	if err != nil {
		log.Printf("transact write cleanup failed: %v", err)
	}
}

// DemonstrateBatchGetBuilder fetches invoices with chunking, retries, and callbacks.
func DemonstrateBatchGetBuilder() {
	db, err := theorydb.New(theorydb.Config{
		Region: "us-east-1",
	})
	if err != nil {
		log.Fatalf("init theorydb: %v", err)
	}

	keys := []any{
		theorydb.NewKeyPair("ORG#42", "INVOICE#2024-01"),
		theorydb.NewKeyPair("ORG#42", "INVOICE#2024-02"),
		theorydb.NewKeyPair("ORG#42", "INVOICE#2024-03"),
		theorydb.NewKeyPair("ORG#42", "INVOICE#2024-04"),
	}

	policy := &core.RetryPolicy{
		MaxRetries:    5,
		InitialDelay:  75 * time.Millisecond,
		MaxDelay:      2 * time.Second,
		BackoffFactor: 1.8,
		Jitter:        0.35,
	}

	var invoices []Invoice
	err = db.Model(&Invoice{}).
		BatchGetBuilder().
		Keys(keys).
		Select("PK", "SK", "Status", "Balance").
		Parallel(4).
		WithRetry(policy).
		OnProgress(func(done, total int) {
			log.Printf("retrieved %d/%d invoices", done, total)
		}).
		OnError(func(chunk []any, chunkErr error) error {
			log.Printf("chunk failed (%d keys): %v", len(chunk), chunkErr)
			return chunkErr
		}).
		Execute(&invoices)
	if err != nil {
		log.Printf("batch get builder failed: %v", err)
	}
}
