#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${repo_root}"

tmp="$(mktemp -d)"
cleanup() { rm -rf "${tmp}"; }
trap cleanup EXIT

# Explicit selection and marshaling use DMS IsEmpty. Duplicate entries are
# distinct known call sites with identical source text.
cat >"${tmp}/expected" <<'EOF'
internal/expr/converter.go|IsEmpty|return reflectutil.IsEmpty(v)
pkg/marshal/marshaler.go|IsEmpty|if fieldMeta.OmitEmpty && reflectutil.IsEmpty(v) {
pkg/marshal/marshaler.go|IsEmpty|if fieldMeta.OmitEmpty && reflectutil.IsEmpty(v) {
pkg/marshal/marshaler.go|IsEmpty|if hasOmitEmpty && reflectutil.IsEmpty(fieldValue) {
pkg/marshal/safe_marshaler.go|IsEmpty|if fieldMeta != nil && fieldMeta.omitEmpty && reflectutil.IsEmpty(v) {
pkg/marshal/safe_marshaler.go|IsEmpty|if hasOmitEmpty && reflectutil.IsEmpty(fieldValue) {
pkg/query/query_execution.go|IsEmpty|if fieldMeta.OmitEmpty && reflectutil.IsEmpty(fieldValue) {
pkg/query/query_execution.go|IsEmpty|if fieldcodec.HasModifier(tag, "omitempty") && reflectutil.IsEmpty(fieldValue) {
pkg/query/query_marshal.go|IsEmpty|if fieldMeta.OmitEmpty && reflectutil.IsEmpty(fieldValue) {
pkg/query/query_marshal.go|IsEmpty|if fieldcodec.HasModifier(tag, "omitempty") && reflectutil.IsEmpty(fieldValue) {
pkg/query/tagged_encode_helpers.go|IsEmpty|if fieldcodec.HasModifier(tag, "omitempty") && reflectutil.IsEmpty(fieldValue) {
pkg/transaction/builder.go|IsEmpty|if fieldMeta.OmitEmpty && reflectutil.IsEmpty(fieldValue) {
pkg/transaction/transaction.go|IsEmpty|if fieldValue.IsValid() && !reflectutil.IsEmpty(fieldValue) {
pkg/transaction/transaction.go|IsEmpty|if fieldMeta.OmitEmpty && reflectutil.IsEmpty(fieldValue) {
EOF

# Only implicit whole-model sparse selection uses the compatibility predicate.
cat >>"${tmp}/expected" <<'EOF'
pkg/query/query_execution.go|IsSparseUpdateEmpty|if fieldMeta.OmitEmpty && reflectutil.IsSparseUpdateEmpty(fieldValue) {
pkg/query/query_execution.go|IsSparseUpdateEmpty|if fieldcodec.HasModifier(tag, "omitempty") && reflectutil.IsSparseUpdateEmpty(fieldValue) {
pkg/query/tagged_encode_helpers.go|IsSparseUpdateEmpty|if fieldcodec.HasModifier(tag, "omitempty") && reflectutil.IsSparseUpdateEmpty(fieldValue) {
pkg/transaction/transaction.go|IsSparseUpdateEmpty|if !fieldValue.IsValid() || (fieldMeta.OmitEmpty && reflectutil.IsSparseUpdateEmpty(fieldValue)) {
pkg/transaction/write_policy.go|IsSparseUpdateEmpty|if !fieldValue.IsValid() || (fieldMeta.OmitEmpty && reflectutil.IsSparseUpdateEmpty(fieldValue)) {
EOF

rg --no-heading --line-number \
  --glob '*.go' \
  --glob '!*_test.go' \
  --glob '!internal/reflectutil/empty.go' \
  'reflectutil\.Is(SparseUpdate)?Empty\(' internal pkg |
  while IFS=: read -r file _ source; do
    predicate="$(grep -oE 'Is(SparseUpdate)?Empty' <<<"${source}" | head -n 1)"
    normalized="$(sed -E 's/^[[:space:]]+//; s/[[:space:]]+/ /g; s/[[:space:]]+$//' <<<"${source}")"
    printf '%s|%s|%s\n' "${file}" "${predicate}" "${normalized}"
  done >"${tmp}/actual"

sort "${tmp}/expected" -o "${tmp}/expected"
sort "${tmp}/actual" -o "${tmp}/actual"

if ! diff -u "${tmp}/expected" "${tmp}/actual"; then
  echo "empty-predicate-call-sites: FAIL update the classified allowlist only after reviewing the call-site semantics" >&2
  exit 1
fi

echo "empty-predicate-call-sites: PASS"
