package spec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadModelsDirRejectsDMSV01(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "model.yml")
	content := `
dms_version: "0.1"
models:
  - name: "User"
`
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	_, err := LoadModelsDir(dir)
	if err == nil || !strings.Contains(err.Error(), `unsupported dms_version "0.1"`) {
		t.Fatalf("expected unsupported DMS version error, got %v", err)
	}
}
