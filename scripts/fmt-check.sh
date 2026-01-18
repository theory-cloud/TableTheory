#!/usr/bin/env bash
set -euo pipefail

files="$(git ls-files '*.go')"
if [[ -z "${files}" ]]; then
  exit 0
fi

unformatted="$(gofmt -l ${files})"
if [[ -n "${unformatted}" ]]; then
  echo "gofmt is required on the following files:"
  echo "${unformatted}"
  exit 1
fi

echo "gofmt: clean"

