from __future__ import annotations

import importlib.util
import os
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path
from types import ModuleType
from typing import cast

import pytest

from theorydb_py import (
    ModelDefinition,
    ModelDefinitionError,
    Projection,
    ValidationError,
    WritePolicy,
    assert_model_definition_equivalent_to_dms,
    assert_models_equivalent,
    get_dms_model,
    gsi,
    lsi,
    model_definition_to_dms_model,
    parse_dms_document,
    theorydb_field,
)
from theorydb_py.dms import _model_definition_to_dms_model


@dataclass(frozen=True)
class _Demo:
    pk: str = theorydb_field(name="PK", roles=["pk"])
    sk: str = theorydb_field(name="SK", roles=["sk"])
    value: str = theorydb_field(name="value", omitempty=True, default="")
    secret: str = theorydb_field(name="secret", encrypted=True, omitempty=True, default="")


@dataclass(frozen=True)
class _JsonDoc:
    pk: str = theorydb_field(name="PK", roles=["pk"])
    payload: dict[str, int] = theorydb_field(name="payload", json=True, default_factory=dict)
    response: str = theorydb_field(name="response", json=True, default="")


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
    write_policy:
      mode: "write_once"
      protected_attributes: ["value"]
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
    normalized = _model_definition_to_dms_model(
        ModelDefinition.from_dataclass(
            _Demo, table_name="tbl", write_policy=WritePolicy("write_once", ("value",))
        )
    )
    assert normalized["write_policy"] == {"mode": "write_once", "protected_attributes": ["value"]}


def test_parse_dms_document_rejects_unsupported_version() -> None:
    with pytest.raises(ValidationError):
        parse_dms_document('dms_version: "9.9"\nmodels: []\n')


def test_parse_dms_document_rejects_unsupported_naming_convention() -> None:
    raw = """
dms_version: "0.1"
models:
  - name: "BadNaming"
    table: { name: "tbl" }
    naming: { convention: "pascalCase" }
    keys:
      partition: { attribute: "PK", type: "S" }
    attributes:
      - attribute: "PK"
        type: "S"
        required: true
        roles: ["pk"]
"""
    with pytest.raises(ValidationError, match="unsupported naming\\.convention"):
        parse_dms_document(raw)


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


def test_parse_dms_document_accepts_native_json_types() -> None:
    raw = """
dms_version: "0.1"
models:
  - name: "_JsonDoc"
    table: { name: "tbl" }
    keys:
      partition: { attribute: "PK", type: "S" }
    attributes:
      - attribute: "PK"
        type: "S"
        required: true
        roles: ["pk"]
      - attribute: "payload"
        type: "M"
        optional: true
        json: true
      - attribute: "response"
        type: "S"
        optional: true
        json: true
"""
    doc = parse_dms_document(raw)
    model = get_dms_model(doc, "_JsonDoc")
    attrs = {attr["attribute"]: attr for attr in model["attributes"]}
    assert attrs["payload"]["type"] == "M"
    assert attrs["payload"]["json"] is True
    assert attrs["response"]["type"] == "S"
    assert attrs["response"]["json"] is True


def test_parse_dms_document_rejects_json_set_storage_types() -> None:
    raw = """
dms_version: "0.1"
models:
  - name: "_BadJson"
    table: { name: "tbl" }
    keys:
      partition: { attribute: "PK", type: "S" }
    attributes:
      - attribute: "PK"
        type: "S"
        required: true
        roles: ["pk"]
      - attribute: "payload"
        type: "SS"
        optional: true
        json: true
"""
    with pytest.raises(ValidationError, match="json fields must use S/N/BOOL/NULL/L/M"):
        parse_dms_document(raw)


def test_parse_dms_document_rejects_unknown_protected_attribute() -> None:
    raw = """
dms_version: "0.1"
models:
  - name: "_BadPolicy"
    table: { name: "tbl" }
    keys:
      partition: { attribute: "PK", type: "S" }
    write_policy:
      mode: "mutable"
      protected_attributes: ["missing"]
    attributes:
      - attribute: "PK"
        type: "S"
        required: true
        roles: ["pk"]
"""
    with pytest.raises(ValidationError, match="protected attribute not found"):
        parse_dms_document(raw)


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


def test_model_definition_to_dms_model_uses_json_storage_type() -> None:
    model = ModelDefinition.from_dataclass(_JsonDoc, table_name="tbl")
    dms_model = model_definition_to_dms_model(model)
    attrs = {attr["attribute"]: attr for attr in dms_model["attributes"]}
    assert attrs["payload"]["type"] == "M"
    assert attrs["payload"]["json"] is True
    assert attrs["response"]["type"] == "S"
    assert attrs["response"]["json"] is True


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


