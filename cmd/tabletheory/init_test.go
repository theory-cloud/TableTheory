package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v3/pkg/dms"
)

func TestInitScaffoldGo(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "app")
	var stdout bytes.Buffer
	err := run([]string{"init", "--lang", "go", "--dir", dir, "--module", "example.com/demo"}, &stdout, ioDiscard{})
	require.NoError(t, err)

	for _, name := range []string{"dms.yml", "docker-compose.yml", "main.go", "go.mod", "Makefile", "README.md"} {
		require.FileExists(t, filepath.Join(dir, name))
	}

	gomod := readFile(t, filepath.Join(dir, "go.mod"))
	require.Contains(t, gomod, "module example.com/demo")
	require.Contains(t, gomod, "github.com/theory-cloud/tabletheory/v2 v2.0.6")

	main := readFile(t, filepath.Join(dir, "main.go"))
	require.Contains(t, main, "func (Note) TableName() string")
	require.Contains(t, main, "db.Model(note).Create()")
	require.Contains(t, main, `db.Model(&got).Update("title")`)
	require.Contains(t, main, "does not mutate got.Version")
	require.Contains(t, main, "read after update")
	require.Contains(t, main, "persistedVersion=%d")
	require.NotContains(t, main, `updated note %s (version %d)`)
	require.Contains(t, main, "OK: TableTheory CRUD against DynamoDB Local succeeded")

	// The generated DMS must be valid.
	doc, err := dms.ParseDocument([]byte(readFile(t, filepath.Join(dir, "dms.yml"))))
	require.NoError(t, err)
	require.Len(t, doc.Models, 1)
	require.Equal(t, "Note", doc.Models[0].Name)

	require.Contains(t, stdout.String(), "Scaffolded TableTheory go quickstart")
}

func TestInitScaffoldTypeScript(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "app")
	err := run([]string{"init", "--lang", "ts", "--dir", dir, "--module", "demo-ts"}, ioDiscard{}, ioDiscard{})
	require.NoError(t, err)

	require.FileExists(t, filepath.Join(dir, "src", "main.ts"))
	require.FileExists(t, filepath.Join(dir, "package.json"))
	require.FileExists(t, filepath.Join(dir, "tsconfig.json"))

	pkg := readFile(t, filepath.Join(dir, "package.json"))
	require.Contains(t, pkg, `"name": "demo-ts"`)
	require.Contains(t, pkg, "theory-cloud-tabletheory-ts-2.0.6.tgz")

	main := readFile(t, filepath.Join(dir, "src", "main.ts"))
	require.Contains(t, main, "ensureTable(ddb, Note)")
	require.Contains(t, main, "OK: TableTheory CRUD against DynamoDB Local succeeded")
}

func TestInitScaffoldPython(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "app")
	err := run([]string{"init", "--lang", "py", "--dir", dir}, ioDiscard{}, ioDiscard{})
	require.NoError(t, err)

	require.FileExists(t, filepath.Join(dir, "main.py"))
	require.FileExists(t, filepath.Join(dir, "requirements.txt"))

	req := readFile(t, filepath.Join(dir, "requirements.txt"))
	require.Contains(t, req, "tabletheory_py-2.0.6-py3-none-any.whl")

	main := readFile(t, filepath.Join(dir, "main.py"))
	require.Contains(t, main, "ensure_table(model, client=client)")
	require.Contains(t, main, "OK: TableTheory CRUD against DynamoDB Local succeeded")
}

func TestInitScaffoldPinsRuntimeVersion(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "app")
	err := run([]string{"init", "--lang", "go", "--dir", dir, "--runtime-version", "2.3.4"}, ioDiscard{}, ioDiscard{})
	require.NoError(t, err)
	require.Contains(t, readFile(t, filepath.Join(dir, "go.mod")), "github.com/theory-cloud/tabletheory/v2 v2.3.4")
}

func TestInitScaffoldPinsV3GoRuntimePath(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "app")
	err := run([]string{"init", "--lang", "go", "--dir", dir, "--runtime-version", "3.0.0"}, ioDiscard{}, ioDiscard{})
	require.NoError(t, err)
	require.Contains(t, readFile(t, filepath.Join(dir, "go.mod")), "github.com/theory-cloud/tabletheory/v3 v3.0.0")
	require.Contains(t, readFile(t, filepath.Join(dir, "main.go")), `"github.com/theory-cloud/tabletheory/v3/pkg/session"`)
}

func TestInitScaffoldPinsV1GoRuntimePath(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "app")
	err := run([]string{"init", "--lang", "go", "--dir", dir, "--runtime-version", "1.10.1"}, ioDiscard{}, ioDiscard{})
	require.NoError(t, err)
	require.Contains(t, readFile(t, filepath.Join(dir, "go.mod")), "github.com/theory-cloud/tabletheory v1.10.1")
	require.Contains(t, readFile(t, filepath.Join(dir, "main.go")), `"github.com/theory-cloud/tabletheory/pkg/session"`)
}

func TestInitScaffoldErrors(t *testing.T) {
	t.Parallel()

	err := run([]string{"init"}, ioDiscard{}, ioDiscard{})
	require.ErrorContains(t, err, "init requires --lang")

	err = run([]string{"init", "--lang", "ruby", "--dir", filepath.Join(t.TempDir(), "app")}, ioDiscard{}, ioDiscard{})
	require.ErrorContains(t, err, "unsupported --lang")

	dir := filepath.Join(t.TempDir(), "app")
	require.NoError(t, run([]string{"init", "--lang", "go", "--dir", dir}, ioDiscard{}, ioDiscard{}))
	err = run([]string{"init", "--lang", "go", "--dir", dir}, ioDiscard{}, ioDiscard{})
	require.ErrorContains(t, err, "is not empty")
	require.NoError(t, run([]string{"init", "--lang", "go", "--dir", dir, "--force"}, ioDiscard{}, ioDiscard{}))
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path) // #nosec G304 -- tests read their own generated output.
	require.NoError(t, err)
	return string(data)
}
