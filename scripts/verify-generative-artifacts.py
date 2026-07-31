#!/usr/bin/env python3
from __future__ import annotations

import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
REQUIRED_FILES = [
    ROOT / "docs" / "llms.txt",
    ROOT / "docs" / "llms-full.txt",
    ROOT / "docs" / "reference" / "tabletheory-vocabulary.json",
    ROOT / "docs" / "ai" / "consumer-rules-template.md",
    ROOT / "docs" / "ai" / "prompt-recipes.md",
]
REQUIRED_TAGS = {"pk", "sk", "gsiNpk", "gsiNsk", "encrypted", "version", "created_at", "updated_at", "ttl", "omitempty"}
REQUIRED_ROLES = {"pk", "sk", "version", "created_at", "updated_at", "ttl"}


def fail(message: str) -> None:
    print(f"generative-artifacts: FAIL ({message})", file=sys.stderr)
    raise SystemExit(1)


def main() -> int:
    for path in REQUIRED_FILES:
        if not path.exists():
            fail(f"missing {path.relative_to(ROOT)}")
        text = path.read_text(encoding="utf-8")
        if "TODO" in text or "FIXME" in text:
            fail(f"placeholder marker remains in {path.relative_to(ROOT)}")

    llms = (ROOT / "docs" / "llms.txt").read_text(encoding="utf-8")
    for token in ("llms-full.txt", "tabletheory-vocabulary.json", "consumer-rules-template", "prompt-recipes"):
        if token not in llms:
            fail(f"llms.txt does not reference {token}")

    vocab = json.loads((ROOT / "docs" / "reference" / "tabletheory-vocabulary.json").read_text(encoding="utf-8"))
    if vocab.get("dms_version") != "0.2":
        fail("vocabulary JSON must declare dms_version 0.2")
    tags = {entry.get("tag") for entry in vocab.get("tags", []) if isinstance(entry, dict)}
    missing_tags = REQUIRED_TAGS - tags
    if missing_tags:
        fail(f"vocabulary JSON missing tags: {sorted(missing_tags)}")
    roles = {entry.get("dms_role") for entry in vocab.get("tags", []) if isinstance(entry, dict) and entry.get("dms_role")}
    missing_roles = REQUIRED_ROLES - roles
    if missing_roles:
        fail(f"vocabulary JSON missing DMS roles: {sorted(missing_roles)}")
    if "GitHub Releases" not in vocab.get("distribution", ""):
        fail("vocabulary JSON must preserve GitHub Releases distribution invariant")

    rules = (ROOT / "docs" / "ai" / "consumer-rules-template.md").read_text(encoding="utf-8")
    for token in ('theorydb:"pk"', "fail closed", "GitHub Release", "defineModel", "theorydb_field"):
        if token not in rules:
            fail(f"consumer rules template missing {token}")

    recipes = (ROOT / "docs" / "ai" / "prompt-recipes.md").read_text(encoding="utf-8")
    for token in ("Go model", "TypeScript model", "Python model", "drift"):
        if token not in recipes:
            fail(f"prompt recipes missing {token}")

    print("generative-artifacts: PASS")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
