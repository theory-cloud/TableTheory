package marshal_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/theory-cloud/tabletheory/v6/pkg/marshal"
)

// conformanceTablePath is the one language-neutral table the Go, TypeScript and
// Python suites all consume. It lives beside the contract test fixtures so a
// single file states these semantics for every runtime.
const conformanceTablePath = "../../contract-tests/serialization/nil-empty-collections.yml"

type conformanceTable struct {
	WriterPaths     []string              `yaml:"writer_paths"`
	Runtimes        []string              `yaml:"runtimes"`
	Representations []string              `yaml:"representations"`
	Rows            []conformanceTableRow `yaml:"rows"`
	Version         int                   `yaml:"version"`
}

type conformanceTableRow struct {
	Expect    map[string]map[string]string `yaml:"expect"`
	Field     string                       `yaml:"field"`
	Container string                       `yaml:"container"`
	OmitEmpty bool                         `yaml:"omitempty"`
}

func loadConformanceTable(t *testing.T) conformanceTable {
	t.Helper()

	raw, err := os.ReadFile(filepath.FromSlash(conformanceTablePath))
	require.NoError(t, err, "the shared conformance table must exist at %s", conformanceTablePath)

	var table conformanceTable
	require.NoError(t, yaml.Unmarshal(raw, &table))
	require.Equal(t, 1, table.Version)
	require.NotEmpty(t, table.Rows)
	return table
}

// conformanceFieldReaders maps each logical field name in the shared table to
// the Go attribute it is stored as, and to a reader over a stored item.
func conformanceFieldReaders() map[string]func(map[string]types.AttributeValue) string {
	root := func(name string) func(map[string]types.AttributeValue) string {
		return func(item map[string]types.AttributeValue) string { return itemAttrType(item, name) }
	}
	nested := func(name string) func(map[string]types.AttributeValue) string {
		return func(item map[string]types.AttributeValue) string {
			return nestedAttrType(item, "nested", name)
		}
	}

	return map[string]func(map[string]types.AttributeValue) string{
		"nil_list":          root("nilList"),
		"empty_list":        root("emptyList"),
		"nil_map":           root("nilMap"),
		"empty_map":         root("emptyMap"),
		"nil_list_omit":     root("nilListOmit"),
		"empty_list_omit":   root("emptyListOmit"),
		"nil_map_omit":      root("nilMapOmit"),
		"empty_map_omit":    root("emptyMapOmit"),
		"false_bool":        root("falseBool"),
		"zero_number":       root("zeroInt"),
		"empty_set":         root("emptySet"),
		"nested_nil_list":   nested("nestedNilList"),
		"nested_empty_list": nested("nestedEmptyList"),
		"nested_nil_map":    nested("nestedNilMap"),
		"nested_empty_map":  nested("nestedEmptyMap"),
	}
}

// TestSharedConformanceTableGoColumn asserts the Go column of the shared table
// against both the captured writer payloads and the preflight answers. Every
// row must be mapped and every cell counted, so a dropped or renamed row fails
// the build instead of silently shrinking the pin.
func TestSharedConformanceTableGoColumn(t *testing.T) {
	table := loadConformanceTable(t)
	readers := conformanceFieldReaders()

	require.Len(t, table.Rows, len(readers),
		"the shared table must declare exactly the fields the Go runtime maps")
	require.Contains(t, table.Runtimes, "go")
	require.NotEmpty(t, table.WriterPaths)

	captured := map[string]map[string]types.AttributeValue{
		"put":            capturePut(t, newPreflightMatrix()),
		"transact_write": captureTransactWritePut(t, newPreflightMatrix()),
	}

	metadata := preflightMetadata(t, newPreflightMatrix())
	preflightPut, err := marshal.MarshalItemForWriterPath(newPreflightMatrix(), metadata, marshal.ItemWriterPathPut)
	require.NoError(t, err)
	preflightTransact, err := marshal.MarshalItemForWriterPath(
		newPreflightMatrix(), metadata, marshal.ItemWriterPathTransactWrite,
	)
	require.NoError(t, err)

	preflight := map[string]map[string]types.AttributeValue{
		"put":            preflightPut,
		"transact_write": preflightTransact,
	}

	seen := make(map[string]bool, len(table.Rows))
	capturedCells := 0
	preflightCells := 0

	for _, row := range table.Rows {
		t.Run(row.Field, func(t *testing.T) {
			read, ok := readers[row.Field]
			require.True(t, ok, "the shared table names a field the Go runtime does not map: %s", row.Field)
			require.False(t, seen[row.Field], "the shared table repeats the field %s", row.Field)
			seen[row.Field] = true

			for _, path := range table.WriterPaths {
				expectations, ok := row.Expect[path]
				require.True(t, ok, "row %s declares no expectation for writer path %s", row.Field, path)
				want, ok := expectations["go"]
				require.True(t, ok, "row %s declares no go expectation for writer path %s", row.Field, path)
				require.Contains(t, table.Representations, want)

				assert.Equal(t, want, read(captured[path]), "captured %s payload for %s", path, row.Field)
				assert.Equal(t, want, read(preflight[path]), "preflight %s answer for %s", path, row.Field)

				capturedCells++
				preflightCells++
			}
		})
	}

	derivedCells := len(table.Rows) * len(table.WriterPaths)
	assert.Equal(t, derivedCells, capturedCells, "every row/writer-path cell must be checked against a capture")
	assert.Equal(t, derivedCells, preflightCells, "every row/writer-path cell must be checked against the preflight")
}
