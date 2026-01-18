from __future__ import annotations

import time
from collections.abc import Callable
from dataclasses import is_dataclass
from decimal import Decimal
from typing import Any, Union, get_args, get_origin, get_type_hints

import boto3
from botocore.exceptions import ClientError

from .errors import AwsError, NotFoundError, ValidationError
from .model import ModelDefinition

BillingMode = str  # "PAY_PER_REQUEST" | "PROVISIONED"


def create_table(
    model: ModelDefinition[Any],
    *,
    client: Any | None = None,
    table_name: str | None = None,
    billing_mode: BillingMode = "PAY_PER_REQUEST",
    provisioned_throughput: dict[str, int] | None = None,
    wait_for_active: bool = True,
    wait_timeout_seconds: float = 300.0,
    poll_interval_seconds: float = 0.25,
    sleep: Callable[[float], None] = time.sleep,
) -> None:
    if client is None:
        client = boto3.client("dynamodb")

    req = build_create_table_request(
        model,
        table_name=table_name,
        billing_mode=billing_mode,
        provisioned_throughput=provisioned_throughput,
    )

    try:
        client.create_table(**req)
    except ClientError as err:
        code = str(err.response.get("Error", {}).get("Code", ""))
        if code != "ResourceInUseException":
            raise _map_schema_error(err) from err

    if wait_for_active:
        _wait_for_table_active(
            client,
            req["TableName"],
            timeout_seconds=wait_timeout_seconds,
            poll_interval_seconds=poll_interval_seconds,
            sleep=sleep,
        )


def ensure_table(
    model: ModelDefinition[Any],
    *,
    client: Any | None = None,
    table_name: str | None = None,
    billing_mode: BillingMode = "PAY_PER_REQUEST",
    provisioned_throughput: dict[str, int] | None = None,
    wait_for_active: bool = True,
    wait_timeout_seconds: float = 300.0,
    poll_interval_seconds: float = 0.25,
    sleep: Callable[[float], None] = time.sleep,
) -> None:
    if client is None:
        client = boto3.client("dynamodb")

    resolved_table = table_name or model.table_name
    if not resolved_table:
        raise ValueError("table_name is required (or set ModelDefinition.table_name)")

    try:
        client.describe_table(TableName=resolved_table)
    except ClientError as err:
        code = str(err.response.get("Error", {}).get("Code", ""))
        if code != "ResourceNotFoundException":
            raise _map_schema_error(err) from err
        create_table(
            model,
            client=client,
            table_name=resolved_table,
            billing_mode=billing_mode,
            provisioned_throughput=provisioned_throughput,
            wait_for_active=wait_for_active,
            wait_timeout_seconds=wait_timeout_seconds,
            poll_interval_seconds=poll_interval_seconds,
            sleep=sleep,
        )
        return

    if wait_for_active:
        _wait_for_table_active(
            client,
            resolved_table,
            timeout_seconds=wait_timeout_seconds,
            poll_interval_seconds=poll_interval_seconds,
            sleep=sleep,
        )


def delete_table(
    model: ModelDefinition[Any],
    *,
    client: Any | None = None,
    table_name: str | None = None,
    wait_for_delete: bool = True,
    wait_timeout_seconds: float = 300.0,
    poll_interval_seconds: float = 0.25,
    ignore_missing: bool = False,
    sleep: Callable[[float], None] = time.sleep,
) -> None:
    if client is None:
        client = boto3.client("dynamodb")

    resolved_table = table_name or model.table_name
    if not resolved_table:
        raise ValueError("table_name is required (or set ModelDefinition.table_name)")

    try:
        client.delete_table(TableName=resolved_table)
    except ClientError as err:
        code = str(err.response.get("Error", {}).get("Code", ""))
        if ignore_missing and code == "ResourceNotFoundException":
            return
        raise _map_schema_error(err) from err

    if wait_for_delete:
        _wait_for_table_deleted(
            client,
            resolved_table,
            timeout_seconds=wait_timeout_seconds,
            poll_interval_seconds=poll_interval_seconds,
            sleep=sleep,
        )


def describe_table(
    model: ModelDefinition[Any],
    *,
    client: Any | None = None,
    table_name: str | None = None,
) -> dict[str, Any]:
    if client is None:
        client = boto3.client("dynamodb")

    resolved_table = table_name or model.table_name
    if not resolved_table:
        raise ValueError("table_name is required (or set ModelDefinition.table_name)")

    try:
        return dict(client.describe_table(TableName=resolved_table))
    except ClientError as err:
        raise _map_schema_error(err) from err


