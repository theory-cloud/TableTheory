from __future__ import annotations

from collections.abc import Sequence
from typing import TYPE_CHECKING, Any

from botocore.exceptions import ClientError

from .aws_errors import map_client_error as _map_client_error
from .errors import ValidationError

if TYPE_CHECKING:
    from .table import Table


def transact_get(
    table: Table[Any],
    keys: Sequence[Any],
    *,
    projection: list[str] | None = None,
) -> list[Any | None]:
    if not keys:
        raise ValidationError("transact_get keys are required")
    if len(keys) > 100:
        raise ValidationError("DynamoDB TransactGetItems supports at most 100 items")

    transact_items: list[dict[str, Any]] = []
    names: dict[str, str] | None = None
    projection_expression: str | None = None
    if projection is not None:
        names = {}
        projection_expression = table._projection_expression(projection, names)

    for key in keys:
        pk, sk = _normalize_key(table, key)
        get: dict[str, Any] = {"TableName": table._table_name, "Key": table._to_key(pk, sk)}
        if projection_expression is not None:
            get["ProjectionExpression"] = projection_expression
            if names:
                get["ExpressionAttributeNames"] = names
        transact_items.append({"Get": get})

    try:
        resp = table._client.transact_get_items(TransactItems=transact_items)
    except ClientError as err:  # pragma: no cover
        raise _map_client_error(err) from err

    out: list[Any | None] = []
    for response in resp.get("Responses", []):
        item = response.get("Item")
        out.append(table._from_item(item) if item else None)
    while len(out) < len(keys):
        out.append(None)
    return out


def _normalize_key(table: Table[Any], key: Any) -> tuple[Any, Any | None]:
    if table._model.sk is None:
        if isinstance(key, tuple):
            if len(key) != 2:
                raise ValidationError("expected key tuple (pk, None) for pk-only models")
            pk, sk = key
            if sk is not None:
                raise ValidationError("sk must be None for pk-only models")
            return pk, None
        return key, None

    if not isinstance(key, tuple) or len(key) != 2:
        raise ValidationError("expected key tuple (pk, sk)")
    pk, sk = key
    return pk, sk
