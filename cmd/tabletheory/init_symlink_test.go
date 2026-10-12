package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInitForceRejectsSymlinkedTargetDir(t *testing.T) {
	t.Parallel()

	victimDir := t.TempDir()
	marker := filepath.Join(victimDir, "marker.txt")
	require.NoError(t, os.WriteFile(marker, []byte("keep"), 0o600))

	link := filepath.Join(t.TempDir(), "app")
	require.NoError(t, os.Symlink(victimDir, link))

	err := run([]string{"init", "--lang", "go", "--dir", link, "--force"}, ioDiscard{}, ioDiscard{})
	require.ErrorContains(t, err, "symbolic link")

	require.Equal(t, "keep", readFile(t, marker))
	require.NoFileExists(t, filepath.Join(victimDir, "go.mod"))
	entries, readErr := os.ReadDir(victimDir)
	require.NoError(t, readErr)
	require.Len(t, entries, 1)
}

func TestInitForceReplacesFinalSymlinkWithoutTouchingVictim(t *testing.T) {
	t.Parallel()

	victim := filepath.Join(t.TempDir(), "victim.yml")
	require.NoError(t, os.WriteFile(victim, []byte("victim"), 0o600))

	dir := t.TempDir()
	require.NoError(t, os.Symlink(victim, filepath.Join(dir, "dms.yml")))

	require.NoError(t, run([]string{"init", "--lang", "go", "--dir", dir, "--force"}, ioDiscard{}, ioDiscard{}))

	require.Equal(t, "victim", readFile(t, victim))
	require.Contains(t, readFile(t, filepath.Join(dir, "dms.yml")), "dms_version")
	info, err := os.Lstat(filepath.Join(dir, "dms.yml"))
	require.NoError(t, err)
	require.Zero(t, info.Mode()&fs.ModeSymlink)
	require.True(t, info.Mode().IsRegular())
}

func TestInitRefusesEscapingParentSymlinkUnderRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "src")))

	err := run([]string{"init", "--lang", "ts", "--dir", root, "--force"}, ioDiscard{}, ioDiscard{})
	require.Error(t, err)

	require.NoFileExists(t, filepath.Join(outside, "main.ts"))
	entries, readErr := os.ReadDir(outside)
	require.NoError(t, readErr)
	require.Empty(t, entries)
}
