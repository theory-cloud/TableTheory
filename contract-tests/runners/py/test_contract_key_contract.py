from __future__ import annotations

from pathlib import Path

from theorydb_py.key_contract import load_key_contract_file, verify_derived_key_fixtures


def _repo_root() -> Path:
    return Path(__file__).resolve().parents[3]


def test_key_contract_fixtures_execute_for_python() -> None:
    root = _repo_root() / "contract-tests" / "key-contracts"
    fixtures = sorted(path for version in root.glob("v*") for path in version.glob("*.yml"))
    assert fixtures

    seen: set[str] = set()
    for path in fixtures:
        contract = load_key_contract_file(path)
        verify_derived_key_fixtures(contract)
        seen.update(str(key["name"]) for key in contract["derived_keys"] if isinstance(key, dict))

    assert {"CanonicalPolicyKey", "CanonicalBindingKey", "InterfaceScopeKey", "LowercaseLookupKey"} <= seen
