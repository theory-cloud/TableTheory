from __future__ import annotations

from dataclasses import dataclass
from typing import Any

import pytest

from tabletheory_py import (
    ModelDefinition,
    Table,
    WritePolicy,
    theorydb_field,
    transition_release_state,
    validate_deploy_authority_metadata,
)
from tabletheory_py.errors import RejectedDeployAuthorityEvidenceError, ValidationError
from tabletheory_py.mocks import FakeDynamoDBClient
from tabletheory_py.release_state import _binding_values_equal


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
    version: int = theorydb_field(default=0)
    version_text: str = theorydb_field(name="versionText", default="")


def _actual_table(client: FakeDynamoDBClient) -> Table[ReleaseStateActual]:
    return Table(
        ModelDefinition.from_dataclass(
            ReleaseStateActual,
            table_name="release_state_contract",
            write_policy=WritePolicy(mode="mutable", protected_attributes=["pinnedReleaseId"]),
        ),
        client=client,
    )


def _event_table(client: FakeDynamoDBClient) -> Table[ReleaseStateEvent]:
    return Table(
        ModelDefinition.from_dataclass(
            ReleaseStateEvent,
            table_name="release_state_contract",
            write_policy=WritePolicy(mode="write_once"),
        ),
        client=client,
    )


def test_transition_release_state_uses_one_transaction() -> None:
    client = FakeDynamoDBClient()
    actual_table = _actual_table(client)
    event_table = _event_table(client)

    def validate(req: dict[str, Any]) -> None:
        items = req["TransactItems"]
        assert len(items) == 2

        update = items[0]["Update"]
        assert update["TableName"] == "release_state_contract"
        assert "SET" in update["UpdateExpression"]
        assert "ADD" in update["UpdateExpression"]
        assert update["ConditionExpression"] == "#rs_version = :rs_expected_version"
        assert update["ExpressionAttributeNames"]["#rs_version"] == "version"
        assert update["ExpressionAttributeValues"][":rs_expected_version"] == {"N": "0"}
        assert "previousReleaseId" in update["ExpressionAttributeNames"].values()

        put = items[1]["Put"]
        assert put["TableName"] == "release_state_contract"
        assert put["ConditionExpression"] == "attribute_not_exists(#pk)"
        assert put["ExpressionAttributeNames"]["#pk"] == "PK"

    client.expect("transact_write_items", validate, response={})

    transition_release_state(
        actual_table,
        event_table,
        actual_key={"PK": "RELEASE#service-a", "SK": "ACTUAL"},
        expected_version=0,
        set_values={"status": "active", "previousReleaseId": "rel_001"},
        event_item=ReleaseStateEvent(
            pk="RELEASE#service-a",
            sk="EVENT#1",
            release_id="rel_002",
            event_type="promoted",
        ),
    )

    client.assert_no_pending()


def test_transition_release_state_requires_shared_context_and_blocks_version_set() -> None:
    actual_table = _actual_table(FakeDynamoDBClient())
    event_table = _event_table(FakeDynamoDBClient())

    with pytest.raises(ValidationError, match="share one DynamoDB client"):
        transition_release_state(
            actual_table,
            event_table,
            actual_key={"PK": "RELEASE#service-a", "SK": "ACTUAL"},
            set_values={"status": "active"},
            event_item=ReleaseStateEvent(pk="RELEASE#service-a", sk="EVENT#1"),
        )

    shared = FakeDynamoDBClient()
    actual_table = _actual_table(shared)
    event_table = _event_table(shared)
    with pytest.raises(ValidationError, match="version"):
        transition_release_state(
            actual_table,
            event_table,
            actual_key={"PK": "RELEASE#service-a", "SK": "ACTUAL"},
            set_values={"version": 2},
            event_item=ReleaseStateEvent(pk="RELEASE#service-a", sk="EVENT#1"),
        )


