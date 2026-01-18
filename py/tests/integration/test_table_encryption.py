from __future__ import annotations

import os
import uuid
from dataclasses import dataclass

import boto3

from theorydb_py import ModelDefinition, Table, theorydb_field


def _dynamodb_endpoint() -> str:
    return os.environ.get("DYNAMODB_ENDPOINT", "http://localhost:8000")


def _client():
    return boto3.client(
        "dynamodb",
        endpoint_url=_dynamodb_endpoint(),
        region_name=os.environ.get("AWS_REGION", "us-east-1"),
        aws_access_key_id=os.environ.get("AWS_ACCESS_KEY_ID", "dummy"),
        aws_secret_access_key=os.environ.get("AWS_SECRET_ACCESS_KEY", "dummy"),
    )


class _FakeKms:
    def __init__(self) -> None:
        self._dek = b"\x02" * 32
        self._edk = b"edk"

    def generate_data_key(self, *, KeyId: str, KeySpec: str):  # noqa: N803
        assert KeyId
        assert KeySpec == "AES_256"
        return {"Plaintext": self._dek, "CiphertextBlob": self._edk}

    def decrypt(self, *, CiphertextBlob: bytes, KeyId: str):  # noqa: N803
        assert KeyId
        assert CiphertextBlob == self._edk
        return {"Plaintext": self._dek}


@dataclass(frozen=True)
class SecretNote:
    pk: str = theorydb_field(roles=["pk"])
    sk: str = theorydb_field(roles=["sk"])
    secret: str = theorydb_field(encrypted=True)


def test_table_encrypted_field_round_trip_in_dynamodb_local() -> None:
    table_name = f"theorydb_py_enc_{uuid.uuid4().hex[:12]}"
    client = _client()
    client.create_table(
        TableName=table_name,
        KeySchema=[{"AttributeName": "pk", "KeyType": "HASH"}, {"AttributeName": "sk", "KeyType": "RANGE"}],
        AttributeDefinitions=[
            {"AttributeName": "pk", "AttributeType": "S"},
            {"AttributeName": "sk", "AttributeType": "S"},
        ],
        BillingMode="PAY_PER_REQUEST",
    )
    waiter = client.get_waiter("table_exists")
    waiter.wait(TableName=table_name)

    try:
        model = ModelDefinition.from_dataclass(SecretNote, table_name=table_name)
        table = Table(
            model,
            client=client,
            kms_key_arn="arn:aws:kms:us-east-1:000000000000:key/test",
            kms_client=_FakeKms(),
        )

        item = SecretNote(pk="A", sk="B", secret="shh")
        table.put(item)

        got = table.get("A", "B")
        assert got == item

        raw = client.get_item(
            TableName=table_name,
            Key={"pk": {"S": "A"}, "sk": {"S": "B"}},
            ConsistentRead=True,
        )["Item"]
        assert "M" in raw["secret"]
        assert set(raw["secret"]["M"].keys()) == {"v", "edk", "nonce", "ct"}
    finally:
        client.delete_table(TableName=table_name)
