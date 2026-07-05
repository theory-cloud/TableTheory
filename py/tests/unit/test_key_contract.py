from __future__ import annotations

import math
from decimal import Decimal
from pathlib import Path

import pytest

from tabletheory_py import ValidationError
from tabletheory_py.key_contract import (
    evaluate_derived_key,
    evaluate_derived_key_definition,
    load_key_contract_file,
    parse_derived_key_contract,
    verify_derived_key_fixtures,
)


def _repo_root() -> Path:
    return Path(__file__).resolve().parents[3]


def _fixture(version: str, name: str) -> dict[str, object]:
    return load_key_contract_file(_repo_root() / "contract-tests" / "key-contracts" / version / name)


def test_v01_theorymcp_fixtures() -> None:
    contract = _fixture("v0.1", "theorymcp-derived-keys.yml")
    verify_derived_key_fixtures(contract)

    counts = {key["name"]: len(key.get("fixtures", [])) for key in contract["derived_keys"]}
    assert counts["WildcardScope"] == 4
    assert counts["CanonicalPolicyKey"] == 3
    assert counts["ScalarNumberKey"] == 3


def test_v02_transform_fixtures() -> None:
    contract = _fixture("v0.2", "derived-key-transforms.yml")
    verify_derived_key_fixtures(contract)

    assert (
        evaluate_derived_key(
            contract,
            "LowercaseLookupKey",
            {"namespace": " İSTANBUL ", "repository": "CAFÉ/Docs"},
        )
        == "ns=%C4%B0stanbul|repo=caf%C3%89%2Fdocs"
    )
    assert (
        evaluate_derived_key(contract, "LiteralUrlEncodedKey", {}) == "marker=region%2Fus-east-1|space=a%20b"
    )


def test_transforms_defaults_omission_and_escaping() -> None:
    key = {
        "name": "ExampleKey",
        "join": "|",
        "inputs": [{"name": "scope", "optional": True}, {"name": "mode", "optional": True}],
        "segments": [
            {
                "name": "scope",
                "prefix": "scope=",
                "value": {"input": "scope"},
                "transforms": ["trim", "wildcard_empty"],
            },
            {
                "name": "mode",
                "prefix": "mode=",
                "value": {"input": "mode"},
                "transforms": ["trim"],
                "default": "manual",
                "optional": True,
                "omit_when": {"default": True},
            },
        ],
    }

    assert evaluate_derived_key_definition(key, {"scope": " keybank "}) == "scope=keybank"
    assert evaluate_derived_key_definition(key, {"scope": "", "mode": "auto"}) == "scope=*|mode=auto"
    assert evaluate_derived_key_definition(key, {"scope": "\u0085keybank\ufeff"}) == "scope=keybank"

    composite = {
        "name": "Composite",
        "join": "|",
        "inputs": [{"name": "tenant"}, {"name": "resource"}],
        "segments": [
            {"prefix": "tenant=", "value": {"input": "tenant"}, "transforms": ["trim"]},
            {"prefix": "resource=", "value": {"input": "resource"}, "transforms": ["trim"]},
        ],
    }
    assert (
        evaluate_derived_key_definition(composite, {"tenant": "user/*", "resource": "café"})
        == "tenant=user%2F%2A|resource=caf%C3%A9"
    )


def test_numbers_are_canonical_and_finite() -> None:
    key = {
        "name": "NumberKey",
        "join": "",
        "inputs": [{"name": "value", "type": "number"}],
        "segments": [{"prefix": "n=", "value": {"input": "value"}}],
    }
    assert evaluate_derived_key_definition(key, {"value": 1e21}) == "n=1000000000000000000000"
    assert evaluate_derived_key_definition(key, {"value": 1e-6}) == "n=0.000001"
    assert evaluate_derived_key_definition(key, {"value": -0.0}) == "n=0"
    for value in (math.nan, math.inf, -math.inf):
        with pytest.raises(ValidationError, match="finite number"):
            evaluate_derived_key_definition(key, {"value": value})


