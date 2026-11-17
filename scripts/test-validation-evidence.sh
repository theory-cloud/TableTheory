#!/usr/bin/env bash
# Purpose: controls for the release-history input class of the validation
# evidence producer and verifier.
#
# A generated release moves exactly two tracked files: the release-please
# manifest and the changelog. Neither carries product bytes, so neither may be
# able to stale a record; everything else must still fail closed. Every case
# below runs the real scripts against a throwaway git repository, and the policy
# cases run the real scripts/evidence/check-policy.py, so nothing is mocked at
# the entry point.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
python_bin="${PYTHON:-python3}"
evidence_py="${repo_root}/scripts/evidence/validation_evidence.py"
policy_py="${repo_root}/scripts/evidence/check-policy.py"
registry_real="${repo_root}/scripts/evidence/validation-gates.json"

tmp="$(mktemp -d "${TMPDIR:-/tmp}/tabletheory-validation-evidence.XXXXXX")"
trap 'rm -rf "${tmp}"' EXIT

# The rubric collector runs this self-test with its own evidence environment
# exported (force/reuse/profile). The fixture controls those itself.
unset TABLE_THEORY_EVIDENCE_FORCE TABLE_THEORY_EVIDENCE_REUSE TABLE_THEORY_EVIDENCE_PROFILE \
  TABLE_THEORY_EVIDENCE_CONSUME_LOG

scratch="${tmp}/repo"
registry="${tmp}/registry.json"
mkdir -p "${scratch}/evidence" "${scratch}/features"

cat >"${registry}" <<'JSON'
{
  "schemaVersion": 1,
  "evidenceDirectory": "evidence",
  "localLogDirectory": "evidence/local",
  "clockSkewSeconds": 300,
  "securityMaxAgeSeconds": 86400,
  "runner": {"maxWorkers": 2, "resources": {"cpu": 2, "repo-write": 1, "docker": 1, "network": 1}},
  "nonProductPaths": ["docs/", "scripts/"],
  "releaseOnly": {
    "exact": ["CHANGELOG.md", ".release-please-manifest.json"],
    "prefixes": [],
    "patterns": []
  },
  "releaseNotes": {
    "exact": ["CHANGELOG.md", ".release-please-manifest.json"]
  },
  "securityInputs": {
    "exact": [],
    "patterns": []
  },
  "gates": {
    "unit": {
      "kind": "source",
      "environment": ["go", "node", "python"],
      "profiles": ["default"],
      "resource": "cpu",
      "paths": ["src.txt", "features/", "CHANGELOG.md", ".release-please-manifest.json"],
      "commands": [{"id": "unit", "argv": ["true"]}]
    }
  }
}
JSON

# The tracked proof profile is declared from this host's real probe, so the
# fixture can collect the default profile anywhere.
live_environment_json="$("${python_bin}" "${evidence_py}" probe)"
"${python_bin}" - "${registry}" "${live_environment_json}" <<'PY'
import json
import sys

path, live_raw = sys.argv[1], sys.argv[2]
probe = json.loads(live_raw)
data = json.loads(open(path, encoding="utf-8").read())
data["proofProfiles"] = {
    "default": {
        "environment": {
            "go": probe["go"],
            "node": str(probe["node"]).lstrip("v").split(".")[0],
            "python": ".".join(str(probe["python"]).split(".")[:2]),
            "platform": probe["platform"],
        }
    }
}
with open(path, "w", encoding="utf-8") as handle:
    json.dump(data, handle, indent=2)
    handle.write("\n")
PY

init_repo() {
  (
    cd "${scratch}"
    git init -q -b staging
    git config user.name "validation-evidence self-test"
    git config user.email "validation-evidence@example.invalid"
    git config commit.gpgsign false
    printf 'one\n' >src.txt
    printf 'feature\n' >features/a.txt
    printf '# Changelog\n\n## [1.0.0](https://example.invalid/compare/v0.9.0...v1.0.0) (2026-01-01)\n\n* seed\n' >CHANGELOG.md
    printf '{\n  ".": "1.0.0"\n}\n' >.release-please-manifest.json
    printf 'keep\n' >evidence/.gitkeep
    git add -A
    git commit -q -m "fix: seed"
  )
}

commit_all() {
  local message="$1"
  (
    cd "${scratch}"
    git add -A
    git commit -q -m "${message}"
  )
}

collect() {
  "${python_bin}" "${evidence_py}" --registry "${registry}" collect --root "${scratch}" "$@"
}

verify() {
  "${python_bin}" "${evidence_py}" --registry "${registry}" verify --root "${scratch}" "$@"
}

pass_count=0
expect_ok() {
  local label="$1"
  shift
  if ! "$@" >"${tmp}/out.log" 2>&1; then
    echo "test-validation-evidence: FAIL (${label})" >&2
    cat "${tmp}/out.log" >&2
    exit 1
  fi
  pass_count=$((pass_count + 1))
  echo "test-validation-evidence: PASS ${label}"
}

expect_fail() {
  local label="$1"
  shift
  if "$@" >"${tmp}/out.log" 2>&1; then
    echo "test-validation-evidence: FAIL (${label}; command unexpectedly passed)" >&2
    cat "${tmp}/out.log" >&2
    exit 1
  fi
  pass_count=$((pass_count + 1))
  echo "test-validation-evidence: PASS ${label}"
}

