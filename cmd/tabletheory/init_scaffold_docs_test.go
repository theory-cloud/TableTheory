package main

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInitScaffoldPythonReadmeDocumentsRuntimePackage(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "app")
	err := run([]string{"init", "--lang", "py", "--dir", dir}, ioDiscard{}, ioDiscard{})
	require.NoError(t, err)

	readme := readFile(t, filepath.Join(dir, "README.md"))
	require.Contains(t, readme, "tabletheory_py")
	require.NotContains(t, readme, "theorydb_py")
}
