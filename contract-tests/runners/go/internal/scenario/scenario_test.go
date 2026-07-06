package scenario

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFileRejectsEmptyAssertionMaps(t *testing.T) {
	path := writeScenarioFixture(t, `
name: "unit.empty_assertion"
dms_version: "0.1"
model: "User"
steps:
  - op: get
    key: { PK: "USER#empty", SK: "PROFILE" }
    expect:
      item_equals: {}
`)

	_, err := LoadFile(path)
	if err == nil || !strings.Contains(err.Error(), "expect.item_equals must not be empty") {
		t.Fatalf("expected empty assertion error, got %v", err)
	}
}

func TestLoadFileRejectsItemAssertionsWithErrorExpectations(t *testing.T) {
	path := writeScenarioFixture(t, `
name: "unit.error_with_assertion"
dms_version: "0.1"
model: "User"
steps:
  - op: get
    key: { PK: "USER#error", SK: "PROFILE" }
    expect:
      error: "ErrItemNotFound"
      item_contains:
        PK: "USER#error"
`)

	_, err := LoadFile(path)
	if err == nil || !strings.Contains(err.Error(), "item/read assertions cannot be combined with error expectations") {
		t.Fatalf("expected error/assertion combination error, got %v", err)
	}
}

func writeScenarioFixture(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "scenario.yml")
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}
