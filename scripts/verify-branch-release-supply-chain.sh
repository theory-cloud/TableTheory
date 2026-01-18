#!/usr/bin/env bash
set -euo pipefail

# Verifies required branch/release supply-chain artifacts exist and are wired for the expected flow:
# - `premain` -> prereleases
# - `main` -> stable releases
#
# This is a deterministic grep-based check (not a full YAML parser).

failures=0

required_files=(
  "docs/development/planning/theorydb-branch-release-policy.md"
  ".github/workflows/prerelease.yml"
  ".github/workflows/release.yml"
  "release-please-config.premain.json"
  "release-please-config.json"
  ".release-please-manifest.premain.json"
  ".release-please-manifest.json"
)

for f in "${required_files[@]}"; do
  if [[ ! -f "${f}" ]]; then
    echo "branch-release: missing ${f}"
    failures=$((failures + 1))
  fi
done

if [[ -f ".github/workflows/prerelease.yml" ]]; then
  grep -Eq 'branches:.*premain' ".github/workflows/prerelease.yml" || {
    echo "branch-release: prerelease workflow must target premain"
    failures=$((failures + 1))
  }
  grep -Eq 'googleapis/release-please-action@[0-9a-fA-F]{40}.*\bv4\b' ".github/workflows/prerelease.yml" || {
    echo "branch-release: prerelease workflow must pin release-please v4 by commit SHA"
    failures=$((failures + 1))
  }
  grep -Eq 'contents:\s*write' ".github/workflows/prerelease.yml" || {
    echo "branch-release: prerelease workflow must request contents: write"
    failures=$((failures + 1))
  }
  grep -Eq 'config-file:\s*release-please-config\.premain\.json' ".github/workflows/prerelease.yml" || {
    echo "branch-release: prerelease workflow must reference release-please-config.premain.json"
    failures=$((failures + 1))
  }
  grep -Eq 'manifest-file:\s*\.release-please-manifest\.premain\.json' ".github/workflows/prerelease.yml" || {
    echo "branch-release: prerelease workflow must reference .release-please-manifest.premain.json"
    failures=$((failures + 1))
  }

  # Ensure prereleases attach multi-language release artifacts.
  grep -Eq 'release_created' ".github/workflows/prerelease.yml" || {
    echo "branch-release: prerelease workflow must use release-please outputs (release_created)"
    failures=$((failures + 1))
  }
  grep -Eq 'pushd ts' ".github/workflows/prerelease.yml" || {
    echo "branch-release: prerelease workflow must package TypeScript from ts/ (pushd ts)"
    failures=$((failures + 1))
  }
  grep -Eq 'npm pack --pack-destination \.\./release-assets' ".github/workflows/prerelease.yml" || {
    echo "branch-release: prerelease workflow must attach TypeScript npm pack artifact"
    failures=$((failures + 1))
  }
  grep -Eq 'python -m build --outdir \.\./release-assets' ".github/workflows/prerelease.yml" || {
    echo "branch-release: prerelease workflow must attach Python wheel/sdist artifacts"
    failures=$((failures + 1))
  }
  grep -Eq 'gh release upload' ".github/workflows/prerelease.yml" || {
    echo "branch-release: prerelease workflow must upload release assets to GitHub release"
    failures=$((failures + 1))
  }
fi

