package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateCommandPasses(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	err := run([]string{"validate", filepath.Join("..", "..", "pkg", "dms", "testdata", "codegen", "dms-note.yml")}, &stdout, ioDiscard{})
	require.NoError(t, err)
	require.Contains(t, stdout.String(), "OK:")
	require.Contains(t, stdout.String(), "1 model(s)")
}

func TestValidateCommandReportsFileAndHint(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	err := run([]string{"validate", filepath.Join("testdata", "invalid.yml")}, &stdout, ioDiscard{})
	require.Error(t, err)
	require.Contains(t, err.Error(), filepath.Join("testdata", "invalid.yml"))
	require.Contains(t, err.Error(), "DMS validation failed")
	require.Contains(t, err.Error(), "Hint:")
}

func TestValidateCommandReportsYamlLineContext(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "broken.yml")
	require.NoError(t, os.WriteFile(path, []byte("dms_version: [\n"), 0o600))

	err := run([]string{"validate", path}, ioDiscard{}, ioDiscard{})
	require.Error(t, err)
	require.Contains(t, err.Error(), path+":1")
	require.Contains(t, err.Error(), "1 | dms_version: [")
}

func TestGenerateCommandWritesGoOutput(t *testing.T) {
	t.Parallel()

	out := filepath.Join(t.TempDir(), "models.go")
	err := run([]string{
		"gen",
		"--lang", "go",
		"--package", "generated",
		"--out", out,
		filepath.Join("..", "..", "pkg", "dms", "testdata", "codegen", "dms-note.yml"),
	}, ioDiscard{}, ioDiscard{})
	require.NoError(t, err)
	data, err := os.ReadFile(out) // #nosec G304 -- tests read their own generated output.
	require.NoError(t, err)
	require.Contains(t, string(data), "type DMSNote struct")
	require.Contains(t, string(data), "func (DMSNote) TableName() string")
}

func TestGenerateCommandWritesTypeScriptStdout(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	err := run([]string{
		"gen",
		"--lang", "ts",
		"--runtime-import", "../../src/index.js",
		filepath.Join("..", "..", "pkg", "dms", "testdata", "codegen", "dms-note.yml"),
	}, &stdout, ioDiscard{})
	require.NoError(t, err)
	require.Contains(t, stdout.String(), "DMSNoteSchema")
	require.Contains(t, stdout.String(), "defineModel")
}

func TestGenerateCommandWritesPythonStdout(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	err := run([]string{
		"gen",
		"--lang", "py",
		"--model", "DMSNote",
		filepath.Join("..", "..", "pkg", "dms", "testdata", "codegen", "dms-note.yml"),
	}, &stdout, ioDiscard{})
	require.NoError(t, err)
	require.Contains(t, stdout.String(), "class DMSNote")
	require.Contains(t, stdout.String(), "ModelDefinition.from_dataclass")
}

func TestGenerateCommandWritesCDKStdout(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	err := run([]string{
		"gen",
		"--cdk",
		filepath.Join("..", "..", "pkg", "dms", "testdata", "codegen", "dms-note.yml"),
	}, &stdout, ioDiscard{})
	require.NoError(t, err)
	require.Contains(t, stdout.String(), "createDMSNoteTable")
	require.Contains(t, stdout.String(), "aws-cdk-lib/aws-dynamodb")
	require.Contains(t, stdout.String(), "timeToLiveAttribute: 'ttl'")
	require.Contains(t, stdout.String(), "addGlobalSecondaryIndex")
}

func TestGenerateCommandRejectsCDKWithLang(t *testing.T) {
	t.Parallel()

	err := run([]string{
		"gen", "--cdk", "--lang", "go",
		filepath.Join("..", "..", "pkg", "dms", "testdata", "codegen", "dms-note.yml"),
	}, ioDiscard{}, ioDiscard{})
	require.ErrorContains(t, err, "cannot be combined with --lang")
}

func TestGenerateCommandRequiresLanguage(t *testing.T) {
	t.Parallel()

	err := run([]string{"gen", filepath.Join("..", "..", "pkg", "dms", "testdata", "codegen", "dms-note.yml")}, ioDiscard{}, ioDiscard{})
	require.ErrorContains(t, err, "gen requires --lang")
}

func TestGenerateCommandReportsArgumentAndGenerationErrors(t *testing.T) {
	t.Parallel()

	dmsPath := filepath.Join("..", "..", "pkg", "dms", "testdata", "codegen", "dms-note.yml")

	err := run([]string{"gen", "--lang", "go"}, ioDiscard{}, ioDiscard{})
	require.ErrorContains(t, err, "gen requires exactly one DMS file")

	err = run([]string{"gen", "--lang", "go", "--model", "Missing", dmsPath}, ioDiscard{}, ioDiscard{})
	require.ErrorContains(t, err, "generate go")
	require.ErrorContains(t, err, "DMS model not found")
}

func TestContractGenerateTSAlias(t *testing.T) {
	t.Parallel()

	out := filepath.Join(t.TempDir(), "keys.ts")
	err := run([]string{
		"contract", "generate-ts",
		"--contract", filepath.Join("..", "..", "contract-tests", "key-contracts", "v0.1", "theorymcp-derived-keys.yml"),
		"--out", out,
	}, ioDiscard{}, ioDiscard{})
	require.NoError(t, err)
	data, err := os.ReadFile(out) // #nosec G304 -- tests read their own generated output.
	require.NoError(t, err)
	require.Contains(t, string(data), "export function canonicalPolicyKey")
}

func TestRunUsageAndUnknownCommand(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	require.NoError(t, run([]string{"help"}, &stdout, ioDiscard{}))
	require.Contains(t, stdout.String(), "tabletheory validate")

	err := run(nil, ioDiscard{}, ioDiscard{})
	require.ErrorContains(t, err, "missing command")

	err = run([]string{"unknown"}, ioDiscard{}, ioDiscard{})
	require.ErrorContains(t, err, "unknown command")
}

func TestContractUsageAndErrors(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	require.NoError(t, run([]string{"contract", "help"}, &stdout, ioDiscard{}))
	require.Contains(t, stdout.String(), "contract generate-ts")

	err := run([]string{"contract"}, ioDiscard{}, ioDiscard{})
	require.ErrorContains(t, err, "contract requires a subcommand")

	err = run([]string{"contract", "unknown"}, ioDiscard{}, ioDiscard{})
	require.ErrorContains(t, err, "unknown contract subcommand")

	err = run([]string{"contract", "generate-ts", "--contract", filepath.Join("..", "..", "contract-tests", "key-contracts", "v0.1", "theorymcp-derived-keys.yml")}, ioDiscard{}, ioDiscard{})
	require.ErrorContains(t, err, "requires --out")
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