def _valid_deploy_authority_item() -> dict[str, Any]:
    return {
        "provenance": {
            "mode": "native",
            "system": "release-control-plane",
            "kind": "operator_command",
            "ref": "operator://deploy/service-a/rel_001",
            "observed_at": "2026-04-24T19:00:00Z",
            "recorded_at": "2026-04-24T19:00:01Z",
            "evidence": [
                {
                    "kind": "operator_command",
                    "source": "release-control-plane",
                    "ref": "operator://deploy/service-a/rel_001",
                    "observed_at": "2026-04-24T19:00:00Z",
                }
            ],
        },
        "confidence": {
            "level": "high",
            "reasons": ["operator_command_authority"],
        },
    }


def test_validate_deploy_authority_metadata_accepts_deterministic_high_confidence() -> None:
    validate_deploy_authority_metadata(_valid_deploy_authority_item())
    validate_deploy_authority_metadata({"PK": "RELEASE#service-a"})


def test_validate_deploy_authority_metadata_rejects_conflicting_evidence() -> None:
    item = _valid_deploy_authority_item()
    item["provenance"] = {
        "mode": "imported",
        "system": "partner-factory",
        "kind": "factory_batch_manifest",
        "ref": "s3://factory/manifests/ambiguous.json",
        "observed_at": "2026-04-24T19:10:00Z",
        "recorded_at": "2026-04-24T19:10:01Z",
        "evidence": [
            {
                "kind": "factory_batch_manifest",
                "source": "partner-factory",
                "ref": "s3://factory/manifests/a.json",
                "observed_at": "2026-04-24T19:09:59Z",
            },
            {
                "kind": "submodule_pin",
                "source": "service-ci",
                "ref": "https://github.com/acme/service-b/tree/conflicting",
                "observed_at": "2026-04-24T19:09:59Z",
            },
        ],
    }
    item["confidence"] = {"level": "low", "reasons": ["conflicting_evidence"]}

    with pytest.raises(RejectedDeployAuthorityEvidenceError):
        validate_deploy_authority_metadata(item)


def test_validate_deploy_authority_metadata_rejects_free_form_notes() -> None:
    item = _valid_deploy_authority_item()
    item["provenance"]["notes"] = "human note"

    with pytest.raises(ValidationError):
        validate_deploy_authority_metadata(item)


@dataclass(frozen=True)
class ReleaseStateOutbox:
    pk: str = theorydb_field(name="PK", roles=["pk"])
    sk: str = theorydb_field(name="SK", roles=["sk"])
    operation: str = theorydb_field(default="")
    status: str = theorydb_field(default="")


@dataclass(frozen=True)
class ReleaseStateScopedOutbox:
    partition: str = theorydb_field(name="OUTBOX_PK", roles=["pk"])
    sk: str = theorydb_field(name="SK", roles=["sk"])
    status: str = theorydb_field(default="")


def _outbox_table(client: FakeDynamoDBClient) -> Table[ReleaseStateOutbox]:
    return Table(
        ModelDefinition.from_dataclass(
            ReleaseStateOutbox,
            table_name="release_state_contract",
            write_policy=WritePolicy(mode="write_once"),
        ),
        client=client,
    )


def _scoped_outbox_table(client: FakeDynamoDBClient) -> Table[ReleaseStateScopedOutbox]:
    return Table(
        ModelDefinition.from_dataclass(
            ReleaseStateScopedOutbox,
            table_name="release_state_contract",
            write_policy=WritePolicy(mode="write_once"),
        ),
        client=client,
    )


def test_transition_release_state_accepts_matching_event_binding() -> None:
    client = FakeDynamoDBClient()
    actual_table = _actual_table(client)
    event_table = _event_table(client)
    client.expect("transact_write_items", None, response={})

    transition_release_state(
        actual_table,
        event_table,
        actual_key={"PK": "RELEASE#service-a", "SK": "ACTUAL"},
        expected_version=0,
        set_values={"previous_release_id": "rel_001"},
        event_item=ReleaseStateEvent(
            pk="RELEASE#service-a",
            sk="EVENT#1",
            release_id="rel_001",
        ),
        event_binding={"releaseId": "set:previous_release_id"},
    )

    assert len(client.calls) == 1
    client.assert_no_pending()