expect_output() {
  local label="$1"
  local needle="$2"
  shift 2
  if ! "$@" >"${tmp}/out.log" 2>&1; then
    echo "test-validation-evidence: FAIL (${label})" >&2
    cat "${tmp}/out.log" >&2
    exit 1
  fi
  if ! grep -Fq -- "${needle}" "${tmp}/out.log"; then
    echo "test-validation-evidence: FAIL (${label}; missing ${needle})" >&2
    cat "${tmp}/out.log" >&2
    exit 1
  fi
  pass_count=$((pass_count + 1))
  echo "test-validation-evidence: PASS ${label}"
}

# 1. Baseline: collect once, then verify the identical tree.
init_repo
expect_ok "collect the release-history fixture" collect --gate unit
expect_output "verify identical inputs" "mode=evidence-verified" verify --gate unit

# 2. The release transform itself: release-please bumps the manifest value and
# prepends a generated section to the changelog, exactly the two paths a
# TableTheory release commit modifies.
cat >"${scratch}/.release-please-manifest.json" <<'JSON'
{
  ".": "1.2.0"
}
JSON
cat >"${tmp}/changelog.new" <<'MD'
## [1.2.0](https://example.invalid/compare/v1.1.0...v1.2.0) (2026-01-02)


### Bug Fixes

* **release:** keep the generated section out of product proof ([abcdef1](https://example.invalid/commit/abcdef1))
MD
printf '# Changelog\n\n' >"${scratch}/CHANGELOG.md.merged"
cat "${tmp}/changelog.new" >>"${scratch}/CHANGELOG.md.merged"
printf '\n' >>"${scratch}/CHANGELOG.md.merged"
tail -n +3 "${scratch}/CHANGELOG.md" >>"${scratch}/CHANGELOG.md.merged"
mv "${scratch}/CHANGELOG.md.merged" "${scratch}/CHANGELOG.md"
commit_all "fix: release 1.2.0 generated by release-please"
expect_output "verify accepts a release-please back-merge unchanged records" \
  "mode=release-transformation-verified" verify --gate unit

# 3. Release history may be edited by hand without staling product proof.
printf '\n* a hand-written note in an existing section\n' >>"${scratch}/CHANGELOG.md"
commit_all "fix: add a hand-written note to an existing section"
expect_ok "a changelog prose edit does not stale a record" verify --gate unit

# 4. The manifest's structure still binds: an unexpected package fails closed.
cp "${scratch}/.release-please-manifest.json" "${tmp}/manifest.json.bak"
cat >"${scratch}/.release-please-manifest.json" <<'JSON'
{
  ".": "1.2.0",
  "ts": "9.9.9"
}
JSON
commit_all "fix: add an unexpected package to the release manifest"
expect_fail "a release-manifest structural edit still stales the record" verify --gate unit
cp "${tmp}/manifest.json.bak" "${scratch}/.release-please-manifest.json"
commit_all "fix: restore the release manifest"

# 5. A product byte still fails closed behind release history.
cp "${scratch}/src.txt" "${tmp}/src.txt.bak"
printf 'x\n' >>"${scratch}/src.txt"
commit_all "fix: change a product byte"
expect_fail "a product byte still stales the record behind release history" verify --gate unit
cp "${tmp}/src.txt.bak" "${scratch}/src.txt"
commit_all "fix: restore the product byte"
expect_output "verify recovers when the product byte returns" \
  "mode=release-transformation-verified" verify --gate unit

# 6. The policy holds the class to the reviewed allowlist. Each case mutates a
#    copy of the real registry, so the real tree is what the final case proves.
policy_case() {
  local label="$1"
  local expected="$2"
  shift 2
  cp "${registry_real}" "${tmp}/registry-under-test.json"
  "${python_bin}" - "${tmp}/registry-under-test.json" "$@" <<'PY'
import json
import sys

path, mutation = sys.argv[1], sys.argv[2]
data = json.loads(open(path, encoding="utf-8").read())
if mutation == "drop":
    data.pop("releaseNotes", None)
elif mutation == "unreviewed":
    data["releaseNotes"]["exact"].append("docs/notes.md")
elif mutation == "prefixes":
    data["releaseNotes"] = {"prefixes": ["CHANGELOG.md"]}
else:
    raise SystemExit(f"unknown mutation {mutation}")
with open(path, "w", encoding="utf-8") as handle:
    json.dump(data, handle, indent=2)
    handle.write("\n")
PY
  local output
  if output="$("${python_bin}" "${policy_py}" --registry "${tmp}/registry-under-test.json" 2>&1)"; then
    echo "test-validation-evidence: FAIL (${label}; policy accepted the mutation)" >&2
    printf '%s\n' "${output}" >&2
    exit 1
  fi
  if ! grep -Fq -- "${expected}" <<<"${output}"; then
    echo "test-validation-evidence: FAIL (${label}; missing ${expected})" >&2
    printf '%s\n' "${output}" >&2
    exit 1
  fi
  pass_count=$((pass_count + 1))
  echo "test-validation-evidence: PASS ${label}"
}

policy_case "a registry without the class fails the policy" \
  "must declare the releaseNotes class" drop
policy_case "an unreviewed release-notes path fails the policy" \
  "must match the reviewed allowlist exactly" unreviewed
policy_case "a prefixed release-notes class fails the policy" \
  "must declare exact paths only" prefixes
expect_ok "the real registry satisfies the release-notes policy" \
  "${python_bin}" "${policy_py}"

echo "test-validation-evidence: PASS (${pass_count} checks)"
