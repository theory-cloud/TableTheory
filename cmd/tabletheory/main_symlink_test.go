package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateOutReplacesFinalSymlinkWithoutTouchingVictim(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	victim := filepath.Join(t.TempDir(), "victim.go")
	require.NoError(t, os.WriteFile(victim, []byte("victim"), 0o600))
	link := filepath.Join(dir, "models.go")
	require.NoError(t, os.Symlink(victim, link))

	err := run([]string{
		"gen", "--lang", "go", "--package", "generated", "--out", link, codegenDmsFixture(),
	}, ioDiscard{}, ioDiscard{})
	require.NoError(t, err)

	require.Equal(t, "victim", readFile(t, victim))
	require.Contains(t, readFile(t, link), "type DMSNote struct")
	info, statErr := os.Lstat(link)
	require.NoError(t, statErr)
	require.Zero(t, info.Mode()&fs.ModeSymlink)
	require.True(t, info.Mode().IsRegular())
}

func TestGenerateOutRefusesEscapingParentSymlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "out")))

	err := run([]string{
		"gen", "--lang", "go", "--out", filepath.Join(root, "out", "models.go"), codegenDmsFixture(),
	}, ioDiscard{}, ioDiscard{})
	require.Error(t, err)
	require.NoFileExists(t, filepath.Join(outside, "models.go"))
}

func TestContractGenerateTSAliasReplacesFinalSymlink(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	victim := filepath.Join(t.TempDir(), "victim.ts")
	require.NoError(t, os.WriteFile(victim, []byte("victim"), 0o600))
	link := filepath.Join(dir, "keys.ts")
	require.NoError(t, os.Symlink(victim, link))

	err := run([]string{
		"contract", "generate-ts",
		"--contract", filepath.Join("..", "..", "contract-tests", "key-contracts", "v0.1", "theorymcp-derived-keys.yml"),
		"--out", link,
	}, ioDiscard{}, ioDiscard{})
	require.NoError(t, err)

	require.Equal(t, "victim", readFile(t, victim))
	require.Contains(t, readFile(t, link), "export function canonicalPolicyKey")
}

func codegenDmsFixture() string {
	return filepath.Join("..", "..", "pkg", "dms", "testdata", "codegen", "dms-note.yml")
}