def test_transition_release_state_accepts_set_binding_by_attribute_name() -> None:
    client = FakeDynamoDBClient()
    actual_table = _actual_table(client)
    event_table = _event_table(client)
    client.expect("transact_write_items", None, response={})

    transition_release_state(
        actual_table,
        event_table,
        actual_key={"PK": "RELEASE#service-a", "SK": "ACTUAL"},
        set_values={"previousReleaseId": "rel_001"},
        event_item=ReleaseStateEvent(
            pk="RELEASE#service-a",
            sk="EVENT#1",
            release_id="rel_001",
        ),
        event_binding={"releaseId": "set:previousReleaseId"},
    )

    assert len(client.calls) == 1
    client.assert_no_pending()


def test_transition_release_state_rejects_mismatched_event_binding() -> None:
    client = FakeDynamoDBClient()
    actual_table = _actual_table(client)
    event_table = _event_table(client)

    with pytest.raises(ValidationError, match='event attribute "releaseId"'):
        transition_release_state(
            actual_table,
            event_table,
            actual_key={"PK": "RELEASE#service-a", "SK": "ACTUAL"},
            set_values={"pinned_release_id": "rel_002"},
            event_item=ReleaseStateEvent(
                pk="RELEASE#service-a",
                sk="EVENT#1",
                release_id="rel_001",
            ),
            event_binding={"releaseId": "set:pinned_release_id"},
        )

    assert client.calls == []


def test_transition_release_state_rejects_missing_event_bound_attribute() -> None:
    client = FakeDynamoDBClient()
    actual_table = _actual_table(client)
    event_table = _event_table(client)

    with pytest.raises(ValidationError, match="event row is missing bound attribute: missingAttr"):
        transition_release_state(
            actual_table,
            event_table,
            actual_key={"PK": "RELEASE#service-a", "SK": "ACTUAL"},
            set_values={"pinned_release_id": "rel_002"},
            event_item=ReleaseStateEvent(pk="RELEASE#service-a", sk="EVENT#1"),
            event_binding={"missingAttr": "set:pinned_release_id"},
        )

    assert client.calls == []


def test_transition_release_state_rejects_unknown_event_binding_source() -> None:
    client = FakeDynamoDBClient()
    actual_table = _actual_table(client)
    event_table = _event_table(client)

    with pytest.raises(ValidationError, match='event binding for "releaseId".*unsupported binding source'):
        transition_release_state(
            actual_table,
            event_table,
            actual_key={"PK": "RELEASE#service-a", "SK": "ACTUAL"},
            set_values={"pinned_release_id": "rel_002"},
            event_item=ReleaseStateEvent(pk="RELEASE#service-a", sk="EVENT#1"),
            event_binding={"releaseId": "guess:pinned_release_id"},
        )

    assert client.calls == []


def test_transition_release_state_rejects_unknown_set_binding_field() -> None:
    client = FakeDynamoDBClient()
    actual_table = _actual_table(client)
    event_table = _event_table(client)

    with pytest.raises(ValidationError, match='event binding for "releaseId".*set field "unknown"'):
        transition_release_state(
            actual_table,
            event_table,
            actual_key={"PK": "RELEASE#service-a", "SK": "ACTUAL"},
            set_values={"pinned_release_id": "rel_002"},
            event_item=ReleaseStateEvent(pk="RELEASE#service-a", sk="EVENT#1"),
            event_binding={"releaseId": "set:unknown"},
        )

    assert client.calls == []


