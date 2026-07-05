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
