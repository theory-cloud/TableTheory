from __future__ import annotations

from dataclasses import dataclass
from typing import Any

import pytest

from theorydb_py import (
    ImmutableModelMutationError,
    ModelDefinition,
    ProtectedFieldMutationError,
    Table,
    TransactDelete,
    TransactPut,
    TransactUpdate,
    WritePolicy,
    theorydb_field,
)
from theorydb_py.mocks import FakeDynamoDBClient


@dataclass(frozen=True)
class ReleaseStateActual:
    pk: str = theorydb_field(name="PK", roles=["pk"])
    sk: str = theorydb_field(name="SK", roles=["sk"])
    status: str = theorydb_field(default="")
    pinned_release_id: str = theorydb_field(name="pinnedReleaseId", default="")
    previous_release_id: str = theorydb_field(name="previousReleaseId", default="")
    version: int = theorydb_field(roles=["version"], default=0)


@dataclass(frozen=True)
class ReleaseStateEvent:
    pk: str = theorydb_field(name="PK", roles=["pk"])
    sk: str = theorydb_field(name="SK", roles=["sk"])
    release_id: str = theorydb_field(name="releaseId", default="")
    event_type: str = theorydb_field(name="eventType", default="")


def _actual_model() -> ModelDefinition[ReleaseStateActual]:
    return ModelDefinition.from_dataclass(
        ReleaseStateActual,
        table_name="release_state_contract",
        write_policy=WritePolicy(
            mode="mutable",
            protected_attributes=["pinned_release_id", "pinnedReleaseId"],
        ),
    )


def _event_model() -> ModelDefinition[ReleaseStateEvent]:
    return ModelDefinition.from_dataclass(
        ReleaseStateEvent,
        table_name="release_state_contract",
        write_policy=WritePolicy(mode="write_once"),
    )


def test_write_once_put_adds_default_condition_and_save_mutations_reject() -> None:
    client = FakeDynamoDBClient()

    def validate_put(req: dict[str, Any]) -> None:
        assert req["ConditionExpression"] == "attribute_not_exists(#pk)"
        assert req["ExpressionAttributeNames"]["#pk"] == "PK"

    client.expect("put_item", validate_put, response={})
    table: Table[ReleaseStateEvent] = Table(_event_model(), client=client)

    event = ReleaseStateEvent(pk="RELEASE#service-a", sk="EVENT#1", event_type="promoted")
    table.put(event)
    client.assert_no_pending()

    for operation in (
        lambda: table.save(event),
        lambda: table.update("RELEASE#service-a", "EVENT#1", {"event_type": "mutated"}),
        lambda: table.delete("RELEASE#service-a", "EVENT#1"),
        lambda: table.batch_write(puts=[event]),
        lambda: table.batch_write(deletes=[("RELEASE#service-a", "EVENT#1")]),
    ):
        with pytest.raises(ImmutableModelMutationError):
            operation()

    assert len(client.calls) == 1


def test_write_once_put_combines_existing_condition_without_loosened_names() -> None:
    client = FakeDynamoDBClient()

    def validate_put(req: dict[str, Any]) -> None:
        assert (
            req["ConditionExpression"] == "(#existing = :expected) AND attribute_not_exists(#write_once_pk)"
        )
        assert req["ExpressionAttributeNames"] == {
            "#pk": "notPK",
            "#existing": "eventType",
            "#write_once_pk": "PK",
        }

    client.expect("put_item", validate_put, response={})
    table: Table[ReleaseStateEvent] = Table(_event_model(), client=client)
    table.put(
        ReleaseStateEvent(pk="RELEASE#service-a", sk="EVENT#1", event_type="promoted"),
        condition_expression="#existing = :expected",
        expression_attribute_names={"#pk": "notPK", "#existing": "eventType"},
        expression_attribute_values={":expected": "promoted"},
    )
    client.assert_no_pending()


def test_protected_put_adds_default_condition_and_overwrites_reject() -> None:
    client = FakeDynamoDBClient()

    def validate_put(req: dict[str, Any]) -> None:
        assert req["ConditionExpression"] == "attribute_not_exists(#pk)"
        assert req["ExpressionAttributeNames"]["#pk"] == "PK"

    client.expect("put_item", validate_put, response={})
    table: Table[ReleaseStateActual] = Table(_actual_model(), client=client)

    actual = ReleaseStateActual(
        pk="RELEASE#service-a",
        sk="ACTUAL",
        status="active",
        pinned_release_id="rel_001",
    )
    table.put(actual)
    client.assert_no_pending()

    for operation in (
        lambda: table.save(actual),
        lambda: table.batch_write(puts=[actual]),
    ):
        with pytest.raises(ProtectedFieldMutationError):
            operation()

    assert len(client.calls) == 1


