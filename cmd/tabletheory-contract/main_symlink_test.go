package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const symlinkContractFixture = "../../contract-tests/key-contracts/v0.1/theorymcp-derived-keys.yml"

func TestGenerateTSReplacesFinalSymlinkWithoutTouchingVictim(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	victim := filepath.Join(t.TempDir(), "victim.ts")
	require.NoError(t, os.WriteFile(victim, []byte("victim"), 0o600))
	link := filepath.Join(dir, "keys.ts")
	require.NoError(t, os.Symlink(victim, link))

	require.NoError(t, run([]string{"generate-ts", "--contract", symlinkContractFixture, "--out", link}))

	require.Equal(t, "victim", mustReadFile(t, victim))
	require.Contains(t, mustReadFile(t, link), "export function canonicalPolicyKey")
	info, err := os.Lstat(link)
	require.NoError(t, err)
	require.Zero(t, info.Mode()&fs.ModeSymlink)
	require.True(t, info.Mode().IsRegular())
}

func TestGenerateTSRefusesEscapingParentSymlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "out")))

	err := run([]string{"generate-ts", "--contract", symlinkContractFixture, "--out", filepath.Join(root, "out", "keys.ts")})
	require.Error(t, err)
	require.NoFileExists(t, filepath.Join(outside, "keys.ts"))
	entries, readErr := os.ReadDir(outside)
	require.NoError(t, readErr)
	require.Empty(t, entries)
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path) // #nosec G304 -- tests read files they created.
	require.NoError(t, err)
	return string(data)
}
