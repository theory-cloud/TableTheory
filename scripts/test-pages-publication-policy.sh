#!/usr/bin/env bash
set -euo pipefail

# TTSEC2-M3-T2: durable Pages publication trust-boundary regression test.
#
# Fails if ANY workflow can deploy GitHub Pages while it is triggered by a
# `staging` branch push or a `pull_request` event. Publication/deployment must
# stay reserved for protected release paths (a `main` push, or the release.yml
# tag / manual `workflow_dispatch` handoff).
#
# The checker evaluates each Pages-deploying job's effective eligibility over a
# matrix of synthetic GitHub event contexts, so a composite `if:` expression
# (`a && b || c`, nested parentheses, negations) cannot hide a pull_request or
# staging deployment path. It is a source-controlled structural gate: it does
# not read or mutate hosted repository/environment settings and never contacts
# GitHub.
#
# It also asserts the inverse (ordinary success control): the trusted
# `main`/tag publication journey must remain eligible, and build-only
# (pull_request) runs must not share the publication concurrency queue.

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmpdirs=()

cleanup() {
  local tmpdir
  for tmpdir in "${tmpdirs[@]}"; do
    rm -rf "${tmpdir}"
  done
}
trap cleanup EXIT

# run_checker WORKFLOWS_DIR -> prints findings, exit 0 when clean, 1 otherwise.
run_checker() {
  local workflows_dir="$1"
  python3 - "${workflows_dir}" <<'PY'
import re
import sys
from pathlib import Path

# Synthetic event contexts. The first three are the trust-boundary cases that
# must never reach a Pages deploy; the rest are the trusted publication journey
# that must keep working.
CONTEXTS = [
    ("pull_request to staging", {"github.event_name": "pull_request", "github.ref": "refs/pull/11/merge", "github.ref_name": "11/merge", "github.base_ref": "staging", "github.head_ref": "feature/docs"}),
    ("pull_request to main", {"github.event_name": "pull_request", "github.ref": "refs/pull/12/merge", "github.ref_name": "12/merge", "github.base_ref": "main", "github.head_ref": "premain"}),
    ("push to staging", {"github.event_name": "push", "github.ref": "refs/heads/staging", "github.ref_name": "staging", "github.base_ref": "", "github.head_ref": ""}),
    ("push to main", {"github.event_name": "push", "github.ref": "refs/heads/main", "github.ref_name": "main", "github.base_ref": "", "github.head_ref": ""}),
    ("dispatch on main", {"github.event_name": "workflow_dispatch", "github.ref": "refs/heads/main", "github.ref_name": "main", "github.base_ref": "", "github.head_ref": ""}),
    ("dispatch on tag", {"github.event_name": "workflow_dispatch", "github.ref": "refs/tags/v9.9.9", "github.ref_name": "v9.9.9", "github.base_ref": "", "github.head_ref": ""}),
]
UNSAFE_CONTEXTS = {"pull_request to staging", "pull_request to main", "push to staging"}
PUBLICATION_CONTEXTS = {"push to main", "dispatch on main", "dispatch on tag"}


class Unsupported(Exception):
    pass


def strip_comment(line: str) -> str:
    out = []
    quote = None
    for i, ch in enumerate(line):
        if quote:
            out.append(ch)
            if ch == quote:
                quote = None
        elif ch in "'\"":
            quote = ch
            out.append(ch)
        elif ch == "#" and (i == 0 or line[i - 1] in " \t"):
            break
        else:
            out.append(ch)
    return "".join(out).rstrip()


def indent_of(line: str) -> int:
    return len(line) - len(line.lstrip(" "))


def parse_top_level(lines):
    entries = []
    for i, raw in enumerate(lines):
        s = strip_comment(raw)
        if not s.strip():
            continue
        if indent_of(s) == 0:
            m = re.match(r"^([^:\s][^:]*):\s*(.*)$", s)
            if m and not s.lstrip().startswith("-"):
                entries.append((m.group(1).strip().strip("'\""), i, m.group(2)))
    result = {}
    for idx, (key, start, inline) in enumerate(entries):
        end = entries[idx + 1][1] if idx + 1 < len(entries) else len(lines)
        result.setdefault(key, (start, end, inline))
    return result


def parse_on(lines, start, end, inline):
    triggers = {}
    if inline.strip():
        value = inline.strip()
        if value.startswith("[") and value.endswith("]"):
            for item in value[1:-1].split(","):
                item = item.strip().strip("'\"")
                if item:
                    triggers[item] = {}
        else:
            triggers[value.strip("'\"")] = {}
        return triggers

    current = None
    sub = {}
    last_key = None
    for i in range(start + 1, end):
        s = strip_comment(lines[i])
        if not s.strip():
            continue
        ind = indent_of(s)
        if ind == 2:
            if current is not None:
                triggers[current] = sub
            m = re.match(r"^\s*([^\s:#][^:]*):\s*(.*)$", s)
            current = m.group(1).strip().strip("'\"") if m else None
            sub = {}
            last_key = None
        elif ind >= 4 and current is not None:
            stripped = s.strip()
            if stripped.startswith("-") and last_key:
                sub[last_key].append(stripped[1:].strip().strip("'\""))
                continue
            m = re.match(r"^\s*([^\s:#][^:]*):\s*(.*)$", s)
            if m:
                last_key = m.group(1).strip()
                sub[last_key] = [m.group(2)] if m.group(2) else []
    if current is not None:
        triggers[current] = sub
    return triggers


def scalar_list(values):
    if not values:
        return None
    items = []
    first = values[0].strip()
    if first.startswith("[") and first.endswith("]"):
        for item in first[1:-1].split(","):
            item = item.strip().strip("'\"")
            if item:
                items.append(item)
        return items
    if first:
        items.append(first.strip("'\""))
    for extra in values[1:]:
        extra = extra.strip()
        if extra:
            items.append(extra.strip("'\""))
    return items


def triggered(triggers, event_name, ref, base_ref):
    if event_name == "workflow_dispatch":
        return "workflow_dispatch" in triggers
    if event_name == "push":
        if "push" not in triggers:
            return False
        branch = ref.split("refs/heads/", 1)[1] if ref.startswith("refs/heads/") else ""
        allow = scalar_list(triggers["push"].get("branches"))
        deny = scalar_list(triggers["push"].get("branches-ignore"))
        if deny and branch in deny:
            return False
        if allow is not None and branch not in allow:
            return False
        return True
    if event_name == "pull_request":
        if "pull_request" not in triggers:
            return False
        allow = scalar_list(triggers["pull_request"].get("branches"))
        deny = scalar_list(triggers["pull_request"].get("branches-ignore"))
        if deny and base_ref in deny:
            return False
        if allow is not None and base_ref not in allow:
            return False
        return True
    return False


def parse_jobs(lines, start, end):
    jobs = []
    current = None
    job_start = None
    for i in range(start + 1, end):
        s = strip_comment(lines[i])
        if not s.strip():
            continue
        if indent_of(s) == 2:
            m = re.match(r"^\s*([^\s:#][^:]*):\s*$", s)
            if m:
                if current is not None:
                    jobs.append((current, job_start, i))
                current = m.group(1).strip()
                job_start = i
    if current is not None:
        jobs.append((current, job_start, end))
    return jobs


def job_body(lines, start, end):
    return lines[start:end]


def job_deploys_pages(body_text):
    return re.search(r"uses:\s*actions/deploy-pages@", body_text) is not None


def job_if_value(body):
    for line in body:
        s = strip_comment(line)
        if indent_of(s) == 4:
            m = re.match(r"^\s*if:\s*(.*)$", s)
            if m:
                return m.group(1).strip()
    return None


def concurrency_group(lines, block):
    start, end, inline = block
    if inline.strip():
        return inline.strip()
    for i in range(start + 1, end):
        s = strip_comment(lines[i])
        m = re.match(r"^\s{2}group:\s*(.*)$", s)
        if m:
            return m.group(1).strip()
    return None


def unwrap(expr):
    expr = expr.strip()
    if expr.startswith("${{") and expr.endswith("}}"):
        expr = expr[3:-2].strip()
    return expr


def tokenize(expr):
    tokens = []
    i = 0
    while i < len(expr):
        c = expr[i]
        if c.isspace():
            i += 1
            continue
        two = expr[i:i + 2]
        if two in ("==", "!=", "&&", "||"):
            tokens.append(("op", two))
            i += 2
            continue
        if c == "!":
            tokens.append(("op", "!"))
            i += 1
            continue
        if c in "()":
            tokens.append(("op", c))
            i += 1
            continue
        if c in "'\"":
            j = expr.find(c, i + 1)
            if j == -1:
                raise Unsupported("unterminated string literal")
            tokens.append(("str", expr[i + 1:j]))
            i = j + 1
            continue
        m = re.match(r"[A-Za-z_][A-Za-z0-9_.]*", expr[i:])
        if m:
            tokens.append(("ident", m.group(0)))
            i += len(m.group(0))
            continue
        raise Unsupported(f"unsupported token {c!r}")
    return tokens


def truthy(value):
    if isinstance(value, bool):
        return value
    if isinstance(value, str):
        return value != ""
    return bool(value)


class Parser:
    def __init__(self, tokens, ctx):
        self.tokens = tokens
        self.ctx = ctx
        self.i = 0

    def peek(self):
        return self.tokens[self.i] if self.i < len(self.tokens) else None

    def advance(self):
        tok = self.peek()
        self.i += 1
        return tok

    def parse(self):
        value = self.parse_or()
        if self.i != len(self.tokens):
            raise Unsupported("trailing tokens")
        return value

    def parse_or(self):
        value = self.parse_and()
        while self.peek() == ("op", "||"):
            self.advance()
            rhs = self.parse_and()
            value = value if truthy(value) else rhs
        return value

    def parse_and(self):
        value = self.parse_unary()
        while self.peek() == ("op", "&&"):
            self.advance()
            rhs = self.parse_unary()
            value = rhs if truthy(value) else value
        return value

    def parse_unary(self):
        if self.peek() == ("op", "!"):
            self.advance()
            return not truthy(self.parse_unary())
        return self.parse_cmp()

    def parse_cmp(self):
        value = self.parse_primary()
        nxt = self.peek()
        if nxt and nxt[0] == "op" and nxt[1] in ("==", "!="):
            op = self.advance()[1]
            rhs = self.parse_primary()
            return (value == rhs) if op == "==" else (value != rhs)
        return value

    def parse_primary(self):
        tok = self.advance()
        if tok is None:
            raise Unsupported("unexpected end of expression")
        kind, val = tok
        if kind == "op" and val == "(":
            value = self.parse_or()
            if self.advance() != ("op", ")"):
                raise Unsupported("missing closing parenthesis")
            return value
        if kind == "str":
            return val
        if kind == "ident":
            if self.peek() == ("op", "("):
                raise Unsupported(f"function call {val}(...) is not analyzable")
            return self.ctx.get(val, "")
        raise Unsupported(f"unexpected token {kind}:{val}")


def eval_expr(expr, ctx):
    expr = unwrap(expr)
    if not expr:
        raise Unsupported("empty expression")
    return Parser(tokenize(expr), ctx).parse()


def check_workflows(root: Path):
    problems = []
    deploy_jobs = 0
    files = sorted(list(root.rglob("*.yml")) + list(root.rglob("*.yaml")))
    if not files:
        problems.append(f"{root}: no workflow files found")
        return problems, deploy_jobs

    for path in files:
        lines = path.read_text(encoding="utf-8").splitlines()
        top = parse_top_level(lines)
        triggers = {}
        if "on" in top:
            start, end, inline = top["on"]
            triggers = parse_on(lines, start, end, inline)
        if "jobs" not in top:
            continue
        job_start, job_end, _ = top["jobs"]
        group_expr = concurrency_group(lines, top["concurrency"]) if "concurrency" in top else None

        for job_id, jstart, jend in parse_jobs(lines, job_start, job_end):
            body = job_body(lines, jstart, jend)
            if not job_deploys_pages("\n".join(body)):
                continue
            deploy_jobs += 1
            condition = job_if_value(body)

            for name, ctx in CONTEXTS:
                if not triggered(triggers, ctx["github.event_name"], ctx["github.ref"], ctx["github.base_ref"]):
                    continue
                try:
                    eligible = True if condition is None else truthy(eval_expr(condition, ctx))
                except Unsupported as exc:
                    problems.append(
                        f"{path}: deploy job {job_id!r} has a condition this gate cannot analyze "
                        f"({exc}); use a simple main push/dispatch guard"
                    )
                    continue
                if eligible and name in UNSAFE_CONTEXTS:
                    problems.append(
                        f"{path}: deploy job {job_id!r} can publish Pages for the {name!r} context "
                        f"(condition: {condition!r})"
                    )
                if not eligible and name in PUBLICATION_CONTEXTS:
                    problems.append(
                        f"{path}: deploy job {job_id!r} no longer publishes for the trusted {name!r} context "
                        f"(condition: {condition!r})"
                    )

            if group_expr is not None:
                build_ctxs = [
                    (name, ctx)
                    for name, ctx in CONTEXTS
                    if name in UNSAFE_CONTEXTS
                    and triggered(triggers, ctx["github.event_name"], ctx["github.ref"], ctx["github.base_ref"])
                ]
                pub_ctxs = [
                    (name, ctx)
                    for name, ctx in CONTEXTS
                    if name in PUBLICATION_CONTEXTS
                    and triggered(triggers, ctx["github.event_name"], ctx["github.ref"], ctx["github.base_ref"])
                ]
                if build_ctxs and pub_ctxs:
                    try:
                        groups = {
                            (name, str(eval_expr(group_expr, ctx)))
                            for name, ctx in build_ctxs + pub_ctxs
                        }
                    except Unsupported as exc:
                        problems.append(
                            f"{path}: concurrency group this gate cannot analyze ({exc}); "
                            "build-only runs must use a group distinct from publication"
                        )
                    else:
                        build_groups = {g for name, g in groups if name in UNSAFE_CONTEXTS}
                        pub_groups = {g for name, g in groups if name in PUBLICATION_CONTEXTS}
                        shared = build_groups & pub_groups
                        if shared:
                            problems.append(
                                f"{path}: build-only runs share concurrency group "
                                f"{', '.join(sorted(shared)) or '<empty>'} with publication runs; a "
                                "build-only update could replace a pending publication"
                            )

    return problems, deploy_jobs


def main():
    root = Path(sys.argv[1])
    problems, deploy_jobs = check_workflows(root)
    if problems:
        for problem in problems:
            print(f"pages-publication-policy: FAIL ({problem})")
        raise SystemExit(1)
    print(f"pages-publication-policy: clean ({deploy_jobs} deploy job(s) reviewed in {root})")


if __name__ == "__main__":
    main()
PY
}

