from __future__ import annotations

from collections.abc import Mapping
from typing import Any

from botocore.exceptions import ClientError

from .errors import ValidationError
from .model import AttributeDefinition, ModelDefinition
from .table import Table, _map_transaction_error
from .transaction import UpdateAdd


def transition_release_state(
    actual_table: Table[Any],
    event_table: Table[Any],
    *,
    actual_key: Mapping[str, Any],
    set_values: Mapping[str, Any],
    event_item: Any,
    expected_version: int | None = None,
    version_field: str = "version",
) -> None:
    """Transactionally update a release-state actual row and append an event.

    The two DynamoDB rows are written through a single TransactWriteItems call
    when both Table instances share the same local client/table context.

    External side effects such as Lambda alias flips or CodePipeline executions
    are intentionally outside this helper's atomicity boundary. Callers should
    pair those side effects with explicit retry/reconciliation/outbox behavior.
    """

    _assert_same_transaction_context(actual_table, event_table)
    if not actual_key:
        raise ValidationError("actual_key is required")
    if not set_values:
        raise ValidationError("set_values is required")
    if event_item is None:
        raise ValidationError("event_item is required")

    version_attr = _resolve_attribute(actual_table._model, version_field, role="version")
    updates = _canonical_updates(actual_table._model, set_values)
    if version_attr.python_name in updates or version_attr.attribute_name in set_values:
        raise ValidationError("transition set must not mutate version directly")
    updates[version_attr.python_name] = UpdateAdd(1)

    condition_expression: str | None = None
    condition_names: dict[str, str] | None = None
    condition_values: dict[str, Any] | None = None
    if expected_version is not None:
        condition_expression = "#rs_version = :rs_expected_version"
        condition_names = {"#rs_version": version_attr.attribute_name}
        condition_values = {":rs_expected_version": expected_version}

    pk = _key_value(actual_table._model.pk, actual_key)
    sk = _key_value(actual_table._model.sk, actual_key) if actual_table._model.sk is not None else None

    update_req = actual_table._build_update_request(
        pk,
        sk,
        updates,
        condition_expression=condition_expression,
        expression_attribute_names=condition_names,
        expression_attribute_values=condition_values,
    )

    put_req: dict[str, Any] = {
        "TableName": event_table._table_name,
        "Item": event_table._to_item(event_item),
    }
    _apply_create_condition(event_table._model, put_req)

    try:
        actual_table._client.transact_write_items(
            TransactItems=[
                {"Update": update_req},
                {"Put": put_req},
            ]
        )
    except ClientError as err:  # pragma: no cover
        raise _map_transaction_error(err) from err


def _assert_same_transaction_context(actual_table: Table[Any], event_table: Table[Any]) -> None:
    if actual_table._client is not event_table._client:
        raise ValidationError("release-state transaction requires both tables to share one DynamoDB client")
    if actual_table._table_name != event_table._table_name:
        raise ValidationError("release-state transaction requires both rows to share one DynamoDB table")


def _key_value(attr: AttributeDefinition, values: Mapping[str, Any]) -> Any:
    if attr.python_name in values:
        return values[attr.python_name]
    if attr.attribute_name in values:
        return values[attr.attribute_name]
    raise ValidationError(f"missing key attribute: {attr.attribute_name}")


def _canonical_updates(model: ModelDefinition[Any], values: Mapping[str, Any]) -> dict[str, Any]:
    by_attribute_name = {attr.attribute_name: attr.python_name for attr in model.attributes.values()}
    updates: dict[str, Any] = {}
    for field, value in values.items():
        name = str(field)
        python_name = name if name in model.attributes else by_attribute_name.get(name)
        if python_name is None:
            raise ValidationError(f"unknown field: {name}")
        updates[python_name] = value
    return updates


def _resolve_attribute(
    model: ModelDefinition[Any],
    name: str,
    *,
    role: str | None = None,
) -> AttributeDefinition:
    attr = model.attributes.get(name)
    if attr is None:
        attr = next(
            (candidate for candidate in model.attributes.values() if candidate.attribute_name == name), None
        )
    if attr is None and role is not None:
        attr = next((candidate for candidate in model.attributes.values() if role in candidate.roles), None)
    if attr is None:
        raise ValidationError(f"unknown field: {name}")
    return attr


def _apply_create_condition(model: ModelDefinition[Any], request: dict[str, Any]) -> None:
    names = dict(request.get("ExpressionAttributeNames") or {})
    placeholder = _pk_placeholder(names, model.pk.attribute_name)
    names[placeholder] = model.pk.attribute_name

    condition = f"attribute_not_exists({placeholder})"
    existing = request.get("ConditionExpression")
    if existing:
        request["ConditionExpression"] = f"({existing}) AND {condition}"
    else:
        request["ConditionExpression"] = condition
    request["ExpressionAttributeNames"] = names


def _pk_placeholder(names: Mapping[str, str], pk_attribute: str) -> str:
    if names.get("#pk") in {None, pk_attribute}:
        return "#pk"
    index = 1
    while f"#pk{index}" in names:
        index += 1
    return f"#pk{index}"
