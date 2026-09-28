package tests

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"
)

// This file bridges the Go guard tests to the real parsers behind the Python
// and TypeScript detectors. Python sources are parsed with Python's own `ast`
// module and TypeScript/JavaScript sources with the TypeScript compiler API
// (from ts/node_modules/typescript). Both helpers live under
// tests/detachedwork/ and speak one batch protocol, so a reviewer can read
// either language's rule set as a single file and the Go test only has to
// marshal a request and read a response.
//
// The helpers run with the repository's own toolchains — `python3` and `node`,
// which CI installs before `make rubric`. A missing interpreter or a source the
// parser rejects fails the test; the guard never quietly stops covering a
// language.

// astSource is one source handed to a language helper.
type astSource struct {
	Path   string `json:"path"`
	Source string `json:"source"`
}

// astRequest is the helper request body.
type astRequest struct {
	Files []astSource `json:"files"`
}

// astFinding is one reported construct; Line is 1-based.
type astFinding struct {
	Text string `json:"text"`
	Rule string `json:"rule"`
	Line int    `json:"line"`
}

// astFileResult is one file's findings, or the parse error that stopped them.
type astFileResult struct {
	Path     string       `json:"path"`
	Error    string       `json:"error"`
	Findings []astFinding `json:"findings"`
}

// astResponse is the helper response body.
type astResponse struct {
	Results []astFileResult `json:"results"`
}

// scannerScript resolves a helper entry point from the repository root, so the
// working directory `go test` happens to use does not matter.
func scannerScript(t *testing.T, relative string) string {
	t.Helper()
	return filepath.Join(detachedWorkRepoRoot(t), filepath.FromSlash(relative))
}

// runASTScanner runs one prepared language helper over a batch of sources and
// returns one result per source, in order. A helper that cannot start, a parse
// error, or a malformed response is a test failure.
//
// interpreter is used only to name the toolchain in diagnostics; the command
// itself is built by the caller from a fixed literal.
func runASTScanner(t *testing.T, command *exec.Cmd, interpreter string, sources []astSource) []astFileResult {
	t.Helper()
	if len(sources) == 0 {
		return nil
	}
	if _, err := exec.LookPath(interpreter); err != nil {
		t.Fatalf("detached-work guard needs %q on PATH to parse sources: %v", interpreter, err)
	}
	payload, err := json.Marshal(astRequest{Files: sources})
	if err != nil {
		t.Fatalf("encode scanner request: %v", err)
	}
	command.Stdin = bytes.NewReader(payload)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("detached-work scanner %s failed: %v\nstderr: %s", interpreter, err, stderr.String())
	}
	var response astResponse
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatalf("decode scanner response: %v\nstdout: %s", err, stdout.String())
	}
	if len(response.Results) != len(sources) {
		t.Fatalf("scanner returned %d results for %d sources", len(response.Results), len(sources))
	}
	for _, result := range response.Results {
		if result.Error != "" {
			t.Errorf("scanner could not parse %s: %s", result.Path, result.Error)
		}
	}
	return response.Results
}

// scanPythonBatch parses Python sources with Python's `ast` module and returns
// each source's findings in order.
func scanPythonBatch(t *testing.T, sources []astSource) [][]pyFinding {
	t.Helper()
	if len(sources) == 0 {
		return nil
	}
	script := scannerScript(t, "tests/detachedwork/scan_python.py")
	//nolint:gosec // G204: the interpreter is a fixed toolchain literal and the script is resolved from the repository root.
	command := exec.CommandContext(t.Context(), "python3", script)
	results := runASTScanner(t, command, "python3", sources)
	findings := make([][]pyFinding, len(results))
	for i, result := range results {
		for _, f := range result.Findings {
			findings[i] = append(findings[i], pyFinding(f))
		}
	}
	return findings
}

// scanTypeScriptBatch parses TypeScript/JavaScript sources with the TypeScript
// compiler API and returns each source's findings in order.
func scanTypeScriptBatch(t *testing.T, sources []astSource) [][]tsFinding {
	t.Helper()
	if len(sources) == 0 {
		return nil
	}
	script := scannerScript(t, "tests/detachedwork/scan_typescript.mjs")
	//nolint:gosec // G204: the interpreter is a fixed toolchain literal and the script is resolved from the repository root.
	command := exec.CommandContext(t.Context(), "node", script)
	results := runASTScanner(t, command, "node", sources)
	findings := make([][]tsFinding, len(results))
	for i, result := range results {
		for _, f := range result.Findings {
			findings[i] = append(findings[i], tsFinding(f))
		}
	}
	return findings
}
