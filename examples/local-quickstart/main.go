package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/theory-cloud/tabletheory"
	"github.com/theory-cloud/tabletheory/pkg/session"
)

// Note is the local quickstart model. The theorydb tags are TableTheory's
// cross-language contract surface; matching json tags keep the DynamoDB
// attribute names explicit.
type Note struct {
	PK        string    `theorydb:"pk" json:"PK"`
	SK        string    `theorydb:"sk" json:"SK"`
	Title     string    `json:"title,omitempty"`
	Value     int64     `json:"value,omitempty"`
	CreatedAt time.Time `theorydb:"created_at" json:"createdAt"`
	UpdatedAt time.Time `theorydb:"updated_at" json:"updatedAt"`
	Version   int64     `theorydb:"version" json:"version"`
}

func (Note) TableName() string { return "tabletheory_go_quickstart" }

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func main() {
	db, err := tabletheory.New(session.Config{
		Region:   envOr("AWS_REGION", "us-east-1"),
		Endpoint: envOr("DYNAMODB_ENDPOINT", "http://localhost:8000"),
	})
	if err != nil {
		log.Fatalf("connect to DynamoDB: %v", err)
	}

	if err := db.CreateTable(&Note{}); err != nil && !strings.Contains(err.Error(), "ResourceInUseException") {
		log.Fatalf("create table: %v", err)
	}

	note := &Note{PK: "NOTE#local", SK: fmt.Sprintf("run#%d", time.Now().UnixNano()), Title: "Hello TableTheory", Value: 42}
	if err := db.Model(note).IfNotExists().Create(); err != nil {
		log.Fatalf("create: %v", err)
	}
	fmt.Printf("created note %s (version %d)\n", note.PK, note.Version)

	var got Note
	if err := db.Model(&Note{}).Where("PK", "=", note.PK).Where("SK", "=", note.SK).First(&got); err != nil {
		log.Fatalf("read: %v", err)
	}
	fmt.Printf("read note: title=%q value=%d version=%d\n", got.Title, got.Value, got.Version)

	got.Title = "Hello TableTheory (updated)"
	if err := db.Model(&got).Update("title"); err != nil {
		log.Fatalf("update: %v", err)
	}
	fmt.Printf("updated note %s (version %d)\n", got.PK, got.Version)

	if err := db.Model(&Note{}).Where("PK", "=", note.PK).Where("SK", "=", note.SK).Delete(); err != nil {
		log.Fatalf("delete: %v", err)
	}
	fmt.Printf("deleted note %s\n", got.PK)

	fmt.Println("OK: TableTheory Go quickstart CRUD against DynamoDB Local succeeded")
}
