from __future__ import annotations

import re
from collections.abc import Mapping
from datetime import datetime
from typing import Any

from botocore.exceptions import ClientError

from .aws_errors import map_transaction_error as _map_transaction_error
from .errors import RejectedDeployAuthorityEvidenceError, ValidationError
from .model import AttributeDefinition, ModelDefinition
from .table import Table
from .transaction import UpdateAdd

_RFC3339_RE = re.compile(r"^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$")
_PROVENANCE_KEYS = {
    "mode",
    "system",
    "kind",
    "ref",
    "commit_sha",
    "observed_at",
    "recorded_at",
    "import_run_id",
    "evidence",
}
_EVIDENCE_KEYS = {"kind", "source", "ref", "observed_at", "digest"}
_CONFIDENCE_KEYS = {"level", "reasons"}
_PROVENANCE_MODES = {"native", "imported", "inferred"}


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


def validate_deploy_authority_metadata(item: Mapping[str, Any]) -> None:
    """Validate deterministic provenance/confidence deploy authority metadata.

    Ambiguous/conflicting or low-confidence evidence is rejected so it cannot be
    persisted as deploy authority. Preserve that evidence as separate immutable
    visibility/event records instead.
    """

    has_provenance = "provenance" in item
    has_confidence = "confidence" in item
    if not has_provenance and not has_confidence:
        return
    if not has_provenance or not has_confidence:
        raise ValidationError("provenance and confidence must be provided together")

    provenance = _mapping_value(item["provenance"], "provenance")
    confidence = _mapping_value(item["confidence"], "confidence")
    _validate_allowed_keys("provenance", provenance, _PROVENANCE_KEYS)
    _validate_allowed_keys("confidence", confidence, _CONFIDENCE_KEYS)
    _validate_provenance_shape(provenance)

    expected_reason = _derive_deploy_authority_reason(provenance)
    _validate_confidence(confidence, expected_reason)


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


def _validate_provenance_shape(provenance: Mapping[str, Any]) -> None:
    mode = _required_string(provenance, "mode")
    if mode not in _PROVENANCE_MODES:
        raise ValidationError(f"unsupported provenance.mode: {mode}")
    for key in ("system", "kind", "ref"):
        _required_string(provenance, key)
    for key in ("observed_at", "recorded_at"):
        _validate_rfc3339(key, _required_string(provenance, key))
    for key in ("commit_sha", "import_run_id"):
        if key in provenance and not isinstance(provenance[key], str):
            raise ValidationError(f"provenance.{key} must be a string")


def _derive_deploy_authority_reason(provenance: Mapping[str, Any]) -> str:
    evidence = _evidence_values(provenance.get("evidence"))
    if not evidence:
        raise RejectedDeployAuthorityEvidenceError("deploy authority requires evidence")

    signature: str | None = None
    first: Mapping[str, Any] | None = None
    for entry in evidence:
        _validate_allowed_keys("provenance.evidence", entry, _EVIDENCE_KEYS)
        kind = _required_string(entry, "kind")
        source = _required_string(entry, "source")
        ref = _required_string(entry, "ref")
        _validate_rfc3339("evidence.observed_at", _required_string(entry, "observed_at"))
        if "digest" in entry and not isinstance(entry["digest"], str):
            raise ValidationError("evidence.digest must be a string")

        current_signature = f"{kind}|{source}|{ref}"
        if signature is None:
            signature = current_signature
            first = entry
            continue
        if current_signature != signature:
            raise RejectedDeployAuthorityEvidenceError("conflicting deploy authority evidence")

    assert first is not None
    kind = _required_string(first, "kind")
    source = _required_string(first, "source")
    if kind == "operator_command" and source == "release-control-plane":
        return "operator_command_authority"
    if kind == "factory_batch_manifest" and source == "partner-factory":
        return "unique_factory_manifest_match"
    if kind == "codepipeline_execution" and source == "service-ci":
        return "codepipeline_execution_authority"
    if kind == "submodule_pin" and source == "service-ci":
        return "unique_submodule_pin_match"
    raise RejectedDeployAuthorityEvidenceError(f"unsupported deploy authority evidence: {source}/{kind}")


def _validate_confidence(confidence: Mapping[str, Any], expected_reason: str) -> None:
    level = _required_string(confidence, "level")
    if level != "high":
        raise RejectedDeployAuthorityEvidenceError(f"{level} confidence cannot authorize deploy state")
    reasons = _string_list(confidence.get("reasons"))
    if len(reasons) != 1 or reasons[0] != expected_reason:
        raise RejectedDeployAuthorityEvidenceError("confidence reasons do not match deterministic authority")


def _validate_allowed_keys(label: str, values: Mapping[str, Any], allowed: set[str]) -> None:
    for key in values:
        if key not in allowed:
            raise ValidationError(f"unsupported {label} key: {key}")


def _required_string(values: Mapping[str, Any], key: str) -> str:
    value = values.get(key)
    if not isinstance(value, str) or not value:
        raise ValidationError(f"{key} must be a non-empty string")
    return value


def _validate_rfc3339(label: str, value: str) -> None:
    if not _RFC3339_RE.match(value):
        raise ValidationError(f"{label} must be RFC3339")
    try:
        datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError as err:
        raise ValidationError(f"{label} must be RFC3339") from err


def _mapping_value(value: Any, label: str) -> Mapping[str, Any]:
    if not isinstance(value, Mapping):
        raise ValidationError(f"{label} must be an object")
    return value


def _evidence_values(value: Any) -> list[Mapping[str, Any]]:
    if not isinstance(value, list):
        raise ValidationError("provenance.evidence must be an array")
    return [_mapping_value(entry, "provenance.evidence entry") for entry in value]


def _string_list(value: Any) -> list[str]:
    if not isinstance(value, list):
        raise ValidationError("confidence.reasons must be a string array")
    out: list[str] = []
    for entry in value:
        if not isinstance(entry, str) or not entry:
            raise ValidationError("confidence.reasons must contain strings")
        out.append(entry)
    return out


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
