from __future__ import annotations

from dataclasses import dataclass

import pytest

from theorydb_py import (
    ModelDefinition,
    Projection,
    ValidationError,
    assert_model_definition_equivalent_to_dms,
    theorydb_field,
    get_dms_model,
    gsi,
    lsi,
    parse_dms_document,
)
from theorydb_py.dms import _model_definition_to_dms_model


@dataclass(frozen=True)
class _Demo:
    pk: str = theorydb_field(name="PK", roles=["pk"])
    sk: str = theorydb_field(name="SK", roles=["sk"])
    value: str = theorydb_field(name="value", omitempty=True, default="")
    secret: str = theorydb_field(name="secret", encrypted=True, omitempty=True, default="")


def test_parse_dms_document_and_get_model() -> None:
    raw = """
dms_version: "0.1"
namespace: "theorydb.test"
models:
  - name: "Demo"
    table: { name: "tbl" }
    keys:
      partition: { attribute: "PK", type: "S" }
      sort: { attribute: "SK", type: "S" }
    attributes:
      - attribute: "PK"
        type: "S"
        required: true
        roles: ["pk"]
      - attribute: "SK"
        type: "S"
        required: true
        roles: ["sk"]
      - attribute: "value"
        type: "S"
        optional: true
        omit_empty: true
      - attribute: "secret"
        type: "S"
        optional: true
        omit_empty: true
        encryption: { v: 1 }
"""
    doc = parse_dms_document(raw)
    model = get_dms_model(doc, "Demo")
    assert model["name"] == "Demo"


def test_parse_dms_document_rejects_unsupported_version() -> None:
    with pytest.raises(ValidationError):
        parse_dms_document('dms_version: "9.9"\nmodels: []\n')


def test_parse_dms_document_rejects_invalid_yaml() -> None:
    with pytest.raises(ValidationError):
        parse_dms_document("dms_version: [")


def test_parse_dms_document_rejects_non_object_root() -> None:
    with pytest.raises(ValidationError):
        parse_dms_document("- 1\n- 2\n")


@pytest.mark.parametrize(
    "extra",
    [
        "timestamp: 2025-01-01T00:00:00Z\n",
        "bad: .nan\n",
        "bad: .inf\n",
    ],
)
def test_parse_dms_document_rejects_non_json_values(extra: str) -> None:
    raw = (
        'dms_version: "0.1"\n'
        "models:\n"
        '  - name: "Demo"\n'
        '    table: { name: "tbl" }\n'
        "    keys:\n"
        '      partition: { attribute: "PK", type: "S" }\n'
        "    attributes:\n"
        '      - attribute: "PK"\n'
        '        type: "S"\n'
        "        required: true\n"
        f"{extra}"
    )
    with pytest.raises(ValidationError):
        parse_dms_document(raw)


def test_get_dms_model_errors() -> None:
    with pytest.raises(ValidationError):
        get_dms_model({}, "Demo")
    with pytest.raises(ValidationError):
        get_dms_model({"models": []}, "Demo")
    with pytest.raises(ValidationError):
        get_dms_model({"models": [{"name": "Other"}]}, "Demo")


def test_model_definition_equivalence_to_dms_ignoring_table_name() -> None:
    raw = """
dms_version: "0.1"
models:
  - name: "_Demo"
    table: { name: "ignored" }
    keys:
      partition: { attribute: "PK", type: "S" }
      sort: { attribute: "SK", type: "S" }
    attributes:
      - attribute: "PK"
        type: "S"
        required: true
        roles: ["pk"]
      - attribute: "SK"
        type: "S"
        required: true
        roles: ["sk"]
      - attribute: "value"
        type: "S"
        optional: true
        omit_empty: true
      - attribute: "secret"
        type: "S"
        optional: true
        omit_empty: true
        encryption: { v: 1 }
"""
    doc = parse_dms_document(raw)
    dms_model = get_dms_model(doc, "_Demo")
    model = ModelDefinition.from_dataclass(_Demo, table_name="tbl")
    assert_model_definition_equivalent_to_dms(model, dms_model, ignore_table_name=True)


def test_model_definition_equivalence_detects_drift() -> None:
    raw = """
dms_version: "0.1"
models:
  - name: "_Demo"
    table: { name: "ignored" }
    keys:
      partition: { attribute: "PK", type: "S" }
      sort: { attribute: "SK", type: "S" }
    attributes:
      - attribute: "PK"
        type: "S"
        required: true
        roles: ["pk"]
      - attribute: "SK"
        type: "S"
        required: true
        roles: ["sk"]
      - attribute: "value"
        type: "S"
        optional: true
        omit_empty: true
"""
    doc = parse_dms_document(raw)
    dms_model = get_dms_model(doc, "_Demo")
    model = ModelDefinition.from_dataclass(_Demo, table_name="tbl")
    with pytest.raises(ValidationError):
        assert_model_definition_equivalent_to_dms(model, dms_model, ignore_table_name=True)


