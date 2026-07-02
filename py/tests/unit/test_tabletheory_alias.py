from __future__ import annotations

import tabletheory_py
import theorydb_py
from tabletheory_py.errors import ValidationError
from tabletheory_py.mocks import FakeDynamoDBClient


def test_tabletheory_py_reexports_public_surface() -> None:
    assert tabletheory_py.__version__ == theorydb_py.__version__
    assert tabletheory_py.__repo_version__ == theorydb_py.__repo_version__
    assert tabletheory_py.ModelDefinition is theorydb_py.ModelDefinition
    assert tabletheory_py.Table is theorydb_py.Table
    assert tabletheory_py.theorydb_field is theorydb_py.theorydb_field
    assert tabletheory_py.SortKeyCondition is theorydb_py.SortKeyCondition
    assert tabletheory_py.__all__ == theorydb_py.__all__


def test_tabletheory_py_submodule_aliases() -> None:
    assert ValidationError is theorydb_py.ValidationError
    assert FakeDynamoDBClient.__name__ == "FakeDynamoDBClient"