expect_accept() {
  local dir="$1"
  local context="$2"
  local output
  if ! output="$(run_checker "${dir}" 2>&1)"; then
    printf '%s\n' "${output}"
    echo "pages-publication-policy: ${context}: expected the checker to pass"
    exit 1
  fi
}

expect_reject() {
  local dir="$1"
  local needle="$2"
  local context="$3"
  local output
  if output="$(run_checker "${dir}" 2>&1)"; then
    printf '%s\n' "${output}"
    echo "pages-publication-policy: ${context}: expected the checker to fail"
    exit 1
  fi
  if ! grep -Fq -- "${needle}" <<<"${output}"; then
    printf '%s\n' "${output}"
    echo "pages-publication-policy: ${context}: expected failure to mention: ${needle}"
    exit 1
  fi
}

# 1. The real workflows must pass: no Pages deploy is reachable from staging or
#    pull_request, the main/tag publication journey is retained, and build-only
#    runs use a concurrency group distinct from publication.
expect_accept "${repo_root}/.github/workflows" "repository workflows"

# 2. Pull request trigger + an unguarded deploy job.
pr_fixture="$(mktemp -d)"
tmpdirs+=("${pr_fixture}")
cat >"${pr_fixture}/pr-deploy.yml" <<'YAML'
name: pr-pages
on:
  pull_request:
    branches: [staging]
