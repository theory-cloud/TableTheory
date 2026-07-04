// Command local-writer writes the shared cdk-multilang DemoItem to a local
// DynamoDB (no AWS account). It is the Go half of the local cross-language
// no-drift check driven by run-local.sh.
package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/theory-cloud/tabletheory"
	"github.com/theory-cloud/tabletheory/pkg/session"
)

// DemoItem is the shared, un-encrypted subset of the cdk-multilang DemoItem
// model. Encryption is intentionally omitted for the local variant: encrypted
// fields fail closed without KMS, which is not available in a no-AWS run.
type DemoItem struct {
	PK    string `theorydb:"pk" json:"PK"`
	SK    string `theorydb:"sk" json:"SK"`
	Value string `theorydb:"attr:value,omitempty" json:"value,omitempty"`
	Lang  string `theorydb:"attr:lang,omitempty" json:"lang,omitempty"`
}

// TableName resolves the shared table from the environment.
func (DemoItem) TableName() string { return envOr("DEMO_TABLE_NAME", "demo_multilang_local") }

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	db, err := tabletheory.New(session.Config{
		Region:   envOr("AWS_REGION", "us-east-1"),
		Endpoint: envOr("DYNAMODB_ENDPOINT", "http://localhost:8020"),
	})
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	if err := db.CreateTable(&DemoItem{}); err != nil && !strings.Contains(err.Error(), "ResourceInUseException") {
		log.Fatalf("create table: %v", err)
	}
	item := &DemoItem{PK: "demo#1", SK: "v1", Value: "shared-value", Lang: "go"}
	if err := db.Model(item).Create(); err != nil {
		log.Fatalf("write: %v", err)
	}
	fmt.Println("go: wrote demo#1/v1 (lang=go)")
}
