package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/theory-cloud/tabletheory/pkg/keycontract"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return usageError("missing command")
	}

	switch args[0] {
	case "generate-ts":
		return generateTS(args[1:])
	case "help", "-h", "--help":
		return printUsage(os.Stdout)
	default:
		return usageError(fmt.Sprintf("unknown command %q", args[0]))
	}
}

func generateTS(args []string) error {
	fs := flag.NewFlagSet("generate-ts", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	contractPath := fs.String("contract", "", "path to tabletheory_model_contract v0.1 YAML/JSON")
	outPath := fs.String("out", "", "path for generated TypeScript helper module")
	runtimeImport := fs.String("runtime-import", "@theory-cloud/tabletheory-ts", "TypeScript import path for TableTheory key-contract runtime")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *contractPath == "" {
		return usageError("generate-ts requires --contract")
	}
	if *outPath == "" {
		return usageError("generate-ts requires --out")
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

func usageError(message string) error {
	return fmt.Errorf("%s\n\n%s", message, usageText())
}

func printUsage(w io.Writer) error {
	_, err := fmt.Fprint(w, usageText())
	return err
}

func usageText() string {
	return `Usage:
  tabletheory-contract generate-ts --contract <file> --out <file> [--runtime-import <module>]
`
}
