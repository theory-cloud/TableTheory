# Development Guidelines (Python)

This guide covers development and contribution conventions for the Python SDK in `py/`.

## Toolchain (pinned)

- Python: **3.14**
- Type checking: `mypy` (strict)
- Lint/format: `ruff` (line length 110; target `py314`)
- Dependency management: `uv` (pinned in CI)

## Common Commands

From the repo root:

- Install deps: `uv --directory py sync --frozen --all-extras`
- Typecheck: `uv --directory py run mypy src`
- Lint: `uv --directory py run ruff check`
- Format check: `uv --directory py run ruff format --check`
- Unit tests + coverage: `uv --directory py run pytest -q`
- Build wheel/sdist: `uv --directory py run python -m build`

## Coding Standards

- Prefer dataclasses with `theorydb_field(...)` roles for pk/sk and lifecycle fields.
- Treat `encrypted` fields as fail-closed: do not allow silent plaintext fallbacks.
- Keep public APIs stable and documented in [API Reference](./api-reference.md).