@dataclass(frozen=True)
class _Complex:
    pk: str = theorydb_field(name="PK", roles=["pk"])
    sk: str = theorydb_field(name="SK", roles=["sk"])
    email: str = theorydb_field(name="email")
    ts: int = theorydb_field(name="ts")
    tags: set[str] = theorydb_field(name="tags", set_=True, default_factory=set)
    scores: set[int] = theorydb_field(name="scores", set_=True, default_factory=set)
    blobs: set[bytes] = theorydb_field(name="blobs", set_=True, default_factory=set)
    blob: bytes = theorydb_field(name="blob", omitempty=True, default=b"")
    flags: list[str] = theorydb_field(name="flags", default_factory=list)
    meta: dict[str, int] = theorydb_field(name="meta", default_factory=dict)
    ok: bool = theorydb_field(name="ok", default=True)
    note: str | None = theorydb_field(name="note", omitempty=True, default=None)
    secret: str = theorydb_field(name="secret", encrypted=True, omitempty=True, default="")


def test_model_definition_equivalence_with_indexes_and_types() -> None:
    raw = """
dms_version: "0.1"
models:
  - name: "_Complex"
    table: { name: "tbl" }
    keys:
      partition: { attribute: "PK", type: "S" }
      sort: { attribute: "SK", type: "S" }
    attributes:
      - attribute: "PK"
        type: "S"
        required: true
        roles: ["pk"]
      - attribute: "SK"
        type: "S"
        required: true
        roles: ["sk"]
      - attribute: "blobs"
        type: "BS"
        optional: true
      - attribute: "blob"
        type: "B"
        optional: true
        omit_empty: true
      - attribute: "email"
        type: "S"
        required: true
      - attribute: "flags"
        type: "L"
        optional: true
      - attribute: "meta"
        type: "M"
        optional: true
      - attribute: "note"
        type: "S"
        optional: true
        omit_empty: true
      - attribute: "ok"
        type: "BOOL"
        optional: true
      - attribute: "scores"
        type: "NS"
        optional: true
      - attribute: "secret"
        type: "S"
        optional: true
        omit_empty: true
        encryption: { v: 1 }
      - attribute: "tags"
        type: "SS"
        optional: true
      - attribute: "ts"
        type: "N"
        required: true
    indexes:
      - name: "gsi-email"
        type: "GSI"
        partition: { attribute: "email", type: "S" }
        sort: { attribute: "ts", type: "N" }
        projection: { type: "INCLUDE", fields: ["meta", "flags"] }
      - name: "lsi-ts"
        type: "LSI"
        partition: { attribute: "PK", type: "S" }
        sort: { attribute: "ts", type: "N" }
        projection: { type: "KEYS_ONLY", fields: [] }
"""
    doc = parse_dms_document(raw)
    dms_model = get_dms_model(doc, "_Complex")

    model = ModelDefinition.from_dataclass(
        _Complex,
        table_name="tbl",
        indexes=[
            gsi("gsi-email", partition="email", sort="ts", projection=Projection.include("meta", "flags")),
            lsi("lsi-ts", sort="ts", projection=Projection.keys_only()),
        ],
    )
    assert_model_definition_equivalent_to_dms(model, dms_model, ignore_table_name=False)


def test_ignore_table_name_allows_dms_without_table() -> None:
    raw = """
dms_version: "0.1"
models:
  - name: "_Demo"
    keys:
      partition: { attribute: "PK", type: "S" }
      sort: { attribute: "SK", type: "S" }
    attributes:
      - attribute: "PK"
        type: "S"
        required: true
        roles: ["pk"]
      - attribute: "SK"
        type: "S"
        required: true
        roles: ["sk"]
      - attribute: "value"
        type: "S"
        optional: true
        omit_empty: true
      - attribute: "secret"
        type: "S"
        optional: true
        omit_empty: true
        encryption: { v: 1 }
"""
    doc = parse_dms_document(raw)
    dms_model = get_dms_model(doc, "_Demo")
    model = ModelDefinition.from_dataclass(_Demo, table_name="tbl")
    assert_model_definition_equivalent_to_dms(model, dms_model, ignore_table_name=True)


@dataclass(frozen=True)
class _BadKeyType:
    pk: list[str] = theorydb_field(name="PK", roles=["pk"])


def test_model_definition_to_dms_rejects_non_scalar_key_type() -> None:
    model = ModelDefinition.from_dataclass(_BadKeyType, table_name="tbl")
    with pytest.raises(ValidationError, match="key attribute must be S/N/B"):
        _model_definition_to_dms_model(model)


@dataclass(frozen=True)
class _BadSetElement:
    pk: str = theorydb_field(name="PK", roles=["pk"])
    things: set[object] = theorydb_field(name="things", set_=True, default_factory=set)


def test_model_definition_to_dms_rejects_unsupported_set_element_type() -> None:
    model = ModelDefinition.from_dataclass(_BadSetElement, table_name="tbl")
    with pytest.raises(ValidationError, match="unsupported set element type"):
        _model_definition_to_dms_model(model)
