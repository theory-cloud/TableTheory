#!/usr/bin/env bash
set -euo pipefail

# Focused policy test for the consumer/Python/Pages dependency surfaces.
#
# Findings:
#   e11fbff8  Python install guidance must not let a higher PyPI version of the
#             top-level package win over the release-asset wheel.
#   516aa188  The Pages build must resolve a reviewed, committed Gemfile.lock in
#             frozen mode instead of ambient compatible gems.
#   1dd9c453  URL-only Renovate managers must not be pointed at integrity-bearing
#             lockfiles / hashed requirements files whose adjacent hash they
#             cannot regenerate.

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${repo_root}"

failures=0
fail() {
  echo "consumer-dependency-policy: FAIL ($1)"
  failures=$((failures + 1))
}

scratch="$(mktemp -d)"
trap 'rm -rf "${scratch}"' EXIT

# --- e11fbff8: find-links top-level package must exclude public indexes --------
generator="scripts/generate-python-find-links-index.py"
if [[ -f "${generator}" ]]; then
  cat > "${scratch}/releases.json" <<'JSON'
{"releases":[{"tag_name":"v2.0.1","published_at":"2026-01-01T00:00:00Z","prerelease":false,"assets":[{"name":"tabletheory_py-2.0.1-py3-none-any.whl","browser_download_url":"https://github.com/theory-cloud/TableTheory/releases/download/v2.0.1/tabletheory_py-2.0.1-py3-none-any.whl","state":"uploaded"}]}]}
JSON
  if python3 "${generator}" --releases-json "${scratch}/releases.json" \
    --output "${scratch}/index.html" >/dev/null 2>&1; then
    grep -q -- "--no-index" "${scratch}/index.html" \
      || fail "generated find-links index must install the top-level package with --no-index"
    if grep -Eq -- '--find-links [^ "]* tabletheory-py( |$)' "${scratch}/index.html"; then
      fail "generated find-links index must not advertise an index-enabled unpinned install"
    fi
  else
    fail "generate-python-find-links-index.py failed on a valid fixture"
  fi
else
  fail "missing ${generator}"
fi

py_guide="py/docs/getting-started.md"
if [[ -f "${py_guide}" ]]; then
  grep -q -- "--no-index" "${py_guide}" \
    || fail "${py_guide} must install the find-links top-level package with --no-index"
  if grep -Eq -- '--find-links [^ "]* tabletheory-py( |$)' "${py_guide}"; then
    fail "${py_guide} must not advertise an index-enabled unpinned find-links install"
  fi
else
  fail "missing ${py_guide}"
fi

# --- 516aa188: reviewed Pages lock, frozen install ----------------------------
if ! git ls-files --error-unmatch docs/Gemfile.lock >/dev/null 2>&1; then
  fail "docs/Gemfile.lock must be committed and tracked"
fi
if git check-ignore -q --no-index -- docs/Gemfile.lock 2>/dev/null; then
  fail "docs/Gemfile.lock must not be git-ignored"
fi
if [[ -f ".github/workflows/pages.yml" ]]; then
  grep -q "BUNDLE_FROZEN" .github/workflows/pages.yml \
    || fail "pages workflow must run bundler in frozen mode"
  grep -q "bundle exec jekyll build" .github/workflows/pages.yml \
    || fail "pages workflow must build with the locked bundle"
fi

# --- 1dd9c453: no integrity-bearing file in a URL-only Renovate manager -------
integrity_patterns=(
  '"/(^|/)package-lock\\.json$/"'
  '"/(^|/)requirements.*\\.txt$/"'
)
renovate_docs=(
  "docs/guides/consumer-updates.md"
  "py/docs/getting-started.md"
  "ts/docs/getting-started.md"
  "docs/runtimes/typescript.md"
  "docs/runtimes/python/getting-started.md"
  "docs/runtimes/typescript/getting-started.md"
)
for doc in "${renovate_docs[@]}"; do
  [[ -f "${doc}" ]] || continue
  for pattern in "${integrity_patterns[@]}"; do
    if grep -Fq -- "${pattern}" "${doc}"; then
      fail "${doc} must not wire an integrity-bearing file into a URL-only Renovate manager (${pattern})"
    fi
  done
done

if [[ "${failures}" -ne 0 ]]; then
  echo "consumer-dependency-policy: FAIL (${failures} issue(s))"
  exit 1
fi

echo "consumer-dependency-policy: PASS (python index exclusion, Pages lock, Renovate integrity)"
