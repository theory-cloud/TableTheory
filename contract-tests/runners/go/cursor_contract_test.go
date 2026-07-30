package contracttests

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory-contract-tests/runners/go/internal/runner"
	"github.com/theory-cloud/tabletheory/v3/pkg/query"
)

func TestCursor_Golden_v01_Corpus(t *testing.T) {
	t.Helper()

	root, err := runner.RepoRootFromModuleDir()
	require.NoError(t, err)

	goldenDir := filepath.Join(root, "contract-tests", "golden", "cursor")
	jsonPaths, err := filepath.Glob(filepath.Join(goldenDir, "cursor_v0.1_*.json"))
	require.NoError(t, err)
	require.NotEmpty(t, jsonPaths)

	for _, jsonPath := range jsonPaths {
		jsonPath := jsonPath
		name := strings.TrimSuffix(filepath.Base(jsonPath), ".json")
		t.Run(name, func(t *testing.T) {
			wantJSON := strings.TrimSpace(mustReadFile(t, jsonPath))
			wantCursor := strings.TrimSpace(mustReadFile(t, filepath.Join(goldenDir, name+".cursor")))

			var decodedFixture query.Cursor
			require.NoError(t, json.Unmarshal([]byte(wantJSON), &decodedFixture))
			lastKey, err := decodedFixture.ToAttributeValues()
			require.NoError(t, err)

			encoded, err := query.EncodeCursor(lastKey, decodedFixture.IndexName, decodedFixture.SortDirection)
			require.NoError(t, err)
			require.Equal(t, wantCursor, encoded)

			if wantCursor == "" {
				return
			}

			data, err := base64.URLEncoding.DecodeString(wantCursor)
			require.NoError(t, err)
			require.Equal(t, wantJSON, string(data))

			decoded, err := query.DecodeCursor(encoded)
			require.NoError(t, err)

			gotJSONBytes, err := json.Marshal(decoded)
			require.NoError(t, err)
			require.Equal(t, wantJSON, string(gotJSONBytes))
		})
	}
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}
