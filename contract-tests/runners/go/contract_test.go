package contracttests

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory-contract-tests/runners/go/internal/driver"
	"github.com/theory-cloud/tabletheory-contract-tests/runners/go/internal/runner"
	"github.com/theory-cloud/tabletheory-contract-tests/runners/go/internal/scenario"
	"github.com/theory-cloud/tabletheory-contract-tests/runners/go/internal/spec"
)

func TestContract_P0(t *testing.T) {
	t.Helper()
	runContractScenarios(t, filepath.Join("contract-tests", "scenarios", "p0"))
}

func TestContract_P1(t *testing.T) {
	t.Helper()
	runContractScenarios(t, filepath.Join("contract-tests", "scenarios", "p1"))
}

func TestContract_Interop(t *testing.T) {
	t.Helper()
	if os.Getenv("CONTRACT_RUN_INTEROP") != "1" {
		t.Skip("set CONTRACT_RUN_INTEROP=1 to run cross-runtime interop scenarios")
	}
	runContractScenarios(t, filepath.Join("contract-tests", "scenarios", "interop"))
}

func runContractScenarios(t *testing.T, scenarioRelDir string) {
	t.Helper()

	ctx := context.Background()

	drv, err := driver.NewTheorydbDriver()
	require.NoError(t, err)

	r, err := runner.New(drv)
	require.NoError(t, err)

	if err := r.Ping(ctx); err != nil {
		t.Skipf("DynamoDB Local not reachable (set DYNAMODB_ENDPOINT or start docker compose): %v", err)
	}

	root, err := runner.RepoRootFromModuleDir()
	require.NoError(t, err)

	models, err := spec.LoadModelsDir(filepath.Join(root, "contract-tests", "dms", "v0.1", "models"))
	require.NoError(t, err)

	scenarioDir := filepath.Join(root, scenarioRelDir)
	files, err := filepath.Glob(filepath.Join(scenarioDir, "*.yml"))
	require.NoError(t, err)
	require.NotEmpty(t, files)

	for _, path := range files {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			s, err := scenario.LoadFile(path)
			require.NoError(t, err)

			if missing := scenario.MissingCapabilities(s.RequiresCapabilities, drv.Capabilities()); len(missing) > 0 {
				t.Skipf("scenario requires unsupported capabilities: %v", missing)
			}

			r.RunScenario(t, ctx, s, models)
		})
	}
}