def build_create_table_request(
    model: ModelDefinition[Any],
    *,
    table_name: str | None = None,
    billing_mode: BillingMode = "PAY_PER_REQUEST",
    provisioned_throughput: dict[str, int] | None = None,
) -> dict[str, Any]:
    if not is_dataclass(model.model_type):
        raise ValidationError("model_type must be a dataclass")

    resolved_table = table_name or model.table_name
    if not resolved_table:
        raise ValueError("table_name is required (or set ModelDefinition.table_name)")

    billing_mode = (billing_mode or "PAY_PER_REQUEST").strip() or "PAY_PER_REQUEST"
    if billing_mode not in {"PAY_PER_REQUEST", "PROVISIONED"}:
        raise ValidationError(f"unsupported billing_mode: {billing_mode}")

    resolved_throughput: dict[str, int] | None = None
    if billing_mode == "PROVISIONED":
        if provisioned_throughput is None:
            raise ValidationError("provisioned_throughput is required when billing_mode=PROVISIONED")
        resolved_throughput = provisioned_throughput

    pk_attr = model.pk.attribute_name
    sk_attr = model.sk.attribute_name if model.sk is not None else None

    key_schema = [{"AttributeName": pk_attr, "KeyType": "HASH"}]
    if sk_attr is not None:
        key_schema.append({"AttributeName": sk_attr, "KeyType": "RANGE"})

    attr_types: dict[str, str] = {pk_attr: _key_scalar_type_for_attribute(model, pk_attr)}
    if sk_attr is not None:
        attr_types[sk_attr] = _key_scalar_type_for_attribute(model, sk_attr)

    gsis: list[dict[str, Any]] = []
    lsis: list[dict[str, Any]] = []

    for idx in model.indexes:
        idx_pk = idx.partition
        idx_sk = idx.sort
        if idx.type == "LSI" and idx_pk != pk_attr:
            raise ValidationError(f"LSI partition key must match table partition key: {idx.name}")

        attr_types[idx_pk] = _key_scalar_type_for_attribute(model, idx_pk)
        if idx_sk is not None:
            attr_types[idx_sk] = _key_scalar_type_for_attribute(model, idx_sk)

        idx_key_schema = [{"AttributeName": idx_pk, "KeyType": "HASH"}]
        if idx_sk is not None:
            idx_key_schema.append({"AttributeName": idx_sk, "KeyType": "RANGE"})

        proj: dict[str, Any] = {"ProjectionType": idx.projection.type}
        if idx.projection.type == "INCLUDE" and idx.projection.fields:
            proj["NonKeyAttributes"] = list(idx.projection.fields)

        if idx.type == "GSI":
            gsi: dict[str, Any] = {
                "IndexName": idx.name,
                "KeySchema": idx_key_schema,
                "Projection": proj,
            }
            if resolved_throughput is not None:
                gsi["ProvisionedThroughput"] = dict(resolved_throughput)
            gsis.append(gsi)
        else:
            lsis.append(
                {
                    "IndexName": idx.name,
                    "KeySchema": idx_key_schema,
                    "Projection": proj,
                }
            )

    attribute_definitions = [
        {"AttributeName": name, "AttributeType": attr_types[name]} for name in sorted(attr_types.keys())
    ]

    req: dict[str, Any] = {
        "TableName": resolved_table,
        "BillingMode": billing_mode,
        "KeySchema": key_schema,
        "AttributeDefinitions": attribute_definitions,
    }
    if resolved_throughput is not None:
        req["ProvisionedThroughput"] = dict(resolved_throughput)
    if gsis:
        req["GlobalSecondaryIndexes"] = gsis
    if lsis:
        req["LocalSecondaryIndexes"] = lsis

    return req


def _wait_for_table_active(
    client: Any,
    table_name: str,
    *,
    timeout_seconds: float,
    poll_interval_seconds: float,
    sleep: Callable[[float], None],
) -> None:
    deadline = time.monotonic() + timeout_seconds
    while time.monotonic() < deadline:
        try:
            resp = client.describe_table(TableName=table_name)
        except ClientError as err:
            code = str(err.response.get("Error", {}).get("Code", ""))
            if code != "ResourceNotFoundException":
                raise _map_schema_error(err) from err
            resp = {}

        status = str(resp.get("Table", {}).get("TableStatus", ""))
        if status == "ACTIVE":
            return
        sleep(poll_interval_seconds)

    raise ValidationError(f"timed out waiting for table ACTIVE: {table_name}")


def _wait_for_table_deleted(
    client: Any,
    table_name: str,
    *,
    timeout_seconds: float,
    poll_interval_seconds: float,
    sleep: Callable[[float], None],
) -> None:
    deadline = time.monotonic() + timeout_seconds
    while time.monotonic() < deadline:
        try:
            client.describe_table(TableName=table_name)
        except ClientError as err:
            code = str(err.response.get("Error", {}).get("Code", ""))
            if code == "ResourceNotFoundException":
                return
            raise _map_schema_error(err) from err
        sleep(poll_interval_seconds)

    raise ValidationError(f"timed out waiting for table deletion: {table_name}")


def _key_scalar_type_for_attribute(model: ModelDefinition[Any], attribute_name: str) -> str:
    python_name = None
    attr_def = None
    for name, definition in model.attributes.items():
        if definition.attribute_name == attribute_name:
            python_name = name
            attr_def = definition
            break
    if python_name is None or attr_def is None:
        raise ValidationError(f"unknown attribute: {attribute_name}")

    try:
        annotation = get_type_hints(model.model_type, include_extras=True).get(python_name, Any)
    except Exception:
        annotation = getattr(model.model_type, "__annotations__", {}).get(python_name, Any)
    annotation = _unwrap_optional(annotation)

    if getattr(attr_def, "json", False):
        return "S"
    if getattr(attr_def, "binary", False) or annotation in {bytes, bytearray}:
        return "B"
    if annotation is str:
        return "S"
    if annotation in {int, float, Decimal}:
        return "N"

    raise ValidationError(f"key attribute must be S/N/B: {attribute_name} (got {annotation})")


def _unwrap_optional(annotation: Any) -> Any:
    origin = get_origin(annotation)
    if origin is not Union:
        return annotation
    args = get_args(annotation)
    if not args:
        return annotation
    non_none = [a for a in args if a is not type(None)]  # noqa: E721
    if len(args) == 2 and len(non_none) == 1:
        return non_none[0]
    return annotation


def _map_schema_error(err: ClientError) -> Exception:
    code = str(err.response.get("Error", {}).get("Code", ""))
    message = str(err.response.get("Error", {}).get("Message", ""))

    if code == "ResourceNotFoundException":
        return NotFoundError(message or "resource not found")
    if code == "ValidationException":
        return ValidationError(message or "validation failed")

    return AwsError(code=code or "UnknownError", message=message or str(err))
