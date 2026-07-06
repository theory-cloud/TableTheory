#!/usr/bin/env bash
# Dry-run proof for the tabletheory CLI release assets.
#
# Cross-compiles the exact CLI binary matrix the release/prerelease workflows
# attach (linux/mac, amd64/arm64), verifies each artifact is produced and
# non-empty, produces the checksums manifest, and smoke-runs the host binary.
# This publishes nothing and mutates no cloud state — it validates the
# asset-build path before it runs in a real release.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

GOTOOLCHAIN_PIN="$(awk '/^toolchain /{print $2; exit}' go.mod)"
export GOTOOLCHAIN="${GOTOOLCHAIN_PIN:-local}"

OUT="$(mktemp -d)"
cleanup() { rm -rf "$OUT"; }
trap cleanup EXIT

targets=(linux/amd64 linux/arm64 darwin/amd64 darwin/arm64)

echo "==> cross-compiling tabletheory CLI matrix"
for target in "${targets[@]}"; do
  os="${target%/*}"
  arch="${target#*/}"
  CGO_ENABLED=0 GOOS="${os}" GOARCH="${arch}" \
    go build -trimpath -ldflags="-s -w" \
    -o "${OUT}/tabletheory-${os}-${arch}" ./cmd/tabletheory
done

echo "==> verifying artifacts"
for target in "${targets[@]}"; do
  os="${target%/*}"
  arch="${target#*/}"
  artifact="${OUT}/tabletheory-${os}-${arch}"
  if [[ ! -s "$artifact" ]]; then
    echo "FAIL: missing or empty artifact ${artifact}" >&2
    exit 1
  fi
  echo "  ok  tabletheory-${os}-${arch} ($(wc -c <"$artifact") bytes)"
done

( cd "$OUT" && sha256sum tabletheory-linux-* tabletheory-darwin-* > tabletheory-SHA256SUMS.txt )
if [[ "$(grep -c . "${OUT}/tabletheory-SHA256SUMS.txt")" -ne "${#targets[@]}" ]]; then
  echo "FAIL: checksum manifest does not cover all ${#targets[@]} binaries" >&2
  exit 1
fi
echo "==> checksums:"
sed 's/^/  /' "${OUT}/tabletheory-SHA256SUMS.txt"

echo "==> smoke-running host binary"
host_os="$(go env GOOS)"
host_arch="$(go env GOARCH)"
host_bin="${OUT}/tabletheory-${host_os}-${host_arch}"
if [[ -x "$host_bin" ]]; then
  "$host_bin" help | grep -q "tabletheory validate" || {
    echo "FAIL: host binary help output unexpected" >&2
    exit 1
  }
  echo "  ok  ${host_os}/${host_arch} binary runs"
else
  echo "  note: host platform ${host_os}/${host_arch} not in the release matrix; skipping run"
fi

echo "PASS: tabletheory CLI release-asset matrix builds and verifies"
