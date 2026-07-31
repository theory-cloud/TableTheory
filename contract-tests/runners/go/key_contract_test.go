package contracttests

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory-contract-tests/runners/go/internal/runner"
	"github.com/theory-cloud/tabletheory/v3/pkg/keycontract"
)

func TestKeyContract_TheoryMCPFixtures(t *testing.T) {
	t.Helper()

	root, err := runner.RepoRootFromModuleDir()
	require.NoError(t, err)

	files, err := filepath.Glob(filepath.Join(root, "contract-tests", "key-contracts", "v*", "*.yml"))
	require.NoError(t, err)
	require.NotEmpty(t, files)

	seen := map[string]bool{}
	for _, path := range files {
		contract, err := keycontract.LoadFile(path)
		require.NoError(t, err, path)
		require.NoError(t, keycontract.VerifyFixtures(contract), path)
		for _, name := range keycontract.DerivedKeyNames(contract) {
			seen[name] = true
		}
	}

	require.True(t, seen["CanonicalPolicyKey"])
	require.True(t, seen["CanonicalBindingKey"])
	require.True(t, seen["InterfaceScopeKey"])
	require.True(t, seen["LowercaseLookupKey"])
}