def test_transition_release_state_accepts_actual_key_binding() -> None:
    client = FakeDynamoDBClient()
    actual_table = _actual_table(client)
    event_table = _event_table(client)
    client.expect("transact_write_items", None, response={})

    transition_release_state(
        actual_table,
        event_table,
        actual_key={"PK": "RELEASE#service-a", "SK": "ACTUAL"},
        set_values={"status": "active"},
        event_item=ReleaseStateEvent(
            pk="RELEASE#service-a",
            sk="EVENT#1",
            release_id="ACTUAL",
        ),
        event_binding={"releaseId": "actual:sk"},
    )

    assert len(client.calls) == 1
    client.assert_no_pending()


def test_transition_release_state_accepts_matching_outbox_binding() -> None:
    client = FakeDynamoDBClient()
    actual_table = _actual_table(client)
    event_table = _event_table(client)
    outbox_table = _outbox_table(client)
    client.expect("transact_write_items", None, response={})

    transition_release_state(
        actual_table,
        event_table,
        actual_key={"PK": "RELEASE#service-a", "SK": "ACTUAL"},
        set_values={"status": "active"},
        event_item=ReleaseStateEvent(pk="RELEASE#service-a", sk="EVENT#1"),
        outbox_item=ReleaseStateOutbox(pk="RELEASE#service-a", sk="OUTBOX#1", status="active"),
        outbox_table=outbox_table,
        outbox_binding={"status": "set:status"},
    )

    assert len(client.calls) == 1
    client.assert_no_pending()


def test_transition_release_state_rejects_mismatched_outbox_binding() -> None:
    client = FakeDynamoDBClient()
    actual_table = _actual_table(client)
    event_table = _event_table(client)
    outbox_table = _outbox_table(client)

    with pytest.raises(ValidationError, match='outbox attribute "status"'):
        transition_release_state(
            actual_table,
            event_table,
            actual_key={"PK": "RELEASE#service-a", "SK": "ACTUAL"},
            set_values={"status": "active"},
            event_item=ReleaseStateEvent(pk="RELEASE#service-a", sk="EVENT#1"),
            outbox_item=ReleaseStateOutbox(pk="RELEASE#service-a", sk="OUTBOX#1", status="inactive"),
            outbox_table=outbox_table,
            outbox_binding={"status": "set:status"},
        )

    assert client.calls == []


def test_transition_release_state_rejects_mismatched_outbox_partition() -> None:
    client = FakeDynamoDBClient()
    actual_table = _actual_table(client)
    event_table = _event_table(client)
    outbox_table = _outbox_table(client)

    with pytest.raises(ValidationError, match="outbox partition does not match"):
        transition_release_state(
            actual_table,
            event_table,
            actual_key={"PK": "RELEASE#service-a", "SK": "ACTUAL"},
            set_values={"status": "active"},
            event_item=ReleaseStateEvent(pk="RELEASE#service-a", sk="EVENT#1"),
            outbox_item=ReleaseStateOutbox(pk="RELEASE#service-b", sk="OUTBOX#1"),
            outbox_table=outbox_table,
        )

    assert client.calls == []


def test_transition_release_state_allows_differently_keyed_outbox_model() -> None:
    client = FakeDynamoDBClient()
    actual_table = _actual_table(client)
    event_table = _event_table(client)
    outbox_table = _scoped_outbox_table(client)

    def validate(req: dict[str, Any]) -> None:
        assert len(req["TransactItems"]) == 3
        assert req["TransactItems"][2]["Put"]["Item"]["OUTBOX_PK"] == {"S": "OUTBOX#service-a"}

    client.expect("transact_write_items", validate, response={})

    transition_release_state(
        actual_table,
        event_table,
        actual_key={"PK": "RELEASE#service-a", "SK": "ACTUAL"},
        set_values={"status": "active"},
        event_item=ReleaseStateEvent(pk="RELEASE#service-a", sk="EVENT#1"),
        outbox_item=ReleaseStateScopedOutbox(
            partition="OUTBOX#service-a",
            sk="OUTBOX#1",
            status="active",
        ),
        outbox_table=outbox_table,
        outbox_binding={"status": "set:status"},
    )

    assert len(client.calls) == 1
    client.assert_no_pending()


