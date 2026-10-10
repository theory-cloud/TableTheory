package safeoutput

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteFileWithinWritesContent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, WriteFileWithin(root, filepath.Join("src", "main.ts"), []byte("body"), 0o600))

	require.Equal(t, "body", readFile(t, filepath.Join(root, "src", "main.ts")))
	info, err := os.Stat(filepath.Join(root, "src"))
	require.NoError(t, err)
	require.True(t, info.IsDir())
}

func TestWriteFileWithinRejectsEscapes(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for _, rel := range []string{
		"",
		".",
		"..",
		filepath.Join("..", "evil.txt"),
		filepath.Join(root, "absolute.txt"),
	} {
		require.Error(t, WriteFileWithin(root, rel, []byte("x"), 0o600), rel)
	}
	require.NoFileExists(t, filepath.Join(filepath.Dir(root), "evil.txt"))
}

func TestWriteFileWithinReplacesFinalSymlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	victim := writeVictimFile(t)
	require.NoError(t, os.Symlink(victim, filepath.Join(root, "main.ts")))

	require.NoError(t, WriteFileWithin(root, "main.ts", []byte("generated"), 0o600))

	require.Equal(t, "victim", readFile(t, victim))
	require.Equal(t, "generated", readFile(t, filepath.Join(root, "main.ts")))
	requireRegularFile(t, filepath.Join(root, "main.ts"))
}

func TestWriteFileWithinRefusesEscapingParentSymlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "src")))

	err := WriteFileWithin(root, filepath.Join("src", "main.ts"), []byte("generated"), 0o600)
	require.Error(t, err)
	require.NoFileExists(t, filepath.Join(outside, "main.ts"))
}

func TestWriteFileWithinFollowsInternalSymlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "real"), 0o750))
	require.NoError(t, os.Symlink("real", filepath.Join(root, "link")))

	require.NoError(t, WriteFileWithin(root, filepath.Join("link", "main.ts"), []byte("generated"), 0o600))
	require.Equal(t, "generated", readFile(t, filepath.Join(root, "real", "main.ts")))
}

func TestWriteFileAtomicWritesContent(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "a", "b", "keys.ts")
	require.NoError(t, WriteFileAtomic(path, []byte("generated"), 0o600))
	require.Equal(t, "generated", readFile(t, path))
}

func TestWriteFileAtomicReplacesFinalSymlink(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	victim := writeVictimFile(t)
	link := filepath.Join(dir, "keys.ts")
	require.NoError(t, os.Symlink(victim, link))

	require.NoError(t, WriteFileAtomic(link, []byte("generated"), 0o600))

	require.Equal(t, "victim", readFile(t, victim))
	require.Equal(t, "generated", readFile(t, link))
	requireRegularFile(t, link)
}

func TestWriteFileAtomicRefusesEscapingParentSymlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "out")))

	err := WriteFileAtomic(filepath.Join(root, "out", "keys.ts"), []byte("generated"), 0o600)
	require.Error(t, err)
	require.NoFileExists(t, filepath.Join(outside, "keys.ts"))
}

func TestWriteFileAtomicRejectsEmptyAndDirectory(t *testing.T) {
	t.Parallel()

	require.Error(t, WriteFileAtomic("", []byte("x"), 0o600))

	dir := t.TempDir()
	require.Error(t, WriteFileAtomic(dir, []byte("x"), 0o600))
}

func writeVictimFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "victim.txt")
	require.NoError(t, os.WriteFile(path, []byte("victim"), 0o600))
	return path
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path) // #nosec G304 -- tests read files they created.
	require.NoError(t, err)
	return string(data)
}

func requireRegularFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Lstat(path)
	require.NoError(t, err)
	require.Zero(t, info.Mode()&fs.ModeSymlink)
	require.True(t, info.Mode().IsRegular())
}
