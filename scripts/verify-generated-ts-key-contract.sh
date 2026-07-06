#!/usr/bin/env bash
set -euo pipefail

fixtures=(
  "contract-tests/key-contracts/v0.1/theorymcp-derived-keys.yml|contract-tests/generated/key-contracts/v0.1/theorymcp-derived-keys.generated.ts|../../../../ts/src/key-contract.js"
  "contract-tests/key-contracts/v0.2/derived-key-transforms.yml|contract-tests/generated/key-contracts/v0.2/derived-key-transforms.generated.ts|../../../../ts/src/key-contract.js"
)

if [[ ! -d "ts/node_modules" ]]; then
  bash scripts/verify-typescript-deps.sh
fi

generated_artifacts=()
for entry in "${fixtures[@]}"; do
  IFS='|' read -r fixture artifact runtime_import <<<"${entry}"
  if [[ ! -f "${fixture}" ]]; then
    echo "generated-ts: FAIL (missing ${fixture})" >&2
    exit 1
  fi

  echo "generated-ts: generate ${fixture}"
  go run ./cmd/tabletheory-contract generate-ts \
    --contract "${fixture}" \
    --out "${artifact}" \
    --runtime-import "${runtime_import}"
  generated_artifacts+=("${artifact}")
done

echo "generated-ts: drift"
git diff --exit-code -- "${generated_artifacts[@]}"

echo "generated-ts: tsc"
for artifact in "${generated_artifacts[@]}"; do
  (cd ts && npx tsc --noEmit \
    --ignoreConfig \
    --target ES2022 \
    --module NodeNext \
    --moduleResolution NodeNext \
    --strict \
    --noUncheckedIndexedAccess \
    --exactOptionalPropertyTypes \
    --skipLibCheck \
    --types node \
    "../${artifact}")
done

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

for entry in "${fixtures[@]}"; do
  IFS='|' read -r fixture artifact _runtime_import <<<"${entry}"
  go_matrix="${tmpdir}/$(basename "${artifact}").go-fixtures.json"
  go run "${tmpdir}/emit-go-fixtures.go" "${fixture}" > "${go_matrix}"

  echo "generated-ts: helper parity ${artifact}"
  npx --prefix ts tsx scripts/verify-generated-ts-key-contract.ts "${artifact}" "${go_matrix}"
done
