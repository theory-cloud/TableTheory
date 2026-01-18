package contracttests

import (
	"context"
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

	scenarioDir := filepath.Join(root, "contract-tests", "scenarios", "p0")
	files, err := filepath.Glob(filepath.Join(scenarioDir, "*.yml"))
	require.NoError(t, err)
	require.NotEmpty(t, files)

	for _, path := range files {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			s, err := scenario.LoadFile(path)
			require.NoError(t, err)

			model, ok := models[s.Model]
			require.True(t, ok, "unknown model %s", s.Model)

			r.RunScenario(t, ctx, s, model)
		})
	}
}
