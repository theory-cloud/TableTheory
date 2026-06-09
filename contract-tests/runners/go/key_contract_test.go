package contracttests

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory-contract-tests/runners/go/internal/runner"
	"github.com/theory-cloud/tabletheory/pkg/keycontract"
)

func TestKeyContract_TheoryMCPFixtures(t *testing.T) {
	t.Helper()

	root, err := runner.RepoRootFromModuleDir()
	require.NoError(t, err)

	contract, err := keycontract.LoadFile(filepath.Join(root, "contract-tests", "key-contracts", "v0.1", "theorymcp-derived-keys.yml"))
	require.NoError(t, err)
	require.NoError(t, keycontract.VerifyFixtures(contract))
}
