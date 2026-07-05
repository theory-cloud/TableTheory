from __future__ import annotations

import importlib
import sys
import warnings

import tabletheory_py
from tabletheory_py.errors import ValidationError
from tabletheory_py.mocks import FakeDynamoDBClient


def test_tabletheory_py_reexports_public_surface() -> None:
    assert tabletheory_py.__version__ == tabletheory_py.__version__
    assert tabletheory_py.__repo_version__ == tabletheory_py.__repo_version__
    assert tabletheory_py.ModelDefinition is tabletheory_py.ModelDefinition
    assert tabletheory_py.Table is tabletheory_py.Table
    assert tabletheory_py.theorydb_field is tabletheory_py.theorydb_field
    assert tabletheory_py.SortKeyCondition is tabletheory_py.SortKeyCondition
    assert tabletheory_py.__all__ == tabletheory_py.__all__


def test_tabletheory_py_submodule_aliases() -> None:
    assert ValidationError is tabletheory_py.ValidationError
    assert FakeDynamoDBClient.__name__ == "FakeDynamoDBClient"


def test_theorydb_py_import_is_warning_shim() -> None:
    sys.modules.pop("theorydb_py", None)

    with warnings.catch_warnings(record=True) as caught:
        warnings.simplefilter("always", DeprecationWarning)
        legacy = importlib.import_module("theorydb_py")

    assert legacy.Table is tabletheory_py.Table
    assert legacy.__version__ == tabletheory_py.__version__
    assert any("import tabletheory_py instead" in str(warning.message) for warning in caught)
