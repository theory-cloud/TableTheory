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
  ".github/workflows/prerelease-pr.yml"
  ".github/workflows/release.yml"
  ".github/workflows/release-pr.yml"
  "release-please-config.premain.json"
  "release-please-config.json"
  ".release-please-manifest.premain.json"
  ".release-please-manifest.json"
  "scripts/sync-post-stable-release-baselines.sh"
  "docs/development/planning/theorydb-release-cycle-recovery-1.9.3.md"
)

for f in "${required_files[@]}"; do
  if [[ ! -f "${f}" ]]; then
    echo "branch-release: missing ${f}"
    failures=$((failures + 1))
  fi
done

if [[ -f "scripts/watch-release-cycle.sh" ]]; then
  grep -Fq "isDraft" "scripts/watch-release-cycle.sh" || {
    echo "branch-release: watch-release-cycle must check GitHub release draft state for --tag"
    failures=$((failures + 1))
  }
  grep -Fq "publishedAt" "scripts/watch-release-cycle.sh" || {
    echo "branch-release: watch-release-cycle must check GitHub release publishedAt for --tag"
    failures=$((failures + 1))
  }
  grep -Fq "git/ref/tags" "scripts/watch-release-cycle.sh" || {
    echo "branch-release: watch-release-cycle must check git tag refs for --tag"
    failures=$((failures + 1))
  }
  grep -Fq "untagged-" "scripts/watch-release-cycle.sh" || {
    echo "branch-release: watch-release-cycle must reject untagged draft release URLs for --tag"
    failures=$((failures + 1))
  }
  grep -Fq "targetCommitish" "scripts/watch-release-cycle.sh" || {
    echo "branch-release: watch-release-cycle must compare release targetCommitish with the tag ref"
    failures=$((failures + 1))
  }
fi

grep -Fq "tag_name was used by an immutable release" "docs/development/planning/theorydb-branch-release-policy.md" || {
  echo "branch-release: branch release policy must document immutable release tag-name reuse recovery"
  failures=$((failures + 1))
}
grep -Fq "one-time-use" "docs/development/planning/theorydb-branch-release-policy.md" || {
  echo "branch-release: branch release policy must name one-time-use immutable release versions"
  failures=$((failures + 1))
}
grep -Fq "Do not manually recreate tags" "AGENTS.md" || {
  echo "branch-release: AGENTS.md must prohibit manual tag recreation during release recovery"
  failures=$((failures + 1))
}
grep -Fq "tag_name was used by an immutable release" \
  "docs/development/planning/templates/high-risk-branch-release-policy.template.md" || {
  echo "branch-release: high-risk branch policy template must include immutable release reuse recovery"
  failures=$((failures + 1))
}
grep -Fq "Release-As:" "docs/development/planning/templates/high-risk-branch-release-policy.template.md" || {
  echo "branch-release: high-risk branch policy template must document release-please Release-As lane recovery"
  failures=$((failures + 1))
}
grep -Fq "Release-As: 1.9.3-rc.1" "AGENTS.md" || {
  echo "branch-release: AGENTS.md must document the THE-1869 1.9.3 recovery footer"
  failures=$((failures + 1))
}
if [[ -f "docs/development/planning/theorydb-release-cycle-recovery-1.9.3.md" ]]; then
  recovery_doc="docs/development/planning/theorydb-release-cycle-recovery-1.9.3.md"
  grep -Fq "1.9.2" "${recovery_doc}" || {
    echo "branch-release: recovery doc must record the abandoned 1.9.2 lane"
    failures=$((failures + 1))
  }
  grep -Fq "v1.9.3-rc.1" "${recovery_doc}" || {
    echo "branch-release: recovery doc must record v1.9.3-rc.1 as the next RC"
    failures=$((failures + 1))
  }
  grep -Fq "v1.9.3" "${recovery_doc}" || {
    echo "branch-release: recovery doc must record v1.9.3 as the next stable release"
    failures=$((failures + 1))
  }
  grep -Fq "Release-As: 1.9.3-rc.1" "${recovery_doc}" || {
    echo "branch-release: recovery doc must record the release-please Release-As footer"
    failures=$((failures + 1))
  }
  grep -Fq "Do not hand-edit" "${recovery_doc}" && grep -Fq ".release-please-manifest*.json" "${recovery_doc}" || {
    echo "branch-release: recovery doc must forbid manual release manifest edits"
    failures=$((failures + 1))
  }
  grep -Fq "staging" "${recovery_doc}" && grep -Fq "premain" "${recovery_doc}" && grep -Fq "main" "${recovery_doc}" || {
    echo "branch-release: recovery doc must document the staging/premain/main path"
    failures=$((failures + 1))
  }