def test_transition_release_state_accepts_next_version_binding() -> None:
    client = FakeDynamoDBClient()
    actual_table = _actual_table(client)
    event_table = _event_table(client)
    client.expect("transact_write_items", None, response={})

    transition_release_state(
        actual_table,
        event_table,
        actual_key={"PK": "RELEASE#service-a", "SK": "ACTUAL"},
        expected_version=2,
        set_values={"status": "active"},
        event_item=ReleaseStateEvent(pk="RELEASE#service-a", sk="EVENT#1", version=3),
        event_binding={"version": "nextVersion"},
    )

    assert len(client.calls) == 1
    client.assert_no_pending()


def test_transition_release_state_rejects_numeric_string_next_version_binding() -> None:
    client = FakeDynamoDBClient()
    actual_table = _actual_table(client)
    event_table = _event_table(client)

    with pytest.raises(ValidationError, match='event attribute "versionText"'):
        transition_release_state(
            actual_table,
            event_table,
            actual_key={"PK": "RELEASE#service-a", "SK": "ACTUAL"},
            expected_version=2,
            set_values={"status": "active"},
            event_item=ReleaseStateEvent(pk="RELEASE#service-a", sk="EVENT#1", version_text="3"),
            event_binding={"versionText": "nextVersion"},
        )

    assert client.calls == []


def test_binding_values_equal_rejects_bool_number_mix() -> None:
    assert _binding_values_equal(7, 7.0)
    assert _binding_values_equal(True, True)
    assert not _binding_values_equal(True, 1)
    assert not _binding_values_equal(False, 0)


def test_transition_release_state_rejects_stale_next_version_binding() -> None:
    client = FakeDynamoDBClient()
    actual_table = _actual_table(client)
    event_table = _event_table(client)

    with pytest.raises(ValidationError, match='event attribute "version"'):
        transition_release_state(
            actual_table,
            event_table,
            actual_key={"PK": "RELEASE#service-a", "SK": "ACTUAL"},
            expected_version=2,
            set_values={"status": "active"},
            event_item=ReleaseStateEvent(pk="RELEASE#service-a", sk="EVENT#1", version=4),
            event_binding={"version": "nextVersion"},
        )

    assert client.calls == []


def test_transition_release_state_next_version_requires_expected_version() -> None:
    client = FakeDynamoDBClient()
    actual_table = _actual_table(client)
    event_table = _event_table(client)

    with pytest.raises(ValidationError, match="nextVersion requires expected_version"):
        transition_release_state(
            actual_table,
            event_table,
            actual_key={"PK": "RELEASE#service-a", "SK": "ACTUAL"},
            set_values={"status": "active"},
            event_item=ReleaseStateEvent(pk="RELEASE#service-a", sk="EVENT#1"),
            event_binding={"version": "nextVersion"},
        )

    assert client.calls == []


def test_transition_release_state_outbox_binding_requires_outbox_item() -> None:
    client = FakeDynamoDBClient()
    actual_table = _actual_table(client)
    event_table = _event_table(client)

    with pytest.raises(ValidationError, match="outbox binding requires an outbox row"):
        transition_release_state(
            actual_table,
            event_table,
            actual_key={"PK": "RELEASE#service-a", "SK": "ACTUAL"},
            set_values={"status": "active"},
            event_item=ReleaseStateEvent(pk="RELEASE#service-a", sk="EVENT#1"),
            outbox_binding={"status": "set:status"},
        )

    assert client.calls == []


def test_transition_release_state_rejects_mismatched_event_partition() -> None:
    client = FakeDynamoDBClient()
    actual_table = _actual_table(client)
    event_table = _event_table(client)

    with pytest.raises(ValidationError, match="does not match"):
        transition_release_state(
            actual_table,
            event_table,
            actual_key={"PK": "RELEASE#service-a", "SK": "ACTUAL"},
            set_values={"status": "active"},
            event_item=ReleaseStateEvent(pk="RELEASE#service-b", sk="EVENT#1"),
        )

    assert client.calls == []