def test_protected_attributes_block_table_updates_with_additive_tightening() -> None:
    client = FakeDynamoDBClient()
    model = _actual_model()
    table: Table[ReleaseStateActual] = Table(model, client=client)

    def validate_status_update(req: dict[str, Any]) -> None:
        assert _updates_field(req, "status")

    client.expect(
        "update_item",
        validate_status_update,
        response={
            "Attributes": table._to_item(
                ReleaseStateActual(pk="RELEASE#service-a", sk="ACTUAL", status="warming")
            )
        },
    )

    table.update("RELEASE#service-a", "ACTUAL", {"status": "warming"})
    client.assert_no_pending()

    with pytest.raises(ProtectedFieldMutationError):
        table.update("RELEASE#service-a", "ACTUAL", {"pinned_release_id": "rel_002"})

    with pytest.raises(ProtectedFieldMutationError):
        table.update("RELEASE#service-a", "ACTUAL", {"pinned_release_id.value": "rel_002"})

    with pytest.raises(ProtectedFieldMutationError):
        table.update(
            "RELEASE#service-a",
            "ACTUAL",
            {"status": "active"},
            protected_attributes=["status"],
        )

    client.expect("delete_item", response={})
    table.delete("RELEASE#service-a", "ACTUAL")
    client.assert_no_pending()


def test_protected_attributes_block_update_builder_mutations() -> None:
    table: Table[ReleaseStateActual] = Table(_actual_model(), client=FakeDynamoDBClient())

    with pytest.raises(ProtectedFieldMutationError):
        table.update_builder("RELEASE#service-a", "ACTUAL").set("pinned_release_id", "rel_002").execute()

    with pytest.raises(ProtectedFieldMutationError):
        table.update_builder("RELEASE#service-a", "ACTUAL").remove("pinned_release_id").execute()


def test_transactions_enforce_write_policy() -> None:
    event_client = FakeDynamoDBClient()

    def validate_put(req: dict[str, Any]) -> None:
        put = req["TransactItems"][0]["Put"]
        assert put["ConditionExpression"] == "attribute_not_exists(#pk)"
        assert put["ExpressionAttributeNames"]["#pk"] == "PK"

    event_client.expect("transact_write_items", validate_put, response={})
    event_table: Table[ReleaseStateEvent] = Table(_event_model(), client=event_client)
    event_table.transact_write(
        [TransactPut(item=ReleaseStateEvent(pk="RELEASE#service-a", sk="EVENT#1", event_type="promoted"))]
    )
    event_client.assert_no_pending()

    with pytest.raises(ImmutableModelMutationError):
        event_table.transact_write(
            [TransactUpdate(pk="RELEASE#service-a", sk="EVENT#1", updates={"event_type": "mutated"})]
        )

    with pytest.raises(ImmutableModelMutationError):
        event_table.transact_write([TransactDelete(pk="RELEASE#service-a", sk="EVENT#1")])

    actual_table: Table[ReleaseStateActual] = Table(_actual_model(), client=FakeDynamoDBClient())
    with pytest.raises(ProtectedFieldMutationError):
        actual_table.transact_write(
            [
                TransactUpdate(
                    pk="RELEASE#service-a",
                    sk="ACTUAL",
                    updates={"status": "warming"},
                    protected_attributes=["status"],
                )
            ]
        )

    actual_client = FakeDynamoDBClient()

    def validate_protected_put(req: dict[str, Any]) -> None:
        put = req["TransactItems"][0]["Put"]
        assert put["ConditionExpression"] == "attribute_not_exists(#pk)"
        assert put["ExpressionAttributeNames"]["#pk"] == "PK"

    actual_client.expect("transact_write_items", validate_protected_put, response={})
    actual_table = Table(_actual_model(), client=actual_client)
    actual_table.transact_write(
        [
            TransactPut(
                item=ReleaseStateActual(
                    pk="RELEASE#service-a",
                    sk="ACTUAL",
                    status="active",
                    pinned_release_id="rel_001",
                )
            )
        ]
    )
    actual_client.assert_no_pending()


def _updates_field(req: dict[str, Any], attribute_name: str) -> bool:
    return attribute_name in req["ExpressionAttributeNames"].values()
