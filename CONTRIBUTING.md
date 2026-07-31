# Contributing to TableTheory

Thank you for helping strengthen TableTheory. This repository is the DynamoDB-first, three-runtime data contract for Go, TypeScript, and Python, so contributor changes are reviewed for contract safety, cross-language parity, and release hygiene.

## Before you start

- Read the repository instructions in [AGENTS.md](./AGENTS.md).
- Review the public docs in [docs/](./docs/) and the development guidelines in [docs/development-guidelines.md](./docs/development-guidelines.md).
- Check existing issues and pull requests in [`theory-cloud/TableTheory`](https://github.com/theory-cloud/TableTheory/issues).
- Keep changes scoped. Runtime behavior changes usually need contract-test coverage across Go, TypeScript, and Python.

## Code of Conduct

This project and everyone participating in it is governed by the [TableTheory Code of Conduct](CODE_OF_CONDUCT.md). By participating, you are expected to uphold this code and report unacceptable behavior through the project maintainer channels.

## Development setup

TableTheory has three independently implemented SDKs that share one contract. Install the toolchain for the runtimes you touch, then run the fast contributor gate before opening a PR.

### Go runtime

The root module is `github.com/theory-cloud/tabletheory/v3`.

```bash
# Use the pinned Go toolchain from go.mod.
export GOTOOLCHAIN="$(awk '/^toolchain /{print $2}' go.mod | head -n1)"

# One-time tool install for local linting.
make install-tools

# Common local checks.
make fmt
make lint
make test-unit
make unit-cover
```

`make test-unit` is the fast Go unit suite with race detection and coverage. It does not require Docker or DynamoDB Local.

### TypeScript runtime

The TypeScript SDK lives in `ts/`. Development requires Node.js 20 or newer; CI validates the current supported matrix, and Node.js 24 is the preferred local development version.

```bash
cd ts
npm ci
npm run check
```

`npm run check` runs Prettier, ESLint, typecheck, build, and unit tests for the TypeScript package.

### Python runtime

The Python SDK lives in `py/`. Repository dependency verification uses Python 3.14 and `uv`; package metadata supports Python 3.12 and newer.

```bash
uv --directory py sync --frozen --all-extras
uv --directory py run ruff format .
uv --directory py run ruff check .
uv --directory py run pytest -q tests/unit
bash scripts/verify-python-build.sh
```

The Python tests are pytest-based. Do not use `python -m unittest` as the primary Python validation command for this repo.

## Contributor validation gates

### Fast local loop

```bash
make rubric-fast
```

`make rubric-fast` is the recommended contributor loop. It runs formatting, lint, unit, and documentation gates across Go, TypeScript, and Python with `SKIP_INTEGRATION=true`. It does not start Docker and does not run the full security, release, integration, or subtree checks.

### Full repository rubric

```bash
make rubric
```

`make rubric` is the full CI-quality gate. It may install pinned verifier tools, start or require DynamoDB Local for integration coverage, run security and release checks, and stage/verify the Theory Cloud subtree. Run it before asking for final review when your change touches rubric-visible code, CI, release tooling, docs, or contract behavior.

### Contract tests

```bash
make contract-tests
```

The contract test target runs the shared Go, TypeScript, and Python contract suite against DynamoDB Local. Use it for changes that affect observable model, marshaling, lifecycle, locking, TTL, DMS, or runner behavior.

## Pull request process

1. Branch from the intended target branch; most feature work lands through `staging`, while project branches may have their own explicit target.
2. Make the smallest coherent change. Public API changes must be additive unless a major-version migration has been explicitly planned.
3. Keep cross-language parity. A behavior visible in one runtime must be pinned for all three runtimes or must remain internal.
4. Add or update tests and docs with the behavior change.
5. Run the relevant validation gates and include the command output summary in the PR body.
6. Use Conventional Commit subjects and include issue references such as `Closes THE-1234` where appropriate.
7. Do not publish releases, retag, overwrite release assets, or mutate branch-protection settings from a contribution PR.

### PR title format

Use Conventional Commit format:

- `feat:` new feature
- `fix:` bug fix
- `docs:` documentation-only change
- `test:` tests only
- `chore:` maintenance and CI/tooling
- `refactor:` internal refactor without behavior change

Use `feat!:` or `fix!:` only for planned breaking changes, and include a `BREAKING CHANGE:` footer with migration notes.

### PR description checklist

```markdown
## Summary
- <summary>

## Contract impact
- none / docs-only / tooling-only / scenario-visible / DMS-visible

## Validation
- [ ] make rubric-fast
- [ ] make contract-tests (if contract-visible)
- [ ] make rubric (for final review or rubric-visible changes)

## Release impact
- none / release-eligible / breaking-major
```

## Style guide

### Go

- Run `make fmt` before committing.
- Use standard Go naming and table-driven tests.
- Public exported identifiers need useful comments.
- Model structs with `theorydb` tags must also carry matching `json` tags.

### TypeScript

- Keep `ts/src/` and generated/package outputs consistent with the existing package policy.
- Run `npm --prefix ts run check` for TypeScript changes.
- Do not add runtime dependencies casually; this package is consumed downstream.

### Python

- Use `ruff format` and `ruff check` through `uv --directory py run ...`.
- Keep typing strict; run the Python build verifier for public API changes.
- Preserve compatibility with the supported Python version floor unless a coordinated release plan says otherwise.

### Commit messages

- Keep the first line at or under 72 characters.
- Use imperative, present-tense subjects.
- Put issue references in the body when the subject would become too long.
- Never hide a breaking change behind a non-breaking commit type.

## Testing guidelines

- Prefer unit tests that do not require Docker.
- Use DynamoDB Local for integration and contract tests; do not point CI at a real AWS account.
- For cross-runtime behavior, update or add contract scenarios and verify Go, TypeScript, and Python together.
- Do not skip or weaken a rubric gate to make a PR pass.

## Documentation

- Update README or public docs when user-facing behavior changes.
- Keep examples runnable and avoid APIs that do not exist in the current runtime.
- Distribution docs must point to immutable GitHub Releases, not npm or PyPI registries.

### Authoring documentation

The public documentation site at `https://tabletheory.theorycloud.ai/` is a Jekyll site rooted at `docs/`. It uses the Theory Cloud design system and is deployed by `.github/workflows/pages.yml` on every push to `staging`.

**Adding a page**

1. Create the markdown file under `docs/` (e.g. `docs/features/my-new-feature.md`).
2. Add minimal front matter:
   ```yaml
   ---
   title: My new feature
   description: One-sentence summary used by the page meta tag.
   ---
   ```
3. Add a corresponding entry to `docs/_data/nav.yml`:
   - Append an item to the appropriate group's `items:` list with `id`, `title`, `url`, `icon` (and optional `tag`).
   - Append the page's `id` to the linear `order:` list at the position you want for prev/next navigation.
   - Add the `url → id` mapping to `url_to_id:`.

**Callouts**

The site supports info / warn / danger callouts via a Liquid include. Wrap markdown body content in a `capture` block:

```liquid
{% raw %}{% capture body %}
You can put **markdown** here, including [links](../).
{% endcapture %}
{% include callout.html type="info" title="Heads up" content=body %}{% endraw %}
```

`type` accepts `info`, `warn`, or `danger`. Callouts auto-tint per surface (`core`, `mcp`, `auth`, `journal`).

**Local preview**

```bash
cd docs
bundle install            # one-time
bundle exec jekyll serve  # http://localhost:4000/tabletheory/
```

**Surface tint per page**

A page can opt into the MCP / Auth surface tint by adding `surface: mcp` (or `auth`) to its front matter. The default surface for TableTheory is `core`.

**What never to do**

- Don't add internal planning, decisions, or clarification docs to the public nav — those live under `docs/development/` and are excluded from the site by `_config.yml`.
- Don't author content that would only work with a specific runtime registry (npm / PyPI). Distribution is GitHub Releases only — see the [release discipline](./AGENTS.md) doc.

## Questions

Open a GitHub issue or discussion in [`theory-cloud/TableTheory`](https://github.com/theory-cloud/TableTheory) with the smallest reproducible context you can provide.