def test_transition_release_state_includes_outbox_in_one_transaction() -> None:
    client = FakeDynamoDBClient()
    actual_table = _actual_table(client)
    event_table = _event_table(client)
    outbox_table = _outbox_table(client)

    def validate(req: dict[str, Any]) -> None:
        items = req["TransactItems"]
        assert len(items) == 3
        outbox = items[2]["Put"]
        assert outbox["TableName"] == "release_state_contract"
        assert outbox["ConditionExpression"] == "attribute_not_exists(#pk)"
        assert outbox["Item"]["PK"] == {"S": "RELEASE#service-a"}

    client.expect("transact_write_items", validate, response={})

    transition_release_state(
        actual_table,
        event_table,
        actual_key={"PK": "RELEASE#service-a", "SK": "ACTUAL"},
        set_values={"status": "active"},
        event_item=ReleaseStateEvent(pk="RELEASE#service-a", sk="EVENT#1"),
        outbox_item=ReleaseStateOutbox(pk="RELEASE#service-a", sk="OUTBOX#1"),
        outbox_table=outbox_table,
    )

    client.assert_no_pending()


def test_transition_release_state_propagates_rejected_transaction() -> None:
    client = FakeDynamoDBClient()
    actual_table = _actual_table(client)
    event_table = _event_table(client)
    outbox_table = _outbox_table(client)

    client.expect(
        "transact_write_items",
        None,
        error=RuntimeError("TransactionCanceledException"),
    )

    with pytest.raises(RuntimeError, match="TransactionCanceledException"):
        transition_release_state(
            actual_table,
            event_table,
            actual_key={"PK": "RELEASE#service-a", "SK": "ACTUAL"},
            set_values={"status": "active"},
            event_item=ReleaseStateEvent(pk="RELEASE#service-a", sk="EVENT#1"),
            outbox_item=ReleaseStateOutbox(pk="RELEASE#service-a", sk="OUTBOX#1"),
            outbox_table=outbox_table,
        )

    assert len(client.calls) == 1
    client.assert_no_pending()


def test_validate_deploy_authority_metadata_digest_conflict() -> None:
    item = _valid_deploy_authority_item()
    base = dict(item["provenance"]["evidence"][0])
    item["provenance"]["evidence"] = [
        {**base, "digest": "sha256:aaaaaaaa"},
        {**base, "digest": "sha256:bbbbbbbb"},
    ]

    with pytest.raises(RejectedDeployAuthorityEvidenceError):
        validate_deploy_authority_metadata(item)


def test_validate_deploy_authority_metadata_identical_evidence_is_idempotent() -> None:
    item = _valid_deploy_authority_item()
    base = dict(item["provenance"]["evidence"][0])
    item["provenance"]["evidence"] = [
        {**base, "digest": "sha256:cccccccc"},
        {**base, "digest": "sha256:cccccccc"},
    ]

    validate_deploy_authority_metadata(item)


def test_validate_deploy_authority_metadata_rejects_unrelated_evidence() -> None:
    item = _valid_deploy_authority_item()
    item["provenance"]["ref"] = "operator://deploy/service-b/rel_999"

    with pytest.raises(RejectedDeployAuthorityEvidenceError):
        validate_deploy_authority_metadata(item)


def test_validate_deploy_authority_metadata_aligns_top_level_digest() -> None:
    item = _valid_deploy_authority_item()
    base = dict(item["provenance"]["evidence"][0])
    item["provenance"]["evidence"] = [{**base, "digest": "sha256:dddddddd"}]
    item["provenance"]["digest"] = "sha256:dddddddd"

    validate_deploy_authority_metadata(item)

    item["provenance"]["digest"] = "sha256:eeeeeeee"
    with pytest.raises(RejectedDeployAuthorityEvidenceError):
        validate_deploy_authority_metadata(item)
