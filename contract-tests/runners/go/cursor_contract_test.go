package contracttests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory-contract-tests/runners/go/internal/runner"
	"github.com/theory-cloud/tabletheory/pkg/query"
)

func TestCursor_Golden_v01_Basic(t *testing.T) {
	t.Helper()

	root, err := runner.RepoRootFromModuleDir()
	require.NoError(t, err)

	wantCursor := strings.TrimSpace(mustReadFile(t, filepath.Join(root, "contract-tests", "golden", "cursor", "cursor_v0.1_basic.cursor")))
	wantJSON := strings.TrimSpace(mustReadFile(t, filepath.Join(root, "contract-tests", "golden", "cursor", "cursor_v0.1_basic.json")))

	lastKey := map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: "USER#1"},
		"SK": &types.AttributeValueMemberS{Value: "PROFILE"},
	}

	encoded, err := query.EncodeCursor(lastKey, "gsi-email", "ASC")
	require.NoError(t, err)
	require.Equal(t, wantCursor, encoded)

	decoded, err := query.DecodeCursor(encoded)
	require.NoError(t, err)

	gotJSONBytes, err := json.Marshal(decoded)
	require.NoError(t, err)
	require.Equal(t, wantJSON, string(gotJSONBytes))
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}
