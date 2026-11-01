// Command local-writer writes the shared cdk-multilang DemoItem to a local
// DynamoDB and KMS (no AWS account). It is the Go half of the local
// cross-language no-drift check driven by run-local.sh.
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/theory-cloud/tabletheory/v6"
	"github.com/theory-cloud/tabletheory/v6/pkg/session"
)

// DemoItem is the local variant of the deployed DemoItem model. The secret
// field carries the same theorydb:"encrypted" tag as the deployed handlers, so
// the local run exercises the real KMS envelope path in every runtime.
type DemoItem struct {
	PK     string `theorydb:"pk" json:"PK"`
	SK     string `theorydb:"sk" json:"SK"`
	Value  string `theorydb:"attr:value,omitempty" json:"value,omitempty"`
	Lang   string `theorydb:"attr:lang,omitempty" json:"lang,omitempty"`
	Secret string `theorydb:"encrypted,attr:secret,omitempty" json:"secret,omitempty"`
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
	kmsKeyARN := os.Getenv("KMS_KEY_ARN")
	if kmsKeyARN == "" {
		log.Fatal("KMS_KEY_ARN is required for the encrypted local variant")
	}
	db, err := tabletheory.New(session.Config{
		Region:    envOr("AWS_REGION", "us-east-1"),
		Endpoint:  envOr("DYNAMODB_ENDPOINT", "http://localhost:8020"),
		KMSKeyARN: kmsKeyARN,
	})
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	if err := db.EnsureTable(&DemoItem{}); err != nil {
		log.Fatalf("ensure table: %v", err)
	}
	item := &DemoItem{PK: "demo#1", SK: "v1", Value: "shared-value", Lang: "go", Secret: "shared-secret-v1"}
	if err := db.Model(item).CreateOrUpdate(); err != nil {
		log.Fatalf("write: %v", err)
	}

	var out DemoItem
	if err := db.Model(&DemoItem{}).Where("PK", "=", item.PK).Where("SK", "=", item.SK).First(&out); err != nil {
		log.Fatalf("read back: %v", err)
	}
	if out.Secret != item.Secret {
		log.Fatalf("decrypt assertion failed: read back %q, want %q", out.Secret, item.Secret)
	}
	fmt.Println("go: wrote demo#1/v1 (lang=go, secret=shared-secret-v1 encrypted)")
}
