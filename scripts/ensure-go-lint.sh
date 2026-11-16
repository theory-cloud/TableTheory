#!/usr/bin/env bash
set -euo pipefail

# Ensure golangci-lint is the version this repository pins.
#
# CI verifies an evidence record and installs no linter, so the pin lives in the
# Makefile (GOLANGCI_LINT_VERSION) and is enforced here, where lint actually
# runs: `make lint` calls this first, and the COM-2 guard reads the same pin.
# A preinstalled binary of another version is replaced rather than trusted.

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${repo_root}"

pin="$(awk -F'=[[:space:]]*' '/^GOLANGCI_LINT_VERSION[[:space:]]*\?=/ {print $2; exit}' Makefile)"
pin="${pin%"${pin##*[![:space:]]}"}"
if [[ ! "${pin}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "ensure-go-lint: FAIL (Makefile GOLANGCI_LINT_VERSION is not a vX.Y.Z pin: '${pin}')" >&2
  exit 1
fi

gobin="$(go env GOBIN)"
if [[ -z "${gobin}" ]]; then
  gobin="$(go env GOPATH)/bin"
fi
mkdir -p "${gobin}"
tool="${gobin}/golangci-lint"

installed=""
if [[ -x "${tool}" ]]; then
  installed="$(go version -m "${tool}" 2>/dev/null | awk '$1 == "mod" { print $3; exit }')"
fi

if [[ "${installed}" != "${pin}" ]]; then
  echo "ensure-go-lint: installing pinned golangci-lint ${pin} into ${gobin}" >&2
  GOBIN="${gobin}" go install "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@${pin}"
  installed="$(go version -m "${tool}" 2>/dev/null | awk '$1 == "mod" { print $3; exit }')"
fi

if [[ "${installed}" != "${pin}" ]]; then
  echo "ensure-go-lint: FAIL (installed ${installed:-none} != pinned ${pin})" >&2
  exit 1
fi

echo "ensure-go-lint: ok (${pin})"
