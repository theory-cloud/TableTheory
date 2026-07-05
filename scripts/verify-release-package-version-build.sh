#!/usr/bin/env bash
# Local, non-publishing proof that tag-derived release package versions are
# stamped into TypeScript and Python release assets.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${repo_root}"

versions=("9.9.9-rc.1" "9.9.9")
if [[ "$#" -gt 0 ]]; then
  versions=()
  while [[ "$#" -gt 0 ]]; do
    case "$1" in
      --version)
        if [[ "$#" -lt 2 ]]; then
          echo "release-package-version-build: FAIL (--version requires a value)" >&2
          exit 2
        fi
        versions+=("$2")
        shift 2
        ;;
      -h|--help)
        cat <<'USAGE'
Usage: bash scripts/verify-release-package-version-build.sh [--version X.Y.Z[-rc.N] ...]

Builds local TypeScript and Python package assets with synthetic tag-derived
versions, verifies the packed metadata, and restores tracked source version
files before exiting. It never pushes, tags, publishes, uploads, edits releases,
or changes GitHub state.
USAGE
        exit 0
        ;;
      *)
        echo "release-package-version-build: FAIL (unknown argument: $1)" >&2
        exit 2
        ;;
    esac
  done
fi

if [[ "${#versions[@]}" -eq 0 ]]; then
  echo "release-package-version-build: FAIL (no versions requested)" >&2
  exit 2
fi

backup_dir="$(mktemp -d)"
assets_dir="$(mktemp -d)"
cleanup() {
  cp "${backup_dir}/ts-package.json" ts/package.json
  cp "${backup_dir}/ts-package-lock.json" ts/package-lock.json
  cp "${backup_dir}/py-version.json" py/src/theorydb_py/version.json
  rm -rf "${backup_dir}" "${assets_dir}"
}
trap cleanup EXIT

cp ts/package.json "${backup_dir}/ts-package.json"
cp ts/package-lock.json "${backup_dir}/ts-package-lock.json"
cp py/src/theorydb_py/version.json "${backup_dir}/py-version.json"

for version in "${versions[@]}"; do
  echo "==> building release package assets for ${version}"
  rm -rf "${assets_dir:?}"/*

  python3 scripts/prepare-release-package-versions.py --version "${version}"

  pushd ts >/dev/null
  npm run build
  npm pack --pack-destination "${assets_dir}"
  popd >/dev/null

  pushd py >/dev/null
  uv run python -m build --outdir "${assets_dir}"
  popd >/dev/null

  python3 scripts/verify-release-package-version-assets.py \
    --version "${version}" \
    --assets-dir "${assets_dir}"
done

echo "release-package-version-build: PASS (${#versions[@]} version(s))"
