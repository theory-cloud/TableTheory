from __future__ import annotations

import importlib.util
import sys
from dataclasses import dataclass
from pathlib import Path
from types import ModuleType

import pytest

from tabletheory_py import IndexSpec, ModelDefinition, theorydb_field
from tabletheory_py.model import ModelDefinitionError


@dataclass(frozen=True)
class User:
    pk: str = theorydb_field(name="PK", roles=["pk"])
    sk: str = theorydb_field(name="SK", roles=["sk"])
    gsi1pk: str = theorydb_field(name="GSI1PK", roles=["gsi1pk"])
    gsi1sk: str = theorydb_field(name="GSI1SK", roles=["gsi1sk"])
    email: str = theorydb_field()


def test_index_spec_uses_python_field_names() -> None:
    model = ModelDefinition.from_dataclass(
        User,
        table_name="users_contract",
        indexes=[IndexSpec(name="gsi_email", type="GSI", partition="gsi1pk", sort="gsi1sk")],
    )

    index = model.indexes[0]
    assert index.partition == "GSI1PK"
    assert index.sort == "GSI1SK"


def test_index_spec_rejects_storage_attribute_names() -> None:
    with pytest.raises(ModelDefinitionError, match="unknown partition field: GSI1PK"):
        ModelDefinition.from_dataclass(
            User,
            table_name="users_contract",
            indexes=[IndexSpec(name="gsi_email", type="GSI", partition="GSI1PK", sort="GSI1SK")],
        )


def _load_example() -> ModuleType:
    path = Path(__file__).resolve().parents[2] / "examples" / "gsi_query.py"
    spec = importlib.util.spec_from_file_location("gsi_query_example", path)
    assert spec is not None and spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


def test_gsi_query_example_runs_offline() -> None:
    module = _load_example()
    module.main()
