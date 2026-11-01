// Package main drives the typed generic API against DynamoDB Local. The
// typedFlow function stays free of initialization so applications can pass a
// handle from their normal startup path; this driver supplies that handle.
package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"

	"github.com/theory-cloud/tabletheory/v6"
	"github.com/theory-cloud/tabletheory/v6/pkg/core"
	"github.com/theory-cloud/tabletheory/v6/pkg/query"
	"github.com/theory-cloud/tabletheory/v6/pkg/session"
)

type User struct {
	PK      string `theorydb:"pk" json:"PK"`
	SK      string `theorydb:"sk" json:"SK"`
	Name    string `json:"name"`
	Version int64  `theorydb:"version" json:"version"`
}

// TableName keeps this example's table distinct from other examples' User
// models when they share one local DynamoDB instance.
func (User) TableName() string { return "tabletheory_typed_go_users" }

func typedFlow(db core.DB) error {
	users := tabletheory.ModelOf[User](db)

	user := User{PK: "TENANT#demo", SK: "USER#ada", Name: "Ada"}
	if err := users.Create(&user); err != nil {
		return err
	}

	got, err := users.Get(users.Key("TENANT#demo", "USER#ada"))
	if err != nil {
		return err
	}

	got.Name = "Ada Lovelace"
	if err := users.Update(&got, "Name"); err != nil {
		return err
	}

	_, err = users.Query().Where("SK", query.OpBeginsWith, "USER#").First()
	return err
}

func main() {
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
	if err := db.EnsureTable(&User{}); err != nil {
		log.Fatalf("failed to ensure the typed-go table: %v", err)
	}
	if err := typedFlow(db); err != nil {
		log.Fatalf("typedFlow failed: %v", err)
	}

	users := tabletheory.ModelOf[User](db)
	got, err := users.Get(users.Key("TENANT#demo", "USER#ada"))
	if err != nil {
		log.Fatalf("typed read-back failed: %v", err)
	}
	if got.Name != "Ada Lovelace" {
		log.Fatalf("typed read-back name = %q, want %q", got.Name, "Ada Lovelace")
	}

	matches, err := users.Query().Where("SK", query.OpBeginsWith, "USER#").All()
	if err != nil {
		log.Fatalf("typed query failed: %v", err)
	}
	if len(matches) != 1 {
		log.Fatalf("typed query returned %d users, want 1", len(matches))
	}

	fmt.Printf(
		"OK: typed-go typedFlow wrote and updated %s/%s name=%q version=%d matches=%d\n",
		got.PK, got.SK, got.Name, got.Version, len(matches),
	)
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
