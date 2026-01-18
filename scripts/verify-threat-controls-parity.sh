#!/usr/bin/env bash
set -euo pipefail

# Deterministic parity check:
# - Every THR-* ID listed in the threat model must appear at least once in the controls matrix.

threat_model="docs/development/planning/theorydb-threat-model.md"
controls_matrix="docs/development/planning/theorydb-controls-matrix.md"

if [[ ! -f "${threat_model}" ]]; then
  echo "threat-parity: FAIL (missing ${threat_model})"
  exit 1
fi
if [[ ! -f "${controls_matrix}" ]]; then
  echo "threat-parity: FAIL (missing ${controls_matrix})"
  exit 1
fi

threats="$(rg -o 'THR-[0-9]+' "${threat_model}" | sort -u || true)"
if [[ -z "${threats}" ]]; then
  echo "threat-parity: FAIL (no THR-* IDs found in threat model; add stable threat IDs)"
  exit 1
fi

mapped_threats="$(rg -o 'THR-[0-9]+' "${controls_matrix}" | sort -u || true)"
if [[ -z "${mapped_threats}" ]]; then
  echo "threat-parity: FAIL (no THR-* IDs found in ${controls_matrix}; map threats to controls)"
  exit 1
fi

missing_list="$(comm -23 <(echo "${threats}") <(echo "${mapped_threats}") || true)"
unknown_list="$(comm -13 <(echo "${threats}") <(echo "${mapped_threats}") || true)"

missing=0
while IFS= read -r tid; do
  if [[ -z "${tid}" ]]; then
    continue
  fi
  echo "threat-parity: missing mapping for ${tid} in ${controls_matrix}"
  missing=$((missing + 1))
done <<< "${missing_list}"

unknown=0
while IFS= read -r tid; do
  if [[ -z "${tid}" ]]; then
    continue
  fi
  echo "threat-parity: unknown threat ${tid} referenced in ${controls_matrix} (not present in ${threat_model})"
  unknown=$((unknown + 1))
done <<< "${unknown_list}"

if [[ "${missing}" -ne 0 || "${unknown}" -ne 0 ]]; then
  echo "threat-parity: FAIL (${missing} missing, ${unknown} unknown)"
  exit 1
fi

echo "threat-parity: PASS"
