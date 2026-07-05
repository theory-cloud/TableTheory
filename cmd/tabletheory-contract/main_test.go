package main

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateTSCommand(t *testing.T) {
	t.Parallel()

	out := filepath.Join(t.TempDir(), "generated", "keys.ts")
	err := run([]string{
		"generate-ts",
		"--contract", filepath.Join("..", "..", "contract-tests", "key-contracts", "v0.1", "theorymcp-derived-keys.yml"),
		"--out", out,
	})
	require.NoError(t, err)

	data, err := readTestFileScoped(out)
	require.NoError(t, err)
	got := string(data)
	require.Contains(t, got, "export function canonicalPolicyKey")
	require.Contains(t, got, "export function canonicalBindingKey")
	require.Contains(t, got, "export function interfaceScopeKey")
	require.Contains(t, got, "export function importSessionScopeKey")
}

func TestGenerateTSCommandRequiresArguments(t *testing.T) {
	t.Parallel()

	err := run([]string{"generate-ts"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "requires --contract")

	err = run([]string{"generate-ts", "--contract", filepath.Join("..", "..", "contract-tests", "key-contracts", "v0.1", "theorymcp-derived-keys.yml")})
	require.Error(t, err)
	require.Contains(t, err.Error(), "requires --out")
}

func TestContractCommandUsageAndUnknownCommand(t *testing.T) {
	t.Parallel()

	require.NoError(t, run([]string{"help"}))

	err := run(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing command")

	err = run([]string{"unknown"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown command")
}

func readTestFileScoped(path string) (data []byte, err error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	dir, name := filepath.Split(abs)
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := root.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()

	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := file.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()
	return io.ReadAll(file)
}
