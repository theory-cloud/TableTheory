// Package main drives the loose examples/*.go snippets against DynamoDB Local.
// Each snippet accepts an initialized DB handle; this driver creates the
// tables, seeds the rows the snippets mutate, and asserts the real outcomes.
package main

import (
	"fmt"
	"log"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"

	"github.com/theory-cloud/tabletheory/v6"
	"github.com/theory-cloud/tabletheory/v6/examples"
	"github.com/theory-cloud/tabletheory/v6/pkg/core"
	"github.com/theory-cloud/tabletheory/v6/pkg/session"
)

func main() {
	db := newDB()
	for _, model := range []any{
		&examples.Session{},
		&examples.Bookmark{},
		&examples.BookmarkQuota{},
		&examples.BookmarkAudit{},
		&examples.Invoice{},
		&examples.Product{},
		&examples.EmbeddedCustomer{},
		&examples.FaceTheoryCacheMetadata{},
	} {
		if err := db.EnsureTable(model); err != nil {
			log.Fatalf("ensure table for %T: %v", model, err)
		}
	}

	checkConditionalHelpers(db)
	checkTransactionBuilder(db)
	batchInvoices := checkBatchGetBuilder(db)
	listTags := checkListOperations(db)
	orders := checkEmbeddedStructs(db)
	checkFaceTheoryTTL(db)

	fmt.Printf(
		"OK: loose examples drove conditional helpers, transaction builder, batch get (%d invoices), list operations (%d tags), embedded structs (%d orders) and the FaceTheory TTL cache on DynamoDB Local\n",
		batchInvoices, listTags, orders,
	)
}

func newDB() core.ExtendedDB {
	cfg := session.Config{
		Region:   "us-east-1",
		Endpoint: localEndpoint(),
		AWSConfigOptions: []func(*config.LoadOptions) error{
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")),
		},
	}
	db, err := tabletheory.New(cfg)
	if err != nil {
		log.Fatalf("failed to create TableTheory client: %v", err)
	}
	return db
}

func checkConditionalHelpers(db core.ExtendedDB) {
	if err := examples.DemonstrateConditionalHelpers(db); err != nil {
		log.Fatalf("conditional helpers: %v", err)
	}
	sessions, err := readAll[examples.Session](db, "PK", "USER#123")
	if err != nil {
		log.Fatalf("read conditional session: %v", err)
	}
	if len(sessions) != 0 {
		log.Fatalf("conditional delete left %d session rows, want 0", len(sessions))
	}
}

func checkTransactionBuilder(db core.ExtendedDB) {
	quota := &examples.BookmarkQuota{
		PK:        "QUOTA#user-42",
		SK:        "WINDOW#2024-06",
		Remaining: 3,
	}
	if err := db.Model(quota).Create(); err != nil {
		log.Fatalf("seed bookmark quota: %v", err)
	}

	if err := examples.DemonstrateTransactionBuilder(db); err != nil {
		log.Fatalf("transaction builder: %v", err)
	}

	quotas, err := readAll[examples.BookmarkQuota](db, "PK", "QUOTA#user-42")
	if err != nil {
		log.Fatalf("read bookmark quota: %v", err)
	}
	if len(quotas) != 1 || quotas[0].Remaining != 2 {
		log.Fatalf("bookmark quota = %+v, want one row with Remaining=2", quotas)
	}

	audits, err := readAll[examples.BookmarkAudit](db, "PK", "BOOKMARK#123")
	if err != nil {
		log.Fatalf("read bookmark audit: %v", err)
	}
	if len(audits) != 1 || audits[0].Action != "create" || audits[0].ActorID != "user-42" {
		log.Fatalf("bookmark audit = %+v, want one create row for user-42", audits)
	}

	bookmarks, err := readAll[examples.Bookmark](db, "PK", "BOOKMARK#123")
	if err != nil {
		log.Fatalf("read bookmark: %v", err)
	}
	if len(bookmarks) != 0 {
		log.Fatalf("transaction cleanup left %d bookmark rows, want 0", len(bookmarks))
	}
}

func checkBatchGetBuilder(db core.ExtendedDB) int {
	for index, sortKey := range []string{
		"INVOICE#2024-01",
		"INVOICE#2024-02",
		"INVOICE#2024-03",
		"INVOICE#2024-04",
	} {
		invoice := &examples.Invoice{
			PK:      "ORG#42",
			SK:      sortKey,
			Status:  "open",
			Total:   int64(100 + index),
			Balance: int64(10 + index),
		}
		if err := db.Model(invoice).Create(); err != nil {
			log.Fatalf("seed invoice %s: %v", sortKey, err)
		}
	}

	invoices, err := examples.DemonstrateBatchGetBuilder(db)
	if err != nil {
		log.Fatalf("batch get builder: %v", err)
	}
	if len(invoices) != 4 {
		log.Fatalf("batch get builder returned %d invoices, want 4", len(invoices))
	}
	var balance int64
	for _, invoice := range invoices {
		balance += invoice.Balance
	}
	if balance != 46 {
		log.Fatalf("batch get builder returned balance %d, want 46", balance)
	}
	return len(invoices)
}

