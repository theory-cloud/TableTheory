package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// examplesDetachedWorkAllowlist lists the example Go files that may start a
// goroutine because the example really is a long-lived process rather than a
// Lambda handler. Every entry carries a justification so that adding one is a
// conscious decision; anything not listed here must deliver its work on the
// invocation path, because Lambda freezes the execution environment the moment
// the handler returns and a frozen goroutine can resume against an invocation
// that is already over.
var examplesDetachedWorkAllowlist = map[string]string{
	"examples/multi-tenant/cmd/local/main.go": "local dev HTTP server, not a Lambda handler: main starts ListenAndServe and then blocks on SIGINT/SIGTERM before a graceful shutdown",
}

// goLaunch matches a Go `go` statement that runs a function on a new goroutine:
// `go worker(i)`, `go s.worker(i)`, `go r.run()`, `go func() { ... }()`. The
// pattern requires a call, so prose in a raw string (for example "go mod tidy")
// and the `//go:build` / `//go:embed` directives never match.
var goLaunch = regexp.MustCompile(`^\s*go\s+(?:func|[A-Za-z_][A-Za-z0-9_.]*)\s*\(`)

// TestExamples_NoDetachedWorkInLambdaEntrypoints fails when an example starts
// work that can outlive the invocation that started it. The blog example
// deploys only as Lambda functions and the payment example only as Lambda
// handlers, so neither may fork a goroutine; the local CLI is allowlisted.
func TestExamples_NoDetachedWorkInLambdaEntrypoints(t *testing.T) {
	root := examplesRepoRoot(t)
	examplesDir := filepath.Join(root, "examples")

	scanned := 0
	seenAllowlisted := make(map[string]bool, len(examplesDetachedWorkAllowlist))
	var problems []string

	walkErr := filepath.WalkDir(examplesDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".venv", "build", "cdk.out", "dist", "node_modules", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		scanned++

		if _, allowed := examplesDetachedWorkAllowlist[rel]; allowed {
			seenAllowlisted[rel] = true
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(content), "\n") {
			if goLaunch.MatchString(line) {
				problems = append(problems, fmt.Sprintf("%s:%d: %s", rel, i+1, strings.TrimSpace(line)))
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("scan examples: %v", walkErr)
	}

	if scanned == 0 {
		t.Fatal("guard is vacuous: no example Go files were scanned")
	}
	for rel := range examplesDetachedWorkAllowlist {
		if !seenAllowlisted[rel] {
			t.Errorf("allowlist entry %q matches no scanned example file; remove it or fix the path", rel)
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		t.Fatalf(
			"examples must not start work that can outlive a Lambda invocation.\n"+
				"Deliver it on the invocation path instead, or add the file to examplesDetachedWorkAllowlist with a justification:\n%s",
			strings.Join(problems, "\n"),
		)
	}
}

// TestExamplesDetachedWorkDetectorIsNotVacuous proves the detector above is not
// vacuous: it must flag real goroutine launches and must not flag Go directives,
// comments, prose, or a filename.
func TestExamplesDetachedWorkDetectorIsNotVacuous(t *testing.T) {
	launches := []string{
		"go worker(i)",
		"\tgo s.worker(i)",
		"    go r.run()",
		"go func() {",
		"go send(ctx, notification)",
	}
	for _, line := range launches {
		if !goLaunch.MatchString(line) {
			t.Errorf("detector missed a goroutine launch: %q", line)
		}
	}

	ignored := []string{
		"//go:build linux && amd64",
		"//go:embed dms/demo.yml",
		"// go worker(i) is deliberately not used here",
		"go mod tidy",
		"worker.go(1)",
	}
	for _, line := range ignored {
		if goLaunch.MatchString(line) {
			t.Errorf("detector false-positive on %q", line)
		}
	}
}

// examplesRepoRoot resolves the repository root from this test file's location,
// independent of the working directory `go test` happens to use.
func examplesRepoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve this test file's path")
	}
	root := filepath.Dir(filepath.Dir(file))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("resolved repo root %q has no go.mod: %v", root, err)
	}
	return root
}
