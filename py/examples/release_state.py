from __future__ import annotations

from dataclasses import dataclass
from typing import Any

from tabletheory_py import (
    ModelDefinition,
    Table,
    WritePolicy,
    theorydb_field,
    transition_release_state,
    validate_deploy_authority_metadata,
)
from tabletheory_py.mocks import FakeDynamoDBClient


@dataclass(frozen=True)
class ReleaseStateActual:
    pk: str = theorydb_field(name="PK", roles=["pk"])
    sk: str = theorydb_field(name="SK", roles=["sk"])
    service: str = theorydb_field(default="")
    status: str = theorydb_field(default="")
    pinned_release_id: str = theorydb_field(name="pinnedReleaseId", default="")
    previous_release_id: str = theorydb_field(name="previousReleaseId", default="")
    provenance: dict[str, Any] | None = theorydb_field(json=True, default=None)
    confidence: dict[str, Any] | None = theorydb_field(json=True, default=None)
    version: int = theorydb_field(roles=["version"], default=0)


@dataclass(frozen=True)
class ReleaseStateEvent:
    pk: str = theorydb_field(name="PK", roles=["pk"])
    sk: str = theorydb_field(name="SK", roles=["sk"])
    service: str = theorydb_field(default="")
    release_id: str = theorydb_field(name="releaseId", default="")
    event_type: str = theorydb_field(name="eventType", default="")
    at: str = theorydb_field(default="")
    actor: str = theorydb_field(default="")
    evidence: dict[str, Any] | None = theorydb_field(json=True, default=None)


@dataclass(frozen=True)
class ReleaseStateOutbox:
    pk: str = theorydb_field(name="PK", roles=["pk"])
    sk: str = theorydb_field(name="SK", roles=["sk"])
    operation: str = theorydb_field(default="")
    idempotency_key: str = theorydb_field(name="idempotencyKey", default="")
    requested_state: str = theorydb_field(name="requestedState", default="")
    next_attempt_at: str = theorydb_field(name="nextAttemptAt", default="")


def main() -> None:
    table_name = "release_state_contract"
    client = FakeDynamoDBClient()

    actual_table = Table(
        ModelDefinition.from_dataclass(
            ReleaseStateActual,
            table_name=table_name,
            write_policy=WritePolicy(mode="mutable", protected_attributes=["pinnedReleaseId"]),
        ),
        client=client,
    )
    event_table = Table(
        ModelDefinition.from_dataclass(
            ReleaseStateEvent,
            table_name=table_name,
            write_policy=WritePolicy(mode="write_once"),
        ),
        client=client,
    )
    _outbox_model = ModelDefinition.from_dataclass(
        ReleaseStateOutbox,
        table_name=table_name,
        write_policy=WritePolicy(mode="write_once"),
    )

    observed_at = "2026-04-24T19:00:00Z"
    recorded_at = "2026-04-24T19:00:01Z"
    service = "service-a"
    release_id = "rel_002"
    ref = f"operator://deploy/{service}/{release_id}"

    provenance: dict[str, Any] = {
        "mode": "native",
        "system": "release-control-plane",
        "kind": "operator_command",
        "ref": ref,
        "observed_at": observed_at,
        "recorded_at": recorded_at,
        "evidence": [
            {
                "kind": "operator_command",
                "source": "release-control-plane",
                "ref": ref,
                "observed_at": observed_at,
            }
        ],
    }
    confidence: dict[str, Any] = {"level": "high", "reasons": ["operator_command_authority"]}
    validate_deploy_authority_metadata({"provenance": provenance, "confidence": confidence})

    def validate_transaction(req: dict[str, Any]) -> None:
        items = req["TransactItems"]
        assert len(items) == 2
        assert items[0]["Update"]["TableName"] == table_name
        assert items[1]["Put"]["TableName"] == table_name

    client.expect("transact_write_items", validate_transaction, response={})
    transition_release_state(
        actual_table,
        event_table,
        actual_key={"PK": f"RELEASE#{service}", "SK": "ACTUAL"},
        expected_version=7,
        set_values={
            "status": "active",
            "previousReleaseId": "rel_001",
            "provenance": provenance,
            "confidence": confidence,
        },
        event_item=ReleaseStateEvent(
            pk=f"RELEASE#{service}",
            sk=f"EVENT#{observed_at}#{release_id}",
            service=service,
            release_id=release_id,
            event_type="promoted",
            at=observed_at,
            actor="operator@example.com",
            evidence=provenance,
        ),
    )
    client.assert_no_pending()

    outbox = ReleaseStateOutbox(
        pk=f"RELEASE#{service}",
        sk=f"OUTBOX#lambda-alias#{release_id}",
        operation="lambda_alias_update",
        idempotency_key=f"{service}:{release_id}",
        requested_state="active",
        next_attempt_at=observed_at,
    )
    print(f"transaction_items=2 outbox={outbox.sk}")


if __name__ == "__main__":
    main()