def test_parse_rejects_invalid_contracts() -> None:
    with pytest.raises(ValidationError, match="invalid key contract YAML/JSON"):
        parse_derived_key_contract("derived_keys: [")
    with pytest.raises(ValidationError, match="must be an object"):
        parse_derived_key_contract("[]")
    with pytest.raises(ValidationError, match=r"include derived_keys\[\]"):
        parse_derived_key_contract('tabletheory_model_contract_version: "0.1"\nderived_keys: []')
    with pytest.raises(ValidationError, match="unsupported tabletheory_model_contract_version"):
        parse_derived_key_contract('tabletheory_model_contract_version: "9.9"\nderived_keys: []')
    with pytest.raises(ValidationError, match="duplicate derived key"):
        parse_derived_key_contract(
            """
tabletheory_model_contract_version: "0.1"
derived_keys:
  - &key
    name: Duplicate
    join: ""
    segments: [{ value: { literal: "a" } }]
  - *key
"""
        )
    with pytest.raises(ValidationError, match="models must be an array"):
        parse_derived_key_contract(
            """
tabletheory_model_contract_version: "0.1"
derived_keys:
  - name: Key
    join: ""
    segments: [{ value: { literal: "a" } }]
models: {}
"""
        )
    with pytest.raises(ValidationError, match="model contract missing name"):
        parse_derived_key_contract(
            """
tabletheory_model_contract_version: "0.1"
derived_keys:
  - name: Key
    join: ""
    segments: [{ value: { literal: "a" } }]
models:
  - derived_keys: [Key]
"""
        )
    with pytest.raises(ValidationError, match="references unknown derived key"):
        parse_derived_key_contract(
            """
tabletheory_model_contract_version: "0.1"
derived_keys:
  - name: Key
    join: ""
    segments: [{ value: { literal: "a" } }]
models:
  - name: Model
    derived_keys: [Missing]
"""
        )
    with pytest.raises(ValidationError, match="unsupported transform"):
        parse_derived_key_contract(
            """
tabletheory_model_contract_version: "0.1"
derived_keys:
  - name: Bad
    join: ""
    segments:
      - value: { input: id }
        transforms: [lower]
"""
        )


def test_definition_validation_errors_cover_schema_guards() -> None:
    for definition, message in [
        ({}, "missing name"),
        ({"name": "Bad", "join": 1, "segments": [{"value": {"literal": "a"}}]}, "join must be a string"),
        ({"name": "Bad", "join": "", "segments": []}, "missing segments"),
        (
            {"name": "Bad", "join": "", "inputs": {}, "segments": [{"value": {"literal": "a"}}]},
            "inputs must be",
        ),
        (
            {"name": "Bad", "join": "", "inputs": [{}], "segments": [{"value": {"literal": "a"}}]},
            "input missing name",
        ),
        (
            {
                "name": "Bad",
                "join": "",
                "inputs": [{"name": "id"}, {"name": "id"}],
                "segments": [{"value": {"literal": "a"}}],
            },
            "duplicate input",
        ),
        ({"name": "Bad", "join": "", "segments": ["x"]}, "segment must be an object"),
        (
            {"name": "Bad", "join": "", "segments": [{"literal": "a", "value": {"literal": "b"}}]},
            "exactly one value source",
        ),
        (
            {
                "name": "Bad",
                "join": "",
                "inputs": [{"name": "id"}],
                "segments": [{"value": {"input": "missing"}}],
            },
            "unknown input",
        ),
        (
            {"name": "Bad", "join": "", "segments": [{"value": {"literal": "a"}, "transforms": "trim"}]},
            "transforms must be an array",
        ),
        (
            {"name": "Bad", "join": "", "segments": [{"value": {"literal": "a"}}], "fixtures": {}},
            "fixtures must be an array",
        ),
        (
            {"name": "Bad", "join": "", "segments": [{"value": {"literal": "a"}}], "fixtures": [{}]},
            "fixture missing name",
        ),
        (
            {
                "name": "Bad",
                "join": "",
                "segments": [{"value": {"literal": "a"}}],
                "fixtures": [
                    {"name": "same", "input": {}, "expect": "a"},
                    {"name": "same", "input": {}, "expect": "a"},
                ],
            },
            "duplicate fixture",
        ),
        (
            {
                "name": "Bad",
                "join": "",
                "segments": [{"value": {"literal": "a"}}],
                "fixtures": [{"name": "fixture", "expect": "a"}],
            },
            "missing input",
        ),
        (
            {
                "name": "Bad",
                "join": "",
                "segments": [{"value": {"literal": "a"}}],
                "fixtures": [{"name": "fixture", "input": {}, "expect": 1}],
            },
            "expect must be a string",
        ),
    ]:
        with pytest.raises(ValidationError, match=message):
            evaluate_derived_key_definition(definition)