func checkListOperations(db core.ExtendedDB) int {
	product := &examples.Product{
		ID:         "prod123",
		Name:       "Original Product Name",
		Tags:       []string{"base"},
		Categories: []string{"misc"},
		Labels:     []string{"obsolete-tag", "keep"},
	}
	if err := db.Model(product).Create(); err != nil {
		log.Fatalf("seed product: %v", err)
	}

	if err := examples.DemonstrateListOperations(db); err != nil {
		log.Fatalf("list operations: %v", err)
	}

	products, err := readAll[examples.Product](db, "ID", "prod123")
	if err != nil {
		log.Fatalf("read product: %v", err)
	}
	if len(products) != 1 {
		log.Fatalf("list operations produced %d product rows, want 1", len(products))
	}
	got := products[0]

	if got.Name != "Updated Product Name" {
		log.Fatalf("product name = %q, want %q", got.Name, "Updated Product Name")
	}
	if got.Description != "Default product description" {
		log.Fatalf("product description = %q, want the if_not_exists default", got.Description)
	}
	if got.ViewCount != 1 {
		log.Fatalf("product ViewCount = %d, want 1", got.ViewCount)
	}
	wantTags := []string{"first-tag", "sale", "base", "new-tag", "another-tag"}
	if !slices.Equal(got.Tags, wantTags) {
		log.Fatalf("product Tags = %v, want %v", got.Tags, wantTags)
	}
	wantCategories := []string{"electronics", "misc"}
	if !slices.Equal(got.Categories, wantCategories) {
		log.Fatalf("product Categories = %v, want %v", got.Categories, wantCategories)
	}
	labels := map[string]bool{}
	for _, label := range got.Labels {
		labels[label] = true
	}
	if labels["obsolete-tag"] || !labels["keep"] {
		log.Fatalf("product Labels = %v, want keep without obsolete-tag", got.Labels)
	}
	return len(got.Tags)
}

func checkEmbeddedStructs(db core.ExtendedDB) int {
	if err := examples.ExampleEmbeddedStructs(db); err != nil {
		log.Fatalf("embedded structs: %v", err)
	}

	customers, err := readIndexPrefix[examples.EmbeddedCustomer](
		db, "gsi1", "GSI1PK", "TENANT#tenant_456", "GSI1SK", "CUSTOMER#",
	)
	if err != nil {
		log.Fatalf("read embedded customers: %v", err)
	}
	if len(customers) != 1 || customers[0].Email != "johndoe@example.com" {
		log.Fatalf("embedded customers = %+v, want one with the updated email", customers)
	}

	byEmail, err := readIndexPrefix[examples.EmbeddedCustomer](
		db, "gsi2", "GSI2PK", "EMAIL#johndoe@example.com", "GSI2SK", "CUSTOMER#",
	)
	if err != nil {
		log.Fatalf("read embedded customer by gsi2: %v", err)
	}
	if len(byEmail) != 1 {
		log.Fatalf("gsi2 email lookup returned %d customers, want 1", len(byEmail))
	}

	orders, err := readIndexPrefix[examples.EmbeddedOrder](
		db, "gsi1", "GSI1PK", "CUSTOMER#cus_123", "GSI1SK", "ORDER#",
	)
	if err != nil {
		log.Fatalf("read embedded orders: %v", err)
	}
	if len(orders) != 1 {
		log.Fatalf("embedded orders = %d, want 1", len(orders))
	}
	return len(orders)
}

func checkFaceTheoryTTL(db core.ExtendedDB) {
	generatedAt := time.Now().Unix()
	meta := &examples.FaceTheoryCacheMetadata{
		PK:                "SITE#example",
		SK:                "PAGE#/",
		S3Key:             "site/example/index.html",
		GeneratedAt:       generatedAt,
		RevalidateSeconds: 60,
		TTL:               examples.FaceTheoryTTLUnix(generatedAt, 24*time.Hour),
	}
	if err := db.Model(meta).Create(); err != nil {
		log.Fatalf("create FaceTheory metadata: %v", err)
	}

	metas, err := readAll[examples.FaceTheoryCacheMetadata](db, "PK", "SITE#example")
	if err != nil {
		log.Fatalf("read FaceTheory metadata: %v", err)
	}
	if len(metas) != 1 {
		log.Fatalf("FaceTheory metadata rows = %d, want 1", len(metas))
	}
	got := metas[0]
	if got.TTL != generatedAt+86400 {
		log.Fatalf("FaceTheory TTL = %d, want %d", got.TTL, generatedAt+86400)
	}
	if examples.FaceTheoryFreshUntilUnix(got) != generatedAt+60 {
		log.Fatalf("FaceTheory fresh-until = %d, want %d", examples.FaceTheoryFreshUntilUnix(got), generatedAt+60)
	}
	if !examples.FaceTheoryIsFresh(got, time.Unix(generatedAt+59, 0)) {
		log.Fatal("FaceTheory metadata must be fresh one second before the revalidate boundary")
	}
	if examples.FaceTheoryIsFresh(got, time.Unix(generatedAt+60, 0)) {
		log.Fatal("FaceTheory metadata must be stale at the revalidate boundary")
	}
}

func readAll[T any](db core.ExtendedDB, field, value string) ([]T, error) {
	var out []T
	var zero T
	if err := db.Model(&zero).Where(field, "=", value).All(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func readIndexPrefix[T any](
	db core.ExtendedDB,
	index, partitionField, partitionValue, sortField, sortPrefix string,
) ([]T, error) {
	var out []T
	var zero T
	err := db.Model(&zero).
		Index(index).
		Where(partitionField, "=", partitionValue).
		Where(sortField, "BEGINS_WITH", sortPrefix).
		All(&out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// localEndpoint honours the local harness endpoint while keeping the historical
// localhost:8000 default for a hand-started DynamoDB Local.
func localEndpoint() string {
	for _, key := range []string{"DYNAMODB_ENDPOINT", "AWS_ENDPOINT_URL_DYNAMODB"} {
		if endpoint := strings.TrimSpace(os.Getenv(key)); endpoint != "" {
			return endpoint
		}
	}
	return "http://localhost:8000"
}
