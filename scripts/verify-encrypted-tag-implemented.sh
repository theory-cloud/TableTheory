#!/usr/bin/env bash
set -euo pipefail

# High-risk domain rule: security-affordance tags must be enforced in production code, not metadata-only.
#
# This repo currently parses `theorydb:"encrypted"` into model metadata. This verifier ensures the tag also has
# real semantics (write-time encryption + read-time decryption) using a provided KMS Key ARN.

roadmap="docs/development/planning/theorydb-encryption-tag-roadmap.md"

if [[ ! -f "${roadmap}" ]]; then
  echo "encrypted-tag: FAIL (missing roadmap: ${roadmap})"
  exit 1
fi

failures=0

# 1) The tag must be referenced somewhere beyond the tag parser lists.
set +e
tag_usage="$(rg -n --no-heading \
  --glob '*.go' \
  --glob '!**/*_test.go' \
  --glob '!examples/**' \
  --glob '!tests/**' \
  --glob '!scripts/**' \
  --glob '!pkg/model/registry.go' \
  --glob '!internal/expr/converter.go' \
  '\bencrypted\b' .)"
tag_status=$?
set -e
if [[ "${tag_status}" -gt 1 ]]; then
  echo "encrypted-tag: FAIL (rg error searching for tag usage: exit ${tag_status})"
  exit 1
fi
if [[ "${tag_status}" -ne 0 ]]; then
  echo "encrypted-tag: missing semantic usage of 'encrypted' outside tag parsing"
  failures=$((failures + 1))
fi

# 2) Implementation should use KMS (GenerateDataKey/Decrypt) and local authenticated encryption (AES-GCM).
set +e
kms_usage="$(rg -n --no-heading \
  --glob '*.go' \
  --glob '!**/*_test.go' \
  --glob '!examples/**' \
  --glob '!tests/**' \
  --glob '!scripts/**' \
  '(service/kms|GenerateDataKey|Decrypt)' .)"
kms_status=$?
set -e
if [[ "${kms_status}" -gt 1 ]]; then
  echo "encrypted-tag: FAIL (rg error searching for KMS usage: exit ${kms_status})"
  exit 1
fi
if [[ "${kms_status}" -ne 0 ]]; then
  echo "encrypted-tag: missing KMS integration signals (expected GenerateDataKey/Decrypt usage)"
  failures=$((failures + 1))
fi

set +e
crypto_usage="$(rg -n --no-heading \
  --glob '*.go' \
  --glob '!**/*_test.go' \
  --glob '!examples/**' \
  --glob '!tests/**' \
  --glob '!scripts/**' \
  '(crypto/aes|crypto/cipher|aes\.NewCipher|cipher\.NewGCM)' .)"
crypto_status=$?
set -e
if [[ "${crypto_status}" -gt 1 ]]; then
  echo "encrypted-tag: FAIL (rg error searching for crypto usage: exit ${crypto_status})"
  exit 1
fi
if [[ "${crypto_status}" -ne 0 ]]; then
  echo "encrypted-tag: missing authenticated encryption signals (expected AES-GCM implementation)"
  failures=$((failures + 1))
fi

if [[ "${failures}" -ne 0 ]]; then
  echo "encrypted-tag: FAIL (${failures} issue(s))"
  echo "encrypted-tag: implement enforced semantics per: ${roadmap}"
  exit 1
fi

echo "encrypted-tag: PASS"