fi

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
  grep -Fq "scripts/verify-release-cycle-state.sh" ".github/workflows/prerelease.yml" || {
    echo "branch-release: prerelease workflow must verify release-cycle state before release-please"
    failures=$((failures + 1))
  }
  grep -Fq "scripts/verify-branch-version-sync.sh" ".github/workflows/prerelease.yml" || {
    echo "branch-release: prerelease workflow must verify branch version sync before release-please"
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
  grep -Fq "scripts/verify-release-cycle-state.sh" ".github/workflows/release.yml" || {
    echo "branch-release: release workflow must verify release-cycle state before release-please"
    failures=$((failures + 1))
  }
  grep -Fq "RELEASE_CYCLE_ALLOW_PENDING_STABLE_PROMOTION=true" ".github/workflows/release.yml" || {
    echo "branch-release: release workflow must explicitly verify pending stable promotion mode"
    failures=$((failures + 1))
  }
  grep -Fq "pending_stable_promotion" ".github/workflows/release.yml" || {
    echo "branch-release: release workflow must classify pending stable promotion"
    failures=$((failures + 1))
  }
  grep -Fq "stable release creation is skipped" ".github/workflows/release.yml" || {
    echo "branch-release: release workflow must skip stable release creation during pending promotion"
    failures=$((failures + 1))
  }
  grep -Fq "steps.cycle.outputs.pending_stable_promotion != 'true'" ".github/workflows/release.yml" || {
    echo "branch-release: release workflow must gate stable release-please on strict release-cycle state"
    failures=$((failures + 1))
  }
  grep -Eq 'missing tag_name output' ".github/workflows/release.yml" || {
    echo "branch-release: release workflow must fail asset/publish steps when tag_name is missing"
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
  grep -Fq "scripts/sync-post-stable-release-baselines.sh" ".github/workflows/release.yml" || {
    echo "branch-release: release workflow must sync post-stable baselines"
    failures=$((failures + 1))
  }
  grep -Fq 'SYNC_RELEASE_BASELINE_PUSH: "true"' ".github/workflows/release.yml" || {
    echo "branch-release: release workflow must enable post-stable baseline pushes"
    failures=$((failures + 1))
  }
  grep -Fq 'SYNC_RELEASE_BASELINE_TARGETS: "premain staging"' ".github/workflows/release.yml" || {
    echo "branch-release: release workflow must sync both premain and staging baselines"
    failures=$((failures + 1))
  }
fi

if [[ -f "scripts/sync-post-stable-release-baselines.sh" ]]; then
  grep -Fq ".release-please-manifest.premain.json" "scripts/sync-post-stable-release-baselines.sh" || {
    echo "branch-release: post-stable sync must reset the premain prerelease manifest"
    failures=$((failures + 1))
  }
  grep -Fq ".release-please-manifest.json" "scripts/sync-post-stable-release-baselines.sh" || {
    echo "branch-release: post-stable sync must copy the stable manifest baseline"
    failures=$((failures + 1))
  }
fi

if [[ -f ".github/workflows/prerelease-pr.yml" ]]; then
  grep -Eq 'branches:.*premain' ".github/workflows/prerelease-pr.yml" || {
    echo "branch-release: prerelease-pr workflow must target premain"
    failures=$((failures + 1))
  }
  grep -Eq 'googleapis/release-please-action@[0-9a-fA-F]{40}.*\bv4\b' ".github/workflows/prerelease-pr.yml" || {
    echo "branch-release: prerelease-pr workflow must pin release-please v4 by commit SHA"
    failures=$((failures + 1))
  }
  grep -Eq 'config-file:\s*release-please-config\.premain\.json' ".github/workflows/prerelease-pr.yml" || {
    echo "branch-release: prerelease-pr workflow must reference release-please-config.premain.json"
    failures=$((failures + 1))
  }
  grep -Eq 'manifest-file:\s*\.release-please-manifest\.premain\.json' ".github/workflows/prerelease-pr.yml" || {
    echo "branch-release: prerelease-pr workflow must reference .release-please-manifest.premain.json"
    failures=$((failures + 1))
  }
  grep -Fq "scripts/verify-release-cycle-state.sh" ".github/workflows/prerelease-pr.yml" || {
    echo "branch-release: prerelease-pr workflow must verify release-cycle state before release-please"
    failures=$((failures + 1))
  }
  grep -Fq "scripts/verify-branch-version-sync.sh" ".github/workflows/prerelease-pr.yml" || {
    echo "branch-release: prerelease-pr workflow must verify branch version sync before release-please"
    failures=$((failures + 1))
  }
  grep -Eq 'skip-github-release:\s*true' ".github/workflows/prerelease-pr.yml" || {
    echo "branch-release: prerelease-pr workflow must set skip-github-release: true"
    failures=$((failures + 1))
  }
fi

if [[ -f ".github/workflows/release-pr.yml" ]]; then
  grep -Eq 'branches:.*main' ".github/workflows/release-pr.yml" || {
    echo "branch-release: release-pr workflow must target main"
    failures=$((failures + 1))
  }
  grep -Eq 'googleapis/release-please-action@[0-9a-fA-F]{40}.*\bv4\b' ".github/workflows/release-pr.yml" || {
    echo "branch-release: release-pr workflow must pin release-please v4 by commit SHA"
    failures=$((failures + 1))
  }
  grep -Eq 'config-file:\s*release-please-config\.json' ".github/workflows/release-pr.yml" || {
    echo "branch-release: release-pr workflow must reference release-please-config.json"
    failures=$((failures + 1))
  }
  grep -Eq 'manifest-file:\s*\.release-please-manifest\.json' ".github/workflows/release-pr.yml" || {
    echo "branch-release: release-pr workflow must reference .release-please-manifest.json"
    failures=$((failures + 1))
  }
  grep -Fq "scripts/verify-release-cycle-state.sh" ".github/workflows/release-pr.yml" || {
    echo "branch-release: release-pr workflow must verify release-cycle state before release-please"
    failures=$((failures + 1))
  }
  grep -Fq "RELEASE_CYCLE_ALLOW_PENDING_STABLE_PROMOTION=true" ".github/workflows/release-pr.yml" || {
    echo "branch-release: release-pr workflow must explicitly allow pending stable promotion verification"
    failures=$((failures + 1))
  }
  grep -Fq "pending stable promotion accepted for stable Release PR generation" ".github/workflows/release-pr.yml" || {
    echo "branch-release: release-pr workflow must document pending promotion PR generation"
    failures=$((failures + 1))
  }
  grep -Eq 'skip-github-release:\s*true' ".github/workflows/release-pr.yml" || {
    echo "branch-release: release-pr workflow must set skip-github-release: true"
    failures=$((failures + 1))
  }

  # Ensure stable releases promote the RC baseline on premain (e.g., 1.3.0-rc.1 -> 1.3.0),
  # so the stable line never lags behind the prerelease line.
  grep -Fq "release-as:" ".github/workflows/release-pr.yml" || {
    echo "branch-release: release-pr workflow must set release-as to promote the premain RC baseline"
    failures=$((failures + 1))
  }
  grep -Fq "steps.version.outputs.release_as" ".github/workflows/release-pr.yml" || {
    echo "branch-release: release-pr workflow must pass release-as from computed premain RC baseline"
    failures=$((failures + 1))
  }
  grep -Fq ".release-please-manifest.premain.json" ".github/workflows/release-pr.yml" || {
    echo "branch-release: release-pr workflow must read .release-please-manifest.premain.json to align versions"
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

if [[ -f ".github/workflows/quality-gates.yml" ]]; then
  grep -Fq "RELEASE_CYCLE_ALLOW_PENDING_STABLE_PROMOTION" ".github/workflows/quality-gates.yml" || {
    echo "branch-release: quality-gates workflow must pass pending stable promotion mode for premain -> main PR checks"
    failures=$((failures + 1))
  }
  grep -Fq "github.base_ref == 'main'" ".github/workflows/quality-gates.yml" &&
    grep -Fq "github.head_ref == 'premain'" ".github/workflows/quality-gates.yml" || {
      echo "branch-release: quality-gates workflow must limit pending stable promotion mode to premain -> main PR checks"
      failures=$((failures + 1))
    }
fi

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

if [[ -f "release-please-config.json" ]]; then
  if ! python3 - <<'PY'
import json
from pathlib import Path

config = json.loads(Path("release-please-config.json").read_text(encoding="utf-8"))
extra_files = config.get("packages", {}).get(".", {}).get("extra-files", [])

for entry in extra_files:
    if (
        isinstance(entry, dict)
        and entry.get("type") == "json"
        and entry.get("path") == ".release-please-manifest.premain.json"
        and entry.get("jsonpath") == "$['.']"
    ):
        raise SystemExit(0)

raise SystemExit(1)
PY
  then
    echo "branch-release: release-please-config.json must normalize .release-please-manifest.premain.json through stable release-please"
    failures=$((failures + 1))
  fi
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
