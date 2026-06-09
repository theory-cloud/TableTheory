#!/usr/bin/env bash
set -euo pipefail

fixture="contract-tests/key-contracts/v0.1/theorymcp-derived-keys.yml"
artifact="contract-tests/generated/key-contracts/v0.1/theorymcp-derived-keys.generated.ts"
runtime_import="../../../../ts/src/key-contract.js"

if [[ ! -f "${fixture}" ]]; then
  echo "generated-ts: FAIL (missing ${fixture})" >&2
  exit 1
fi
if [[ ! -d "ts/node_modules" ]]; then
  bash scripts/verify-typescript-deps.sh
fi

echo "generated-ts: generate"
go run ./cmd/tabletheory-contract generate-ts \
  --contract "${fixture}" \
  --out "${artifact}" \
  --runtime-import "${runtime_import}"

echo "generated-ts: drift"
git diff --exit-code -- "${artifact}"

echo "generated-ts: tsc"
(cd ts && npx tsc --noEmit \
  --target ES2022 \
  --module NodeNext \
  --moduleResolution NodeNext \
  --strict \
  --noUncheckedIndexedAccess \
  --exactOptionalPropertyTypes \
  --skipLibCheck \
  --types node \
  "../${artifact}")

tmpdir="$(mktemp -d)"
trap 'rm -rf "${tmpdir}"' EXIT

go_matrix="${tmpdir}/go-fixtures.json"
cat > "${tmpdir}/emit-go-fixtures.go" <<'EOF'
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/theory-cloud/tabletheory/pkg/keycontract"
)

type fixtureRow struct {
	Key     string `json:"key"`
	Fixture string `json:"fixture"`
	Expect  string `json:"expect"`
	Go      string `json:"go"`
}

func main() {
	if len(os.Args) != 2 {
		_, _ = fmt.Fprintln(os.Stderr, "usage: emit-go-fixtures <contract.yml>")
		os.Exit(2)
	}
	contract, err := keycontract.LoadFile(os.Args[1])
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	rows := make([]fixtureRow, 0)
	for _, key := range contract.DerivedKeys {
		for _, fixture := range key.Fixtures {
			got, err := keycontract.EvaluateDerivedKey(key, fixture.Input)
			if err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "derived key %s fixture %s: %v\n", key.Name, fixture.Name, err)
				os.Exit(1)
			}
			rows = append(rows, fixtureRow{
				Key:     key.Name,
				Fixture: fixture.Name,
				Expect:  fixture.Expect,
				Go:      got,
			})
		}
	}
	if err := json.NewEncoder(os.Stdout).Encode(rows); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
EOF

go run "${tmpdir}/emit-go-fixtures.go" "${fixture}" > "${go_matrix}"

echo "generated-ts: helper parity"
npx --prefix ts tsx scripts/verify-generated-ts-key-contract.ts "${artifact}" "${go_matrix}"