def test_generated_python_fixture_equivalent_to_dms_source() -> None:
    root = Path(__file__).resolve().parents[3]
    raw = (root / "pkg" / "dms" / "testdata" / "codegen" / "dms-note.yml").read_text(encoding="utf-8")
    doc = parse_dms_document(raw)
    dms_model = get_dms_model(doc, "DMSNote")

    generated = _load_generated_dms_note()
    definition = cast(ModelDefinition[object], vars(generated)["DMSNoteDefinition"])
    assert_models_equivalent(definition, dms_model)
    assert model_definition_to_dms_model(definition)["name"] == "DMSNote"


def test_cli_generated_python_imports_when_required_field_follows_optional(tmp_path: Path) -> None:
    root = Path(__file__).resolve().parents[3]
    dms_source = """
dms_version: "0.1"
models:
  - name: "RequiredAfterOptional"
    table: { name: "required_after_optional" }
    keys:
      partition: { attribute: "PK", type: "S" }
    attributes:
      - attribute: "PK"
        type: "S"
        required: true
        roles: ["pk"]
      - attribute: "optionalTitle"
        type: "S"
        optional: true
        omit_empty: true
      - attribute: "requiredStatus"
        type: "S"
        required: true
"""
    dms_path = tmp_path / "required_after_optional.yml"
    generated_path = tmp_path / "required_after_optional.py"
    dms_path.write_text(dms_source, encoding="utf-8")

    env = _repo_env(root)
    result = subprocess.run(
        [
            "go",
            "run",
            "./cmd/tabletheory",
            "gen",
            "--lang",
            "py",
            "--out",
            str(generated_path),
            str(dms_path),
        ],
        cwd=root,
        env=env,
        check=False,
        capture_output=True,
        text=True,
    )
    assert result.returncode == 0, result.stderr

    generated_source = generated_path.read_text(encoding="utf-8")
    assert generated_source.index("required_status") < generated_source.index("optional_title")

    typecheck = subprocess.run(
        [
            sys.executable,
            "-m",
            "mypy",
            "--strict",
            "--cache-dir",
            str(tmp_path / "mypy-cache"),
            str(generated_path),
        ],
        cwd=root / "py",
        env=env,
        check=False,
        capture_output=True,
        text=True,
    )
    assert typecheck.returncode == 0, typecheck.stdout + typecheck.stderr

    generated = _load_python_module(generated_path, "generated_required_after_optional")
    definition = cast(ModelDefinition[object], vars(generated)["RequiredAfterOptionalDefinition"])
    doc = parse_dms_document(dms_source)
    assert_models_equivalent(definition, get_dms_model(doc, "RequiredAfterOptional"))


def test_assert_models_equivalent_accepts_mapping_and_detects_drift() -> None:
    root = Path(__file__).resolve().parents[3]
    raw = (root / "pkg" / "dms" / "testdata" / "codegen" / "dms-note.yml").read_text(encoding="utf-8")
    doc = parse_dms_document(raw)
    dms_model = get_dms_model(doc, "DMSNote")
    drifted = dict(dms_model)
    drifted["write_policy"] = {"mode": "mutable", "protected_attributes": ["count"]}

    with pytest.raises(ValidationError, match="models not equivalent"):
        assert_models_equivalent(drifted, dms_model)


def _load_generated_dms_note() -> ModuleType:
    root = Path(__file__).resolve().parents[2]
    fixture = root / "tests" / "fixtures" / "dms_codegen" / "generated_dms_note.py"
    return _load_python_module(fixture, "generated_dms_note")


def _load_python_module(path: Path, module_name: str) -> ModuleType:
    spec = importlib.util.spec_from_file_location(module_name, path)
    assert spec is not None and spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


def _repo_env(root: Path) -> dict[str, str]:
    env = os.environ.copy()
    py_src = str(root / "py" / "src")
    env["PYTHONPATH"] = py_src + os.pathsep + env["PYTHONPATH"] if env.get("PYTHONPATH") else py_src
    env.setdefault("GOTOOLCHAIN", _go_toolchain(root))
    return env


def _go_toolchain(root: Path) -> str:
    for line in (root / "go.mod").read_text(encoding="utf-8").splitlines():
        if line.startswith("toolchain "):
            return line.split(maxsplit=1)[1]
    return "auto"


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
    with pytest.raises(ModelDefinitionError, match="unsupported set element type"):
        ModelDefinition.from_dataclass(_BadSetElement, table_name="tbl")
