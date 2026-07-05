from __future__ import annotations

import importlib
import json
import re
import sys
import warnings
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


def test_theorydb_py_import_is_warning_shim() -> None:
    _clear_theorydb_py_modules()

    with warnings.catch_warnings(record=True) as caught:
        warnings.simplefilter("always", DeprecationWarning)
        legacy = importlib.import_module("theorydb_py")

    assert legacy.Table is tabletheory_py.Table
    assert legacy.__version__ == tabletheory_py.__version__
    assert any("import tabletheory_py instead" in str(warning.message) for warning in caught)


def test_theorydb_py_legacy_submodules_reexport_public_symbols() -> None:
    _clear_theorydb_py_modules()

    from tabletheory_py.aggregates import AggregateResult as CanonAggregateResult
    from tabletheory_py.aggregates import aggregate_field as canon_aggregate_field
    from tabletheory_py.attr_types import JSON_STORAGE_TYPES as canon_json_storage_types
    from tabletheory_py.attr_types import infer_storage_type as canon_infer_storage_type
    from tabletheory_py.aws_errors import map_client_error as canon_map_client_error
    from tabletheory_py.dms import parse_dms_document as canon_parse_dms_document
    from tabletheory_py.encryption import marshal_attribute_value_json as canon_marshal_attribute_value_json
    from tabletheory_py.errors import AwsError as CanonAwsError
    from tabletheory_py.errors import ValidationError as CanonValidationError
    from tabletheory_py.fakedb import Item as CanonItem
    from tabletheory_py.fakedb import StatefulDynamoDBClient as CanonStatefulDynamoDBClient
    from tabletheory_py.key_contract import TRANSFORM_TRIM as CANON_TRANSFORM_TRIM
    from tabletheory_py.key_contract import parse_derived_key_contract as canon_parse_derived_key_contract

    with warnings.catch_warnings():
        warnings.simplefilter("ignore", DeprecationWarning)
        from theorydb_py.aggregates import AggregateResult, aggregate_field
        from theorydb_py.attr_types import JSON_STORAGE_TYPES, infer_storage_type
        from theorydb_py.aws_errors import map_client_error
        from theorydb_py.dms import parse_dms_document
        from theorydb_py.encryption import marshal_attribute_value_json
        from theorydb_py.errors import AwsError, ValidationError
        from theorydb_py.fakedb import Item, StatefulDynamoDBClient
        from theorydb_py.key_contract import TRANSFORM_TRIM, parse_derived_key_contract

    assert AggregateResult is CanonAggregateResult
    assert aggregate_field is canon_aggregate_field
    assert JSON_STORAGE_TYPES is canon_json_storage_types
    assert infer_storage_type is canon_infer_storage_type
    assert map_client_error is canon_map_client_error
    assert parse_dms_document is canon_parse_dms_document
    assert marshal_attribute_value_json is canon_marshal_attribute_value_json
    assert AwsError is CanonAwsError
    assert ValidationError is CanonValidationError
    assert Item is CanonItem
    assert StatefulDynamoDBClient is CanonStatefulDynamoDBClient
    assert TRANSFORM_TRIM is CANON_TRANSFORM_TRIM
    assert parse_derived_key_contract is canon_parse_derived_key_contract
