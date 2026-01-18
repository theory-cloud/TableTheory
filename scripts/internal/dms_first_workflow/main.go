package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/theory-cloud/tabletheory/pkg/dms"
	"github.com/theory-cloud/tabletheory/pkg/model"

	demogo "github.com/theory-cloud/tabletheory/examples/cdk-multilang/lambdas/go/demo"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "dms-first-workflow: FAIL: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("dms-first-workflow: PASS")
}

func run() error {
	if err := verifyContractFixtures(); err != nil {
		return err
	}
	return verifyCdkMultilangDemo()
}

func verifyContractFixtures() error {
	modelNames, err := collectContractModelNames()
	if err != nil {
		return err
	}
	return verifyP0ScenarioModels(modelNames)
}

func collectContractModelNames() (map[string]struct{}, error) {
	modelsDir := filepath.Join("contract-tests", "dms", "v0.1", "models")
	entries, err := os.ReadDir(modelsDir)
	if err != nil {
		return nil, fmt.Errorf("read contract models dir: %w", err)
	}

	modelsFS := os.DirFS(modelsDir)
	modelNames := make(map[string]struct{})
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !isYAMLFile(entry.Name()) {
			continue
		}
		path := filepath.Join(modelsDir, entry.Name())
		data, err := fs.ReadFile(modelsFS, entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		doc, err := dms.ParseDocument(data)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		for _, m := range doc.Models {
			if m.Name != "" {
				modelNames[m.Name] = struct{}{}
			}
		}
	}
	if len(modelNames) == 0 {
		return nil, fmt.Errorf("no contract DMS models found under %s", modelsDir)
	}
	return modelNames, nil
}

func verifyP0ScenarioModels(modelNames map[string]struct{}) error {
	// Verify scenarios reference known models (cheap drift check).
	p0Dir := filepath.Join("contract-tests", "scenarios", "p0")
	entries, err := os.ReadDir(p0Dir)
	if err != nil {
		return fmt.Errorf("read p0 scenarios dir: %w", err)
	}

	p0FS := os.DirFS(p0Dir)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !isYAMLFile(entry.Name()) {
			continue
		}
		path := filepath.Join(p0Dir, entry.Name())
		data, err := fs.ReadFile(p0FS, entry.Name())
		if err != nil {
			return fmt.Errorf("read scenario %s: %w", path, err)
		}

		var s struct {
			Model string `yaml:"model"`
		}
		if err := yaml.Unmarshal(data, &s); err != nil {
			return fmt.Errorf("parse scenario %s: %w", path, err)
		}
		if s.Model == "" {
			return fmt.Errorf("scenario %s missing model", path)
		}
		if _, ok := modelNames[s.Model]; !ok {
			return fmt.Errorf("scenario %s references unknown model %q", path, s.Model)
		}
	}

	return nil
}

func verifyCdkMultilangDemo() error {
	dmsDir := filepath.Join("examples", "cdk-multilang", "dms")
	dmsFile := "demo.yml"
	dmsPath := filepath.Join(dmsDir, dmsFile)
	data, err := fs.ReadFile(os.DirFS(dmsDir), dmsFile)
	if err != nil {
		return fmt.Errorf("read %s: %w", dmsPath, err)
	}
	doc, err := dms.ParseDocument(data)
	if err != nil {
		return fmt.Errorf("parse %s: %w", dmsPath, err)
	}
	want, ok := dms.FindModel(doc, "DemoItem")
	if !ok {
		return fmt.Errorf("%s missing model DemoItem", dmsPath)
	}

	reg := model.NewRegistry()
	if err = reg.Register(demogo.DemoItem{}); err != nil {
		return fmt.Errorf("register go DemoItem: %w", err)
	}
	meta, err := reg.GetMetadata(demogo.DemoItem{})
	if err != nil {
		return fmt.Errorf("get go DemoItem metadata: %w", err)
	}
	got, err := dms.FromMetadata(meta)
	if err != nil {
		return fmt.Errorf("convert go DemoItem to DMS: %w", err)
	}

	if err := dms.AssertModelsEquivalent(got, *want, dms.CompareOptions{IgnoreTableName: true}); err != nil {
		return fmt.Errorf("cdk-multilang go DemoItem does not match DMS: %w", err)
	}
	return nil
}

func isYAMLFile(name string) bool {
	switch filepath.Ext(name) {
	case ".yml", ".yaml":
		return true
	default:
		return false
	}
}