def test_evaluation_error_and_edge_paths() -> None:
    contract = {
        "tabletheory_model_contract_version": "0.2",
        "derived_keys": [
            {
                "name": "Edge",
                "join": "|",
                "inputs": [
                    {"name": "id"},
                    {"name": "optional"},
                    {"name": "number", "type": "number"},
                    {"name": "skip"},
                ],
                "segments": [
                    {"prefix": "literal=", "literal": "top"},
                    {"prefix": "nested=", "value": {"literal": "nested"}},
                    {"prefix": "id=", "value": {"input": "id"}, "transforms": ["trim"]},
                    {"prefix": "optional=", "value": {"input": "optional"}, "optional": True},
                    {"prefix": "skip=", "value": {"input": "skip"}, "omit_when": {"values": ["gone"]}},
                    {"prefix": "number=", "value": {"input": "number"}},
                    {"prefix": "empty=", "value": {"literal": ""}, "omit_when": {"empty": True}},
                ],
                "fixtures": [
                    {"name": "bad", "input": {"id": "x", "number": 1, "skip": "gone"}, "expect": "wrong"}
                ],
            }
        ],
        "models": [{"name": "EdgeModel"}],
    }

    assert (
        evaluate_derived_key(contract, "Edge", {"id": "A/B", "number": Decimal("1.250"), "skip": "gone"})
        == "literal=top|nested=nested|id=A%2FB|number=1.250"
    )
    with pytest.raises(ValidationError, match="derived key not found"):
        evaluate_derived_key(contract, "Missing", {})
    with pytest.raises(ValidationError, match="expected"):
        verify_derived_key_fixtures(contract)

    missing_input_key = {
        "name": "MissingInput",
        "join": "",
        "inputs": [{"name": "id"}],
        "segments": [{"value": {"input": "id"}}],
    }
    with pytest.raises(ValidationError, match="missing required input"):
        evaluate_derived_key_definition(missing_input_key, {})

    numeric_key = {
        "name": "Number",
        "join": "",
        "inputs": [{"name": "value", "type": "number"}],
        "segments": [{"value": {"input": "value"}}],
    }
    assert evaluate_derived_key_definition(numeric_key, {"value": "0.00"}) == "0"
    with pytest.raises(ValidationError, match="finite number"):
        evaluate_derived_key_definition(numeric_key, {"value": "not-a-number"})
    with pytest.raises(ValidationError, match="finite number"):
        evaluate_derived_key_definition(numeric_key, {"value": Decimal("NaN")})
    with pytest.raises(ValidationError, match="must not be null"):
        evaluate_derived_key_definition(numeric_key, {"value": None})  # type: ignore[dict-item]
    with pytest.raises(ValidationError, match="must be a scalar"):
        evaluate_derived_key_definition(numeric_key, {"value": object()})  # type: ignore[dict-item]
