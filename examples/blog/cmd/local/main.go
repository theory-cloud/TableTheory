// Command local provisions and inspects the blog example's local DynamoDB
// tables. The tables use the same names the SAM template creates, so a local
// run and a deployed stage exercise the same model metadata.
//
// It is the local counterpart of the template's table resources: the examples
// harness runs `-mode setup` before invoking the Lambda handlers, and `make
// setup` runs it after starting the checked-in DynamoDB Local compose stack.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/theory-cloud/tabletheory/v6"
	"github.com/theory-cloud/tabletheory/v6/examples/blog/models"
	"github.com/theory-cloud/tabletheory/v6/pkg/core"
)

// templateTables are the tables deployment/sam-template.yaml creates. The
// Models method below maps each one to the model that owns it.
func templateTables() []struct {
	model any
} {
	return []struct{ model any }{
		{model: &models.Post{}},
		{model: &models.Comment{}},
		{model: &models.Author{}},
		{model: &models.SearchIndex{}},
	}
}

func main() {
	mode := flag.String("mode", "setup", "setup, verify or drop")
	flag.Parse()

	cfg := tabletheory.Config{
		Region:   envOr("AWS_REGION", "us-east-1"),
		Endpoint: os.Getenv("DYNAMODB_ENDPOINT"),
	}
	db, err := tabletheory.New(cfg)
	if err != nil {
		fatal("connect to DynamoDB: %v", err)
	}

	switch *mode {
	case "setup":
		setup(db)
	case "verify":
		verify(db)
	case "drop":
		drop(db)
	default:
		fatal("unknown -mode %q (want setup, verify or drop)", *mode)
	}
}

func setup(db core.ExtendedDB) {
	names := make([]string, 0, len(templateTables()))
	for _, entry := range templateTables() {
		if err := db.EnsureTable(entry.model); err != nil {
			fatal("ensure %T: %v", entry.model, err)
		}
		name := tableNameOf(entry.model)
		names = append(names, name)
		fmt.Printf("table: %s\n", name)
	}
	emit(map[string]any{"ok": true, "tables": names})
}

func verify(db core.ExtendedDB) {
	counts := make(map[string]int64, len(templateTables()))
	for _, entry := range templateTables() {
		name := tableNameOf(entry.model)
		count, err := db.Model(entry.model).Count()
		if err != nil {
			fatal("count %s: %v", name, err)
		}
		counts[name] = count
	}
	emit(map[string]any{"ok": true, "counts": counts})
}

func drop(db interface {
	DeleteTable(any) error
}) {
	for _, entry := range templateTables() {
		name := tableNameOf(entry.model)
		if err := db.DeleteTable(entry.model); err != nil {
			fmt.Fprintf(os.Stderr, "drop %s: %v\n", name, err)
			continue
		}
		fmt.Printf("table: %s dropped\n", name)
	}
}

func tableNameOf(model any) string {
	type named interface{ TableName() string }
	if value, ok := model.(named); ok {
		return value.TableName()
	}
	return fmt.Sprintf("%T", model)
}

func emit(payload map[string]any) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		fatal("encode result: %v", err)
	}
	fmt.Println(string(encoded))
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "blog local: "+format+"\n", args...)
	os.Exit(1)
}
