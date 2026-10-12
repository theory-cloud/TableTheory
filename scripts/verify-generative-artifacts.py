#!/usr/bin/env python3
from __future__ import annotations

import json
import re
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
GO_TAG_EXPR = re.compile(r'theorydb:"([^"]*)"')
BARE_INDEX_ROLE = re.compile(r"^gsi\d+(pk|sk)$")
INDEX_CLAUSE_PREFIXES = ("index:", "lsi:")
INDEX_ROLE_MODIFIERS = ("pk", "sk")


def fail(message: str) -> None:
    print(f"generative-artifacts: FAIL ({message})", file=sys.stderr)
    raise SystemExit(1)


def validate_go_index_tags(vocab: dict) -> None:
    for entry in vocab.get("tags", []):
        if not isinstance(entry, dict):
            continue
        tag = entry.get("tag")
        go_value = entry.get("go")
        if not isinstance(go_value, str):
            fail(f"vocabulary tag {tag} is missing a go mapping")

        expressions = GO_TAG_EXPR.findall(go_value)
        if not expressions:
            fail(f'vocabulary tag {tag} go mapping has no theorydb:"..." tag')

        for expression in expressions:
            tokens = [token.strip() for token in expression.split(",") if token.strip()]
            for token in tokens:
                if BARE_INDEX_ROLE.match(token):
                    fail(
                        f'vocabulary tag {tag} uses unsupported Go index tag theorydb:"{token}"; '
                        'use theorydb:"index:<name>,pk|sk" (or lsi:<name>,pk|sk)'
                    )

            index_tokens = [token for token in tokens if token.startswith(INDEX_CLAUSE_PREFIXES)]
            if index_tokens:
                if not any(token in INDEX_ROLE_MODIFIERS for token in tokens):
                    fail(
                        f'vocabulary tag {tag} index tag theorydb:"{expression}" must declare a pk or sk modifier'
                    )
                for index_token in index_tokens:
                    index_name = index_token.split(":", 1)[1].strip()
                    if not index_name:
                        fail(f'vocabulary tag {tag} index tag theorydb:"{expression}" is missing an index name')
            elif tag in ("gsiNpk", "gsiNsk"):
                fail(
                    f'vocabulary tag {tag} must use the indexed Go form theorydb:"index:<name>,pk|sk"'
                )


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
    validate_go_index_tags(vocab)

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
