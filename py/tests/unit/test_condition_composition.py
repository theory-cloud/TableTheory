from __future__ import annotations

from dataclasses import dataclass

from tabletheory_py import (
    ModelDefinition,
    StatefulDynamoDBClient,
    Table,
    theorydb_field,
)

_TABLE_NAME = "versioned_notes"


@dataclass(frozen=True)
class VersionedNote:
    pk: str = theorydb_field(name="PK", roles=["pk"])
    sk: str = theorydb_field(name="SK", roles=["sk"])
    name: str = theorydb_field(name="name")
    version: int = theorydb_field(name="version", roles=["version"], default=0)


def _table(client: StatefulDynamoDBClient) -> Table[VersionedNote]:
    model = ModelDefinition.from_dataclass(VersionedNote, table_name=_TABLE_NAME)
    return Table(
        model,
        client=client,
        now=lambda: "2026-01-02T03:04:05.123456789Z",
    )


def test_chained_update_conditions_keep_distinct_placeholders() -> None:
    builder = _table(StatefulDynamoDBClient()).update_builder("A", "1")
    builder.set("name", "alice").condition_version(0).condition("name", "=", "alice")

    request = builder._build_request()

    expression = request["ConditionExpression"]
    assert isinstance(expression, str)
    # Both caller predicates survive as separate comparisons.
    assert expression.count("=") == 2

    names = request["ExpressionAttributeNames"]
    assert "version" in names.values()
    assert "name" in names.values()

    expression_names = request["ExpressionAttributeNames"]
    version_ref = next(ref for ref, attr in expression_names.items() if attr == "version")
    name_ref = next(ref for ref, attr in expression_names.items() if attr == "name" and ref.startswith("#c_"))
    assert version_ref in expression
    assert name_ref in expression
    assert version_ref != name_ref

    values = request["ExpressionAttributeValues"]
    condition_values = [ref for ref in values if ref.startswith(":c") and ref[2:].isdigit()]
    assert len(condition_values) == 2, "each condition must own its own value placeholder"
