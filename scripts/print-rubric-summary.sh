#!/usr/bin/env bash
set -euo pipefail

report="${1:-hgm-infra/evidence/hgm-rubric-report.json}"

if [[ ! -f "${report}" ]]; then
  echo "rubric-summary: no report found at ${report}"
  exit 0
fi

python3 - "${report}" <<'PY'
from __future__ import annotations

import json
import os
import sys
from pathlib import Path

report_path = Path(sys.argv[1])

try:
    data = json.loads(report_path.read_text(encoding="utf-8"))
except Exception as e:
    print(f"rubric-summary: FAIL (unable to parse {report_path}: {e})")
    raise SystemExit(0)

summary = data.get("summary", {}) or {}
status = summary.get("status", "UNKNOWN")
passed = summary.get("pass", "?")
failed = summary.get("fail", "?")
blocked = summary.get("blocked", "?")

print(f"rubric-summary: status={status} pass={passed} fail={failed} blocked={blocked}")

results = data.get("results", []) or []
bad = [r for r in results if r.get("status") in ("FAIL", "BLOCKED")]
if not bad:
    print("rubric-summary: no failing/blocked checks")
    raise SystemExit(0)

repo_root = Path(os.getcwd())

def read_tail(path: Path, max_lines: int = 120) -> str:
    try:
        text = path.read_text(encoding="utf-8", errors="replace")
    except FileNotFoundError:
        return "<missing evidence file>"
    lines = text.splitlines()
    if len(lines) <= max_lines:
        return text.rstrip("\n")
    return "\n".join(lines[-max_lines:]).rstrip("\n")

for r in bad:
    rid = r.get("id", "?")
    cat = r.get("category", "?")
    rst = r.get("status", "?")
    msg = r.get("message", "")
    ev = r.get("evidencePath", "")

    print("")
    print(f"== {rid} ({cat}) {rst} ==")
    if msg:
        print(msg)
    if not ev:
        continue

    ev_path = Path(ev)
    if not ev_path.is_absolute():
        ev_path = (repo_root / ev_path).resolve()

    try:
        ev_rel = ev_path.relative_to(repo_root)
    except ValueError:
        ev_rel = ev_path

    print(f"evidence: {ev_rel}")
    print("--- evidence tail ---")
    print(read_tail(ev_path))
PY