concurrency:
  group: pages
  cancel-in-progress: false
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/upload-pages-artifact@0000000000000000000000000000000000000000
  deploy:
    needs: build
    runs-on: ubuntu-latest
    steps:
      - uses: actions/deploy-pages@0000000000000000000000000000000000000000
YAML
expect_reject "${pr_fixture}" "can publish Pages" "pull_request deploy job"

# 3. Composite condition that admits a staging push alongside main.
composite_fixture="$(mktemp -d)"
tmpdirs+=("${composite_fixture}")
cat >"${composite_fixture}/composite.yml" <<'YAML'
name: composite-pages
on:
  push:
    branches: [staging, main]
concurrency:
  group: pages
  cancel-in-progress: false
jobs:
  deploy:
    runs-on: ubuntu-latest
    if: (github.event_name == 'push' && (github.ref == 'refs/heads/main' || github.ref == 'refs/heads/staging')) || github.event_name == 'workflow_dispatch'
    steps:
      - uses: actions/deploy-pages@0000000000000000000000000000000000000000
YAML
expect_reject "${composite_fixture}" "'push to staging'" "composite staging deploy condition"

# 4. Negated guard that still admits a staging push.
negated_fixture="$(mktemp -d)"
tmpdirs+=("${negated_fixture}")
cat >"${negated_fixture}/negated.yml" <<'YAML'
name: negated-pages
on:
  push:
    branches: [staging]
