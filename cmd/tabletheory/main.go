package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/theory-cloud/tabletheory/v3/pkg/dms"
	"github.com/theory-cloud/tabletheory/v3/pkg/keycontract"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer, stderr io.Writer) error {
	if len(args) == 0 {
		return usageError("missing command")
	}

	switch args[0] {
	case "validate":
		return validate(args[1:], stdout, stderr)
	case "gen":
		return generate(args[1:], stdout, stderr)
	case "init":
		return initScaffold(args[1:], stdout, stderr)
	case "contract":
		return contract(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		return printUsage(stdout)
	default:
		return usageError(fmt.Sprintf("unknown command %q", args[0]))
	}
}

func validate(args []string, stdout io.Writer, stderr io.Writer) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usageError("validate requires exactly one DMS file")
	}

	path := fs.Arg(0)
	data, err := os.ReadFile(path) // #nosec G304 -- CLI intentionally reads the user-provided DMS path.
	if err != nil {
		return fmt.Errorf("%s: read DMS: %w", path, err)
	}
	doc, err := dms.ParseDocument(data)
	if err != nil {
		return formatDmsError(path, data, err)
	}
	_, err = fmt.Fprintf(stdout, "OK: %s is valid DMS v%s (%d model(s))\n", path, doc.DMSVersion, len(doc.Models))
	return err
}

func generate(args []string, stdout io.Writer, stderr io.Writer) error {
	fs := flag.NewFlagSet("gen", flag.ContinueOnError)
	fs.SetOutput(stderr)
	lang := fs.String("lang", "", "generation target: go, ts, or py")
	cdk := fs.Bool("cdk", false, "generate AWS CDK DynamoDB table constructs (TypeScript) instead of models")
	outPath := fs.String("out", "", "output file path (defaults to stdout)")
	packageName := fs.String("package", "models", "Go package name for --lang go")
	modelName := fs.String("model", "", "optional model name to generate")
	runtimeImport := fs.String("runtime-import", "", "TypeScript runtime import path for --lang ts")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *cdk {
		if *lang != "" && *lang != "cdk" {
			return usageError("gen --cdk cannot be combined with --lang")
		}
		*lang = "cdk"
	}
	if *lang == "" {
		return usageError("gen requires --lang <go|ts|py> or --cdk")
	}
	if fs.NArg() != 1 {
		return usageError("gen requires exactly one DMS file")
	}

	path := fs.Arg(0)
	data, err := os.ReadFile(path) // #nosec G304 -- CLI intentionally reads the user-provided DMS path.
	if err != nil {
		return fmt.Errorf("%s: read DMS: %w", path, err)
	}
	doc, err := dms.ParseDocument(data)
	if err != nil {
		return formatDmsError(path, data, err)
	}
	generated, err := dms.Generate(doc, dms.GenerateOptions{
		Lang:          *lang,
		PackageName:   *packageName,
		ModelName:     *modelName,
		RuntimeImport: *runtimeImport,
	})
	if err != nil {
		return fmt.Errorf("%s: generate %s: %w", path, *lang, err)
	}
	if *outPath == "" {
		_, err = stdout.Write(generated)
		return err
	}
	if err := os.MkdirAll(filepath.Dir(*outPath), 0o700); err != nil {
		return err
	}
	return os.WriteFile(*outPath, generated, 0o600)
}

func contract(args []string, stdout io.Writer, stderr io.Writer) error {
	if len(args) == 0 {
		return usageError("contract requires a subcommand")
	}
	switch args[0] {
	case "generate-ts":
		return generateContractTS(args[1:], stderr)
	case "help", "-h", "--help":
		_, err := fmt.Fprint(stdout, contractUsageText())
		return err
	default:
		return usageError(fmt.Sprintf("unknown contract subcommand %q", args[0]))
	}
}

func generateContractTS(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("contract generate-ts", flag.ContinueOnError)
	fs.SetOutput(stderr)
	contractPath := fs.String("contract", "", "path to tabletheory_model_contract v0.1 YAML/JSON")
	outPath := fs.String("out", "", "path for generated TypeScript helper module")
	runtimeImport := fs.String("runtime-import", "@theory-cloud/tabletheory-ts", "TypeScript import path for TableTheory key-contract runtime")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *contractPath == "" {
		return usageError("contract generate-ts requires --contract")
	}
	if *outPath == "" {
		return usageError("contract generate-ts requires --out")
	}

	contract, err := keycontract.LoadFile(*contractPath)
	if err != nil {
		return fmt.Errorf("load contract: %w", err)
	}
	module, err := keycontract.GenerateTypeScript(contract, keycontract.GenerateTypeScriptOptions{RuntimeImport: *runtimeImport})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(*outPath), 0o700); err != nil {
		return err
	}
	return os.WriteFile(*outPath, []byte(module), 0o600)
}

func formatDmsError(path string, data []byte, err error) error {
	line := extractLineNumber(err.Error())
	if line <= 0 {
		return fmt.Errorf("%s: DMS validation failed: %w\nHint: run `tabletheory validate <dms.yml>` after checking dms_version, models[], keys, and attributes[]", path, err)
	}
	context := lineContext(data, line)
	if context == "" {
		return fmt.Errorf("%s:%d: DMS validation failed: %w", path, line, err)
	}
	return fmt.Errorf("%s:%d: DMS validation failed: %w\n%s", path, line, err, context)
}

var linePattern = regexp.MustCompile(`(?i)line\s+(\d+)`)

func extractLineNumber(message string) int {
	match := linePattern.FindStringSubmatch(message)
	if len(match) != 2 {
		return 0
	}
	line, err := strconv.Atoi(match[1])
	if err != nil {
		return 0
	}
	return line
}

func lineContext(data []byte, line int) string {
	if line <= 0 {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	if line > len(lines) {
		return ""
	}
	return fmt.Sprintf("  %4d | %s", line, lines[line-1])
}

func usageError(message string) error {
	return fmt.Errorf("%s\n\n%s", message, usageText())
}

func printUsage(w io.Writer) error {
	_, err := fmt.Fprint(w, usageText())
	return err
}

func usageText() string {
	return `Usage:
  tabletheory validate <dms.yml>
  tabletheory gen --lang <go|ts|py> [--model <name>] [--out <file>] <dms.yml>
  tabletheory gen --cdk [--model <name>] [--out <file>] <dms.yml>
  tabletheory init --lang <go|ts|py> [--dir <path>] [--module <name>] [--force]
  tabletheory contract generate-ts --contract <file> --out <file> [--runtime-import <module>]
`
}

func contractUsageText() string {
	return `Usage:
  tabletheory contract generate-ts --contract <file> --out <file> [--runtime-import <module>]
`
}
