from __future__ import annotations

import json
import re
from importlib.resources import files
from typing import TYPE_CHECKING, Any

from .errors import (
    AwsError,
    BatchRetryExceededError,
    ConditionFailedError,
    EncryptionNotConfiguredError,
    LeaseHeldError,
    LeaseNotOwnedError,
    NotFoundError,
    TheorydbPyError,
    TransactionCanceledError,
    ValidationError,
)
from .model import (
    AttributeConverter,
    IndexDefinition,
    IndexSpec,
    ModelDefinition,
    ModelDefinitionError,
    Projection,
    gsi,
    lsi,
    theorydb_field,
)
from .query import FilterCondition, FilterGroup, Page, SortKeyCondition
from .transaction import (
    TransactConditionCheck,
    TransactDelete,
    TransactPut,
    TransactUpdate,
    TransactWriteAction,
)

if TYPE_CHECKING:
    from .aggregates import (
        AggregateResult,
        GroupByQuery,
        GroupedResult,
        aggregate_field,
        average_field,
        count_distinct,
        group_by,
        max_field,
        min_field,
        sum_field,
    )
    from .dms import assert_model_definition_equivalent_to_dms, get_dms_model, parse_dms_document
    from .lease import Lease, LeaseKey, LeaseManager
    from .multiaccount import AccountConfig, MultiAccountSessions
    from .optimizer import QueryOptimizer, QueryPlan, QueryShape, ScanShape
    from .protection import ConcurrencyLimiter, SimpleLimiter
    from .runtime import (
        AwsCallMetric,
        create_lambda_boto3_config,
        get_lambda_boto3_client,
        get_lambda_dynamodb_client,
        get_lambda_kms_client,
        instrument_boto3_client,
        is_lambda_environment,
    )
    from .schema import build_create_table_request, create_table, delete_table, describe_table, ensure_table
    from .streams import unmarshal_stream_image, unmarshal_stream_record
    from .table import Table
    from .validation import (
        MaxExpressionLength,
        MaxFieldNameLength,
        MaxNestedDepth,
        MaxOperatorLength,
        MaxValueStringLength,
        SecurityValidationError,
        validate_expression,
        validate_field_name,
        validate_index_name,
        validate_operator,
        validate_table_name,
        validate_value,
    )


def _read_repo_version() -> str:
    try:
        data = json.loads(files(__package__).joinpath("version.json").read_text(encoding="utf-8"))
    except Exception:
        return "0.0.0"

    version = data.get("version")
    return version if isinstance(version, str) and version else "0.0.0"


def _normalize_repo_version(repo_version: str) -> str:
    match = re.match(r"^(\d+\.\d+\.\d+)-rc\.?([0-9]+)$", repo_version)
    if match:
        return f"{match.group(1)}rc{match.group(2)}"
    return repo_version


__repo_version__ = _read_repo_version()
__version__ = _normalize_repo_version(__repo_version__)


def __getattr__(name: str) -> Any:
    if name in {"parse_dms_document", "get_dms_model", "assert_model_definition_equivalent_to_dms"}:
        from . import dms

        return getattr(dms, name)
    if name in {
        "build_create_table_request",
        "create_table",
        "delete_table",
        "describe_table",
        "ensure_table",
    }:
        from . import schema

        return getattr(schema, name)
    if name == "Table":
        from .table import Table

        return Table
    if name in {
        "AggregateResult",
        "GroupByQuery",
        "GroupedResult",
        "aggregate_field",
        "average_field",
        "count_distinct",
        "group_by",
        "max_field",
        "min_field",
        "sum_field",
    }:
        from . import aggregates

        return getattr(aggregates, name)
    if name in {"QueryOptimizer", "QueryPlan", "QueryShape", "ScanShape"}:
        from . import optimizer

        return getattr(optimizer, name)
    if name == "unmarshal_stream_image":
        from .streams import unmarshal_stream_image

        return unmarshal_stream_image
    if name == "unmarshal_stream_record":
        from .streams import unmarshal_stream_record

        return unmarshal_stream_record
    if name in {
        "AwsCallMetric",
        "create_lambda_boto3_config",
        "get_lambda_boto3_client",
        "get_lambda_dynamodb_client",
        "get_lambda_kms_client",
        "instrument_boto3_client",
        "is_lambda_environment",
    }:
        from . import runtime

        return getattr(runtime, name)
    if name in {"AccountConfig", "MultiAccountSessions"}:
        from . import multiaccount

        return getattr(multiaccount, name)
    if name in {
        "ConcurrencyLimiter",
        "SimpleLimiter",
    }:
        from . import protection

        return getattr(protection, name)
    if name in {
        "MaxExpressionLength",
        "MaxFieldNameLength",
        "MaxNestedDepth",
        "MaxOperatorLength",
        "MaxValueStringLength",
        "SecurityValidationError",
        "validate_expression",
        "validate_field_name",
        "validate_index_name",
        "validate_operator",
        "validate_table_name",
        "validate_value",
    }:
        from . import validation

        return getattr(validation, name)
    if name in {"Lease", "LeaseKey", "LeaseManager"}:
        from . import lease

        return getattr(lease, name)
    raise AttributeError(name)


__all__ = [
    "AwsError",
    "AwsCallMetric",
    "assert_model_definition_equivalent_to_dms",
    "AggregateResult",
    "aggregate_field",
    "AttributeConverter",
    "average_field",
    "BatchRetryExceededError",
    "build_create_table_request",
    "ConditionFailedError",
    "LeaseHeldError",
    "LeaseNotOwnedError",
    "count_distinct",
    "create_lambda_boto3_config",
    "create_table",
    "delete_table",
    "TheorydbPyError",
    "describe_table",
    "group_by",
    "GroupByQuery",
    "GroupedResult",
    "EncryptionNotConfiguredError",
    "ensure_table",
    "get_dms_model",
    "get_lambda_boto3_client",
    "get_lambda_dynamodb_client",
    "get_lambda_kms_client",
    "IndexDefinition",
    "IndexSpec",
    "ModelDefinition",
    "ModelDefinitionError",
    "NotFoundError",
    "Projection",
    "FilterCondition",
    "FilterGroup",
    "ConcurrencyLimiter",
    "Lease",
    "LeaseKey",
    "LeaseManager",
    "MaxExpressionLength",
    "MaxFieldNameLength",
    "MaxNestedDepth",
    "MaxOperatorLength",
    "MaxValueStringLength",
    "instrument_boto3_client",
    "is_lambda_environment",
    "AccountConfig",
    "MultiAccountSessions",
    "Page",
    "QueryOptimizer",
    "QueryPlan",
    "QueryShape",
    "SecurityValidationError",
    "SimpleLimiter",
    "SortKeyCondition",
    "ScanShape",
    "sum_field",
    "max_field",
    "min_field",
    "TransactConditionCheck",
    "TransactDelete",
    "TransactPut",
    "TransactUpdate",
    "TransactWriteAction",
    "TransactionCanceledError",
    "Table",
    "ValidationError",
    "__repo_version__",
    "__version__",
    "theorydb_field",
    "gsi",
    "lsi",
    "parse_dms_document",
    "unmarshal_stream_image",
    "unmarshal_stream_record",
    "validate_expression",
    "validate_field_name",
    "validate_index_name",
    "validate_operator",
    "validate_table_name",
    "validate_value",
]