concurrency:
  group: pages
  cancel-in-progress: false
jobs:
  deploy:
    runs-on: ubuntu-latest
    if: github.event_name != 'pull_request'
    steps:
      - uses: actions/deploy-pages@0000000000000000000000000000000000000000
YAML
expect_reject "${negated_fixture}" "'push to staging'" "negated staging deploy condition"

# 5. A deploy condition that cannot be reduced to the supported guard shape
#    fails closed instead of silently passing.
opaque_fixture="$(mktemp -d)"
tmpdirs+=("${opaque_fixture}")
cat >"${opaque_fixture}/opaque.yml" <<'YAML'
name: opaque-pages
on:
  push:
    branches: [main]
concurrency:
  group: pages
  cancel-in-progress: false
jobs:
  deploy:
    runs-on: ubuntu-latest
    if: contains(github.ref, 'main')
    steps:
      - uses: actions/deploy-pages@0000000000000000000000000000000000000000
YAML
expect_reject "${opaque_fixture}" "cannot analyze" "opaque deploy condition"

# 6. Publication runs sharing a concurrency group with build-only runs.
shared_group_fixture="$(mktemp -d)"
tmpdirs+=("${shared_group_fixture}")
cat >"${shared_group_fixture}/shared-group.yml" <<'YAML'
name: shared-group-pages
on:
  push:
    branches: [main]
  pull_request:
    branches: [staging]
