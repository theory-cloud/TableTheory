package main

import (
	"bytes"
	"embed"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
)

//go:embed templates
var templatesFS embed.FS

// defaultRuntimeVersion is the TableTheory release the generated scaffold pins.
// It can be overridden with --runtime-version.
var defaultRuntimeVersion = "3.0.0"

const goModuleBase = "github.com/theory-cloud/tabletheory"

type initData struct {
	Module           string
	Lang             string
	GoModulePath     string
	GoModuleVersion  string
	TsPackageVersion string
	PyPackageVersion string
}

func initScaffold(args []string, stdout io.Writer, stderr io.Writer) error {
	cmd := flag.NewFlagSet("init", flag.ContinueOnError)
	cmd.SetOutput(stderr)
	lang := cmd.String("lang", "", "scaffold language: go, ts, or py")
	dir := cmd.String("dir", "tabletheory-quickstart", "output directory for the scaffold")
	module := cmd.String("module", "", "module/package name (defaults to the directory name)")
	runtimeVersion := cmd.String("runtime-version", defaultRuntimeVersion, "TableTheory release version to pin")
	force := cmd.Bool("force", false, "write into a non-empty directory")
	if err := cmd.Parse(args); err != nil {
		return err
	}

	langKey, err := normalizeInitLang(*lang)
	if err != nil {
		return err
	}
	if cmd.NArg() != 0 {
		return usageError("init takes no positional arguments; use --dir")
	}

	targetDir := *dir
	if ensureErr := ensureWritableDir(targetDir, *force); ensureErr != nil {
		return ensureErr
	}

	normalizedRuntimeVersion := strings.TrimPrefix(*runtimeVersion, "v")
	data := initData{
		Module:           resolveModuleName(*module, targetDir),
		Lang:             langKey,
		GoModulePath:     goModulePathForRuntimeVersion(normalizedRuntimeVersion),
		GoModuleVersion:  "v" + normalizedRuntimeVersion,
		TsPackageVersion: normalizedRuntimeVersion,
		PyPackageVersion: normalizedRuntimeVersion,
	}

	written, err := renderTemplateTree(targetDir, "shared", data)
	if err != nil {
		return err
	}
	langWritten, err := renderTemplateTree(targetDir, langKey, data)
	if err != nil {
		return err
	}
	written = append(written, langWritten...)

	var msg strings.Builder
	fmt.Fprintf(&msg, "Scaffolded TableTheory %s quickstart in %s:\n", langKey, targetDir)
	for _, w := range written {
		fmt.Fprintf(&msg, "  %s\n", w)
	}
	fmt.Fprintf(&msg, "\nNext:\n")
	fmt.Fprintf(&msg, "  cd %s\n", targetDir)
	msg.WriteString(initNextSteps(langKey))
	_, err = io.WriteString(stdout, msg.String())
	return err
}

func goModulePathForRuntimeVersion(version string) string {
	majorPart := version
	if dot := strings.Index(majorPart, "."); dot >= 0 {
		majorPart = majorPart[:dot]
	}
	major, err := strconv.Atoi(majorPart)
	if err != nil || major < 2 {
		return goModuleBase
	}
	return fmt.Sprintf("%s/v%d", goModuleBase, major)
}

func normalizeInitLang(lang string) (string, error) {
	switch lang {
	case "go":
		return "go", nil
	case "ts", "typescript":
		return "ts", nil
	case "py", "python":
		return "py", nil
	case "":
		return "", usageError("init requires --lang <go|ts|py>")
	default:
		return "", usageError(fmt.Sprintf("unsupported --lang %q (want go, ts, or py)", lang))
	}
}

func ensureWritableDir(dir string, force bool) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return os.MkdirAll(dir, 0o750)
		}
		return err
	}
	if len(entries) > 0 && !force {
		return fmt.Errorf("directory %s is not empty (use --force to write anyway)", dir)
	}
	return nil
}

func resolveModuleName(module, dir string) string {
	if module != "" {
		return module
	}
	base := filepath.Base(filepath.Clean(dir))
	if base == "." || base == string(filepath.Separator) || base == "" {
		return "tabletheory-quickstart"
	}
	return base
}

func renderTemplateTree(targetDir, treeName string, data initData) ([]string, error) {
	root := "templates/" + treeName
	var written []string
	err := fs.WalkDir(templatesFS, root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		outRel := strings.TrimSuffix(rel, ".tmpl")
		outPath := filepath.Join(targetDir, outRel)
		if mkdirErr := os.MkdirAll(filepath.Dir(outPath), 0o750); mkdirErr != nil {
			return mkdirErr
		}
		rendered, renderErr := renderTemplateFile(path, data)
		if renderErr != nil {
			return renderErr
		}
		if writeErr := os.WriteFile(outPath, rendered, 0o600); writeErr != nil {
			return writeErr
		}
		written = append(written, outRel)
		return nil
	})
	return written, err
}

func renderTemplateFile(path string, data initData) ([]byte, error) {
	content, err := templatesFS.ReadFile(path)
	if err != nil {
		return nil, err
	}
	tmpl, err := template.New(filepath.Base(path)).Option("missingkey=error").Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("parse template %s: %w", path, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("render template %s: %w", path, err)
	}
	return buf.Bytes(), nil
}

func initNextSteps(lang string) string {
	switch lang {
	case "go":
		return "  make smoke   # start DynamoDB Local and run the CRUD program\n" +
			"  # AI artifacts: https://tabletheory.theorycloud.ai/llms.txt\n"
	case "ts":
		return "  npm install && npm run smoke\n" +
			"  # AI artifacts: https://tabletheory.theorycloud.ai/llms.txt\n"
	case "py":
		return "  python -m venv .venv && . .venv/bin/activate && pip install -r requirements.txt\n" +
			"  docker compose up -d && DYNAMODB_ENDPOINT=http://localhost:8000 python main.py\n" +
			"  # AI artifacts: https://tabletheory.theorycloud.ai/llms.txt\n"
	default:
		return ""
	}
}
