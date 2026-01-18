#!/usr/bin/env bash
set -euo pipefail

go run ./scripts/internal/dms_first_workflow

if [[ -f "py/pyproject.toml" ]]; then
  if [[ ! -d "py/.venv" ]]; then
    bash scripts/verify-python-deps.sh
  fi

  export REPO_ROOT="${REPO_ROOT:-$(pwd)}"
  uv --directory py run python - <<'PY'
from __future__ import annotations

import importlib.util
import os
import pathlib
import sys

from theorydb_py import (
    ModelDefinition,
    assert_model_definition_equivalent_to_dms,
    get_dms_model,
    parse_dms_document,
)

root = pathlib.Path(os.environ["REPO_ROOT"])
dms_path = root / "examples/cdk-multilang/dms/demo.yml"
handler_path = root / "examples/cdk-multilang/lambdas/python/handler.py"

raw = dms_path.read_text(encoding="utf-8")
doc = parse_dms_document(raw)
dms_model = get_dms_model(doc, "DemoItem")

spec = importlib.util.spec_from_file_location("demo_handler", handler_path)
if spec is None or spec.loader is None:
    raise RuntimeError(f"failed to load python handler module: {handler_path}")

mod = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = mod
spec.loader.exec_module(mod)

DemoItem = getattr(mod, "DemoItem")
model = ModelDefinition.from_dataclass(DemoItem, table_name="ignored")
assert_model_definition_equivalent_to_dms(model, dms_model, ignore_table_name=True)

print("dms-first-workflow: python demo model OK")
PY
fi

echo "dms-first-workflow: ok"