concurrency:
  group: pages
  cancel-in-progress: false
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo build
  deploy:
    needs: build
    runs-on: ubuntu-latest
    if: (github.event_name == 'push' && github.ref == 'refs/heads/main') || github.event_name == 'workflow_dispatch'
    steps:
      - uses: actions/deploy-pages@0000000000000000000000000000000000000000
YAML
expect_reject "${shared_group_fixture}" "share concurrency group" "shared publication concurrency group"

# 7. A guard that never publishes is rejected (the main/tag journey must remain).
dead_fixture="$(mktemp -d)"
tmpdirs+=("${dead_fixture}")
cat >"${dead_fixture}/dead.yml" <<'YAML'
name: dead-pages
on:
  push:
    branches: [main]
concurrency:
  group: pages
  cancel-in-progress: false
jobs:
  deploy:
    runs-on: ubuntu-latest
    if: github.ref == 'refs/heads/premain'
    steps:
      - uses: actions/deploy-pages@0000000000000000000000000000000000000000
YAML
expect_reject "${dead_fixture}" "no longer publishes" "over-restrictive deploy condition"

# 8. A main-only publication workflow is accepted (ordinary success control).
main_only_fixture="$(mktemp -d)"
tmpdirs+=("${main_only_fixture}")
cat >"${main_only_fixture}/main-only.yml" <<'YAML'
name: main-only-pages
on:
  push:
    branches: [main]
  pull_request:
    branches: [staging]
  workflow_dispatch:
concurrency:
  group: ${{ github.event_name == 'pull_request' && github.ref || 'pages' }}
  cancel-in-progress: false
jobs:
  build:
    runs-on: ubuntu-latest
    if: (github.event_name == 'push' && github.ref == 'refs/heads/main') || github.event_name == 'workflow_dispatch' || github.event_name == 'pull_request'
    steps:
      - run: echo build
  deploy:
    needs: build
    runs-on: ubuntu-latest
    if: (github.event_name == 'push' && github.ref == 'refs/heads/main') || github.event_name == 'workflow_dispatch'
    steps:
      - uses: actions/deploy-pages@0000000000000000000000000000000000000000
YAML
expect_accept "${main_only_fixture}" "main-only publication fixture"

echo "pages-publication-policy: PASS"
