#!/usr/bin/env bash
set -euo pipefail

# TTSEC2-M4-T1 focused policy test for
# finding commit:1a30049242a48191b98f2a5c775cf282.
#
# The generated-key verifier (`scripts/verify-generated-ts-key-contract.mts`)
# awaits `main()` at the module top level. A top-level await suspends module
# evaluation, so any lexical binding `main` reads during that suspension must
# already be initialized. `lowerCamel` reads `splitNamePattern`, and it runs for
# otherwise-valid contracts whose derived key omits `helper` or whose input omits
# `ts_name`. When the constant is declared *below* the await it is still in the
# temporal dead zone, so the verifier fails closed with a ReferenceError.
#
# This test drives the real Go generator and the real verifier against
# contracts that omit those optional names (the hostile/implicit path) and a
# contract that supplies them (the ordinary path). It proves the assertion is
# non-vacuous by (a) requiring the fixture to genuinely omit the names,
# (b) requiring the generated artifact to carry the implicit lowerCamel helper,
# and (c) requiring the `.mts` constant to be declared before the top-level
# await.

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${REPO_ROOT}"

verifier="scripts/verify-generated-ts-key-contract.mts"

echo "generated-ts-tdz: static ordering guard"
await_line="$(grep -n 'await main()' "${verifier}" | head -1 | cut -d: -f1)"
pattern_line="$(grep -n 'const splitNamePattern' "${verifier}" | head -1 | cut -d: -f1)"
if [[ -z "${await_line}" || -z "${pattern_line}" ]]; then
  echo "generated-ts-tdz: FAIL (could not locate splitNamePattern or await main() in ${verifier})"
  exit 1
fi
if ((pattern_line >= await_line)); then
  echo "generated-ts-tdz: FAIL (splitNamePattern (line ${pattern_line}) must be declared before await main() (line ${await_line}))"
  exit 1
fi

if [[ ! -d "ts/node_modules" ]]; then
  bash scripts/verify-typescript-deps.sh
fi

tmpdir="$(mktemp -d)"
trap 'rm -rf "${tmpdir}"' EXIT

contract="${tmpdir}/implicit-names.yml"
artifact="${tmpdir}/implicit-names.generated.ts"
go_matrix="${tmpdir}/go-fixtures.json"

cat > "${contract}" <<'EOF'
tabletheory_model_contract_version: "0.1"
namespace: "tabletheory.tdz"
derived_keys:
  - name: "ImplicitHelperKey"
    join: "|"
    inputs:
      - { name: "tenant_name", type: "string" }
    segments:
      - name: "tenant"
        prefix: "tenant="
        value: { input: "tenant_name" }
    fixtures:
      - name: "implicit"
        input:
          tenant_name: "TheoryCloud"
        expect: "tenant=TheoryCloud"
  - name: "ExplicitHelperKey"
    helper: "explicitHelperKey"
    join: "|"
    inputs:
      - { name: "tenant_name", ts_name: "tenantName", type: "string" }
    segments:
      - name: "tenant"
        prefix: "tenant="
        value: { input: "tenant_name" }
    fixtures:
      - name: "explicit"
        input:
          tenant_name: "TheoryCloud"
        expect: "tenant=TheoryCloud"
EOF

echo "generated-ts-tdz: fixture omits helper/ts_name"
implicit_block="$(sed -n '/name: "ImplicitHelperKey"/,/name: "ExplicitHelperKey"/p' "${contract}")"
if [[ -z "${implicit_block}" ]]; then
  echo "generated-ts-tdz: FAIL (implicit fixture missing)"
  exit 1
fi
if grep -Eq '(^|[[:space:]])helper:|ts_name:' <<<"${implicit_block}"; then
  echo "generated-ts-tdz: FAIL (implicit fixture unexpectedly declares helper/ts_name)"
  exit 1
fi

runtime_import="$(python3 - "${REPO_ROOT}/ts/src/key-contract.js" <<'PY'
import sys
from pathlib import Path

print(Path(sys.argv[1]).resolve().as_uri())
PY
)"

echo "generated-ts-tdz: generate artifact via the real Go generator"
go run ./cmd/tabletheory-contract generate-ts \
  --contract "${contract}" \
  --out "${artifact}" \
  --runtime-import "${runtime_import}"

if ! grep -q 'export function implicithelperkey(' "${artifact}"; then
  echo "generated-ts-tdz: FAIL (implicit lowerCamel helper not generated; fixture does not exercise lowerCamel)"
  exit 1
fi

cat > "${tmpdir}/emit-go-fixtures.go" <<'EOF'
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/theory-cloud/tabletheory/v4/pkg/keycontract"
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

go run "${tmpdir}/emit-go-fixtures.go" "${contract}" > "${go_matrix}"

echo "generated-ts-tdz: helper parity (implicit + explicit)"
output="$(npx --prefix ts tsx "${verifier}" "${artifact}" "${go_matrix}")"
printf '%s\n' "${output}"
if [[ "${output}" != *"generated-ts: PASS (2 fixtures)"* ]]; then
  echo "generated-ts-tdz: FAIL (unexpected verifier output)"
  exit 1
fi

echo "generated-ts-tdz: PASS"