if [[ -f ".github/workflows/release.yml" ]]; then
  grep -Eq 'branches:.*main' ".github/workflows/release.yml" || {
    echo "branch-release: release workflow must target main"
    failures=$((failures + 1))
  }
  grep -Eq 'googleapis/release-please-action@[0-9a-fA-F]{40}.*\bv4\b' ".github/workflows/release.yml" || {
    echo "branch-release: release workflow must pin release-please v4 by commit SHA"
    failures=$((failures + 1))
  }
  grep -Eq 'contents:\s*write' ".github/workflows/release.yml" || {
    echo "branch-release: release workflow must request contents: write"
    failures=$((failures + 1))
  }
  grep -Eq 'config-file:\s*release-please-config\.json' ".github/workflows/release.yml" || {
    echo "branch-release: release workflow must reference release-please-config.json"
    failures=$((failures + 1))
  }
  grep -Eq 'manifest-file:\s*\.release-please-manifest\.json' ".github/workflows/release.yml" || {
    echo "branch-release: release workflow must reference .release-please-manifest.json"
    failures=$((failures + 1))
  }

  # Ensure stable releases attach multi-language release artifacts.
  grep -Eq 'release_created' ".github/workflows/release.yml" || {
    echo "branch-release: release workflow must use release-please outputs (release_created)"
    failures=$((failures + 1))
  }
  grep -Eq 'pushd ts' ".github/workflows/release.yml" || {
    echo "branch-release: release workflow must package TypeScript from ts/ (pushd ts)"
    failures=$((failures + 1))
  }
  grep -Eq 'npm pack --pack-destination \.\./release-assets' ".github/workflows/release.yml" || {
    echo "branch-release: release workflow must attach TypeScript npm pack artifact"
    failures=$((failures + 1))
  }
  grep -Eq 'python -m build --outdir \.\./release-assets' ".github/workflows/release.yml" || {
    echo "branch-release: release workflow must attach Python wheel/sdist artifacts"
    failures=$((failures + 1))
  }
  grep -Eq 'gh release upload' ".github/workflows/release.yml" || {
    echo "branch-release: release workflow must upload release assets to GitHub release"
    failures=$((failures + 1))
  }
fi

for wf in ".github/workflows/quality-gates.yml" ".github/workflows/codeql.yml"; do
  if [[ ! -f "${wf}" ]]; then
    continue
  fi
  grep -Eq 'branches:.*premain.*main|branches:.*main.*premain' "${wf}" || {
    echo "branch-release: ${wf}: expected triggers for both premain and main"
    failures=$((failures + 1))
  }
done

if [[ -f "ts/package.json" ]]; then
  grep -Eq '"private"\s*:\s*true' "ts/package.json" || {
    echo "branch-release: ts/package.json must remain private (no npm publishing)"
    failures=$((failures + 1))
  }

  for cfg in "release-please-config.premain.json" "release-please-config.json"; do
    if [[ ! -f "${cfg}" ]]; then
      continue
    fi
    grep -Eq '"extra-files"\s*:' "${cfg}" || {
      echo "branch-release: ${cfg}: must define extra-files for multi-language versioning"
      failures=$((failures + 1))
    }
    grep -Eq '"path"\s*:\s*"ts/package\.json"' "${cfg}" || {
      echo "branch-release: ${cfg}: must bump ts/package.json version"
      failures=$((failures + 1))
    }
    grep -Eq '"path"\s*:\s*"ts/package-lock\.json"' "${cfg}" || {
      echo "branch-release: ${cfg}: must bump ts/package-lock.json version"
      failures=$((failures + 1))
    }
    grep -Eq "\\$\\.packages\\[''\\]\\.version" "${cfg}" || {
      echo "branch-release: ${cfg}: must bump ts/package-lock.json packages[''].version"
      failures=$((failures + 1))
    }
  done
fi

if [[ -f "py/pyproject.toml" ]]; then
  for cfg in "release-please-config.premain.json" "release-please-config.json"; do
    if [[ ! -f "${cfg}" ]]; then
      continue
    fi
    grep -Eq '"extra-files"\s*:' "${cfg}" || {
      echo "branch-release: ${cfg}: must define extra-files for multi-language versioning"
      failures=$((failures + 1))
    }
    grep -Eq '"path"\s*:\s*"py/src/theorydb_py/version\.json"' "${cfg}" || {
      echo "branch-release: ${cfg}: must bump py/src/theorydb_py/version.json version"
      failures=$((failures + 1))
    }
  done
fi

if [[ "${failures}" -ne 0 ]]; then
  echo "branch-release: FAIL (${failures} issue(s))"
  exit 1
fi

echo "branch-release: PASS"
