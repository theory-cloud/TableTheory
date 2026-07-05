from __future__ import annotations

import importlib
import importlib.util
import json
import re
import sys
from importlib.resources import files

import tabletheory_py
from tabletheory_py.errors import ValidationError
from tabletheory_py.mocks import FakeDynamoDBClient
from tabletheory_py.model import ModelDefinition, theorydb_field
from tabletheory_py.query import SortKeyCondition
from tabletheory_py.table import Table


def _clear_theorydb_py_modules() -> None:
    for name in list(sys.modules):
        if name == "theorydb_py" or name.startswith("theorydb_py."):
            sys.modules.pop(name, None)


def _repo_version_from_package_data() -> str:
    data = json.loads(files(tabletheory_py).joinpath("version.json").read_text(encoding="utf-8"))
    version = data.get("version")
    assert isinstance(version, str)
    return version


def _normalized_version(repo_version: str) -> str:
    match = re.match(r"^(\d+\.\d+\.\d+)-rc\.?([0-9]+)$", repo_version)
    if match:
        return f"{match.group(1)}rc{match.group(2)}"
    return repo_version


def test_tabletheory_py_reexports_public_surface() -> None:
    repo_version = _repo_version_from_package_data()

    assert tabletheory_py.__repo_version__ == repo_version
    assert tabletheory_py.__version__ == _normalized_version(repo_version)
    assert tabletheory_py.ModelDefinition is ModelDefinition
    assert tabletheory_py.Table is Table
    assert tabletheory_py.theorydb_field is theorydb_field
    assert tabletheory_py.SortKeyCondition is SortKeyCondition
    assert {"__repo_version__", "__version__", "ModelDefinition", "Table"}.issubset(tabletheory_py.__all__)
    assert len(tabletheory_py.__all__) == len(set(tabletheory_py.__all__))


def test_tabletheory_py_submodule_aliases() -> None:
    assert ValidationError is tabletheory_py.ValidationError
    assert FakeDynamoDBClient.__name__ == "FakeDynamoDBClient"


def test_theorydb_py_import_path_is_removed() -> None:
    _clear_theorydb_py_modules()

    assert importlib.util.find_spec("theorydb_py") is None
    try:
        importlib.import_module("theorydb_py")
    except ModuleNotFoundError as exc:
        assert exc.name == "theorydb_py"
    else:  # pragma: no cover - defensive assertion message for stale local installs
        raise AssertionError("legacy theorydb_py import path unexpectedly resolved")
