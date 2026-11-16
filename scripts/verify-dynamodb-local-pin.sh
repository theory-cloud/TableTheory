#!/usr/bin/env bash
set -euo pipefail

# COM-7: DynamoDB Local is pinned by image digest everywhere a gate, a floor, a
# checked-in local profile or a scaffold can start it, and a local runner
# publishes it on loopback only.
#
# A tag is mutable: `amazon/dynamodb-local:3.1.0` names whatever the registry
# serves when the container starts, so a gate could execute a different binary
# than the one the recorded evidence was produced against. The digest is the
# content address; `:3.1.0@sha256:...` keeps the tag readable while the digest
# stays authoritative. Markdown is documentation rather than an executable
# reference and is not scanned.
#
# The scan covers every file that can declare or start the image: compose files,
# Makefiles, shell scripts, container templates and the checked-in profile JSON.
#
# Usage: scripts/verify-dynamodb-local-pin.sh [--root <dir>]
#
# The root defaults to the current directory and must look like this repository
# (go.mod present). A file may opt out of the image-reference scan only by
# declaring, in its own text, the marker
# `dynamodb-local-pin-guard: file-owns-image-fixtures`; the policy test proves
# the scan still rejects an unpinned reference, so the marker cannot make the
# guard vacuous.

root="$PWD"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --root)
      root="${2:-}"
      shift 2
      ;;
    --help|-h)
      sed -n '2,31p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *)
      echo "dynamodb-local: FAIL (unknown argument: $1)" >&2
      exit 2
      ;;
  esac
done

if [[ ! -d "${root}" ]]; then
  echo "dynamodb-local: FAIL (root ${root} is not a directory)" >&2
  exit 2
fi
if [[ ! -f "${root}/go.mod" ]]; then
  echo "dynamodb-local: FAIL (root ${root} has no go.mod; run this from the repository root)" >&2
  exit 2
fi

failures=0

set +e
scan_output="$(python3 - "${root}" "${BASH_SOURCE[0]}" <<'PY'
import os
import re
import sys
from pathlib import Path

root = Path(sys.argv[1]).resolve()
checker = Path(sys.argv[2]).resolve()

image = "amazon/dynamodb-local"
digest = re.compile(r"@sha256:[0-9a-f]{64}(?![0-9a-f])")
marker = "dynamodb-local-pin-guard: file-owns-image-fixtures"
scan_roots = [".", "scripts", "tests", "examples", "contract-tests", "cmd", ".github", "gov-infra"]
suffixes = {".sh", ".yml", ".yaml", ".json", ".tmpl", ".tpl"}
skip_dirs = {
    ".git",
    ".venv",
    ".tools",
    ".pytest_cache",
    ".mypy_cache",
    ".ruff_cache",
    "node_modules",
    "dist",
    "build",
    "coverage",
    "cdk.out",
}
skip_names = {"package-lock.json"}
max_bytes = 1 << 20

candidates: list[Path] = []
seen: set[Path] = set()
for relative in scan_roots:
    base = (root / relative).resolve()
    if not base.is_dir():
        continue
    for directory, subdirectories, filenames in os.walk(base):
        subdirectories[:] = sorted(name for name in subdirectories if name not in skip_dirs)
        for filename in sorted(filenames):
            path = Path(directory) / filename
            if path in seen or path == checker:
                continue
            if filename in skip_names:
                continue
            if filename != "Makefile" and path.suffix not in suffixes:
                continue
            try:
                if path.stat().st_size > max_bytes:
                    continue
                text = path.read_text(encoding="utf-8")
            except (OSError, UnicodeDecodeError):
                continue
            if marker in text:
                continue
            seen.add(path)
            candidates.append(path)

violations = 0
for path in sorted(candidates):
    relative = path.relative_to(root)
    for number, line in enumerate(path.read_text(encoding="utf-8").splitlines(), start=1):
        if image not in line:
            continue
        if digest.search(line):
            continue
        print(f"{relative}:{number}: image is not pinned by digest: {line.strip()}")
        violations += 1

print(f"scanned={len(candidates)} violations={violations}")
PY
)"
status=$?
set -e
if [[ "${status}" -ne 0 ]]; then
  echo "dynamodb-local: FAIL (image-reference scan did not complete)" >&2
  exit 2
fi

summary="$(printf '%s\n' "${scan_output}" | tail -n 1)"
offenders="$(printf '%s\n' "${scan_output}" | sed '$d' | grep -v '^$' || true)"
if [[ -n "${offenders}" ]]; then
  while IFS= read -r offender; do
    [[ -n "${offender}" ]] || continue
    echo "dynamodb-local: ${offender}"
  done <<< "${offenders}"
  echo "dynamodb-local: fix: reference the image as amazon/dynamodb-local:<tag>@sha256:<64 hex> so a moved tag cannot change what a gate executes" >&2
  failures=$((failures + 1))
fi

# DynamoDB Local enforces no authentication, so a local runner must publish its
# port on loopback only. Docker publishes on every host interface when the host
# IP is omitted (finding 3e93ce38).
run_local="examples/cdk-multilang/local/run-local.sh"
if [[ -f "${root}/${run_local}" ]]; then
  if ! grep -Fq -- '-p "127.0.0.1:${PORT}:8000"' "${root}/${run_local}"; then
    echo "dynamodb-local: ${run_local} must publish DynamoDB Local on loopback (127.0.0.1)" >&2
    failures=$((failures + 1))
  fi
  if grep -Fq -- '-p "${PORT}:8000"' "${root}/${run_local}"; then
    echo "dynamodb-local: ${run_local} publishes DynamoDB Local on all interfaces (missing loopback host)" >&2
    failures=$((failures + 1))
  fi
fi

if [[ "${failures}" -ne 0 ]]; then
  echo "dynamodb-local: FAIL (${failures} issue(s))" >&2
  exit 1
fi

echo "dynamodb-local: pinned by digest (${summary})"
