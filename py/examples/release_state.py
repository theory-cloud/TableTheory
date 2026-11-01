from __future__ import annotations

import os
from dataclasses import dataclass
from typing import Any

import boto3
from botocore.exceptions import ClientError

from tabletheory_py import (
    ModelDefinition,
    Table,
    WritePolicy,
    theorydb_field,
    transition_release_state,
    validate_deploy_authority_metadata,
)

# The runtime-specific table keeps the Go, TypeScript and Python rows from
# sharing seed state in one harness database; their assertions and printed
# parity line are identical.
TABLE_NAME = "release_state_contract_py"


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


def _client():
    return boto3.client(
        "dynamodb",
        endpoint_url=os.environ.get("DYNAMODB_ENDPOINT", "http://localhost:8000"),
        region_name=os.environ.get("AWS_REGION", "us-east-1"),
        aws_access_key_id=os.environ.get("AWS_ACCESS_KEY_ID", "dummy"),
        aws_secret_access_key=os.environ.get("AWS_SECRET_ACCESS_KEY", "dummy"),
    )


def _ensure_table(client) -> None:
    try:
        client.describe_table(TableName=TABLE_NAME)
        return
    except ClientError as err:
        if err.response.get("Error", {}).get("Code") != "ResourceNotFoundException":
            raise

    try:
        client.create_table(
            TableName=TABLE_NAME,
            AttributeDefinitions=[
                {"AttributeName": "PK", "AttributeType": "S"},
                {"AttributeName": "SK", "AttributeType": "S"},
            ],
            KeySchema=[
                {"AttributeName": "PK", "KeyType": "HASH"},
                {"AttributeName": "SK", "KeyType": "RANGE"},
            ],
            BillingMode="PAY_PER_REQUEST",
        )
    except ClientError as err:
        if err.response.get("Error", {}).get("Code") != "ResourceInUseException":
            raise
    client.get_waiter("table_exists").wait(TableName=TABLE_NAME)


def main() -> None:
    client = _client()
    _ensure_table(client)

    actual_table = Table(
        ModelDefinition.from_dataclass(
            ReleaseStateActual,
            table_name=TABLE_NAME,
            write_policy=WritePolicy(mode="mutable", protected_attributes=["pinnedReleaseId"]),
        ),
        client=client,
    )
    event_table = Table(
        ModelDefinition.from_dataclass(
            ReleaseStateEvent,
            table_name=TABLE_NAME,
            write_policy=WritePolicy(mode="write_once"),
        ),
        client=client,
    )
    outbox_table = Table(
        ModelDefinition.from_dataclass(
            ReleaseStateOutbox,
            table_name=TABLE_NAME,
            write_policy=WritePolicy(mode="write_once"),
        ),
        client=client,
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

    partition = f"RELEASE#{service}"
    event = ReleaseStateEvent(
        pk=partition,
        sk=f"EVENT#{observed_at}#{release_id}",
        service=service,
        release_id=release_id,
        event_type="promoted",
        at=observed_at,
        actor="operator@example.com",
        evidence=provenance,
    )
    outbox = ReleaseStateOutbox(
        pk=partition,
        sk=f"OUTBOX#lambda-alias#{release_id}",
        operation="lambda_alias_update",
        idempotency_key=f"{service}:{release_id}",
        requested_state="active",
        next_attempt_at=observed_at,
    )

    actual_table.put(
        ReleaseStateActual(
            pk=partition,
            sk="ACTUAL",
            service=service,
            status="idle",
            version=7,
        )
    )

    transition_release_state(
        actual_table,
        event_table,
        actual_key={"PK": partition, "SK": "ACTUAL"},
        expected_version=7,
        set_values={
            "status": "active",
            "previousReleaseId": "rel_001",
            "provenance": provenance,
            "confidence": confidence,
        },
        event_item=event,
        outbox_item=outbox,
        outbox_table=outbox_table,
    )

    actual = actual_table.get(partition, "ACTUAL", consistent_read=True)
    if actual.status != "active" or actual.previous_release_id != "rel_001":
        raise AssertionError(
            f"actual row status={actual.status} previousReleaseId={actual.previous_release_id}; "
            "want status=active previousReleaseId=rel_001"
        )
    if actual.version != 8:
        raise AssertionError(f"actual version={actual.version}; want 8")

    stored_event = event_table.get(partition, event.sk, consistent_read=True)
    if stored_event.event_type != "promoted" or stored_event.release_id != release_id:
        raise AssertionError(
            f"event row eventType={stored_event.event_type} releaseId={stored_event.release_id}; "
            f"want promoted/{release_id}"
        )

    stored_outbox = outbox_table.get(partition, outbox.sk, consistent_read=True)
    if stored_outbox.operation != "lambda_alias_update":
        raise AssertionError(
            f"outbox operation={stored_outbox.operation}; want lambda_alias_update"
        )

    stale_rejected = False
    try:
        transition_release_state(
            actual_table,
            event_table,
            actual_key={"PK": partition, "SK": "ACTUAL"},
            expected_version=7,
            set_values={
                "status": "active",
                "previousReleaseId": "rel_001",
                "provenance": provenance,
                "confidence": confidence,
            },
            event_item=event,
            outbox_item=outbox,
            outbox_table=outbox_table,
        )
    except Exception:  # noqa: BLE001 - the point is that the stale transition fails
        stale_rejected = True
    if not stale_rejected:
        raise AssertionError("stale expectedVersion transition succeeded; want a condition failure")

    print(
        f"release-state: actual={partition}/ACTUAL version={actual.version} "
        f"status={actual.status} event={event.sk} outbox={outbox.sk}"
    )


if __name__ == "__main__":
    main()
