from __future__ import annotations

from dataclasses import dataclass

import pytest

from theorydb_py import ModelDefinition, Table, theorydb_field
from theorydb_py.mocks import ANY, FakeDynamoDBClient, FakeKmsClient


@dataclass(frozen=True)
class Note:
    pk: str = theorydb_field(roles=["pk"])
    sk: str = theorydb_field(roles=["sk"])
    value: int = theorydb_field()


def test_fake_dynamodb_client_records_and_matches_put_item() -> None:
    client = FakeDynamoDBClient()
    client.expect("put_item", {"TableName": "notes", "Item": ANY})

    model = ModelDefinition.from_dataclass(Note, table_name="notes")
    table = Table(model, client=client)

    table.put(Note(pk="A", sk="B", value=1))

    client.assert_no_pending()
    assert client.calls[0][0] == "put_item"


@dataclass(frozen=True)
class SecretNote:
    pk: str = theorydb_field(roles=["pk"])
    sk: str = theorydb_field(roles=["sk"])
    secret: str = theorydb_field(encrypted=True)


def _as_bytes(value: object) -> bytes:
    if isinstance(value, (bytes, bytearray)):
        return bytes(value)
    return bytes(value)  # type: ignore[arg-type]


def test_table_encryption_can_be_deterministic_with_fake_clients() -> None:
    fixed_nonce = b"\x02" * 12
    plaintext_key = b"\x01" * 32
    edk = b"ciphertext-data-key"

    kms = FakeKmsClient(plaintext_key=plaintext_key, ciphertext_blob=edk)
    ddb = FakeDynamoDBClient()

    def validate_put(req: dict) -> None:
        item = req["Item"]
        env = item["secret"]["M"]

        assert env["v"]["N"] == "1"
        assert _as_bytes(env["edk"]["B"]) == edk
        assert _as_bytes(env["nonce"]["B"]) == fixed_nonce
        assert _as_bytes(env["ct"]["B"])

    ddb.expect("put_item", validate_put)

    model = ModelDefinition.from_dataclass(SecretNote, table_name="notes")
    table = Table(
        model,
        client=ddb,
        kms_key_arn="arn:aws:kms:us-east-1:111111111111:key/test",
        kms_client=kms,
        rand_bytes=lambda n: fixed_nonce[:n],
    )

    table.put(SecretNote(pk="A", sk="B", secret="top-secret"))

    ddb.assert_no_pending()
    assert [c[0] for c in kms.calls] == ["generate_data_key"]


def test_fake_dynamodb_client_asserts_pending_calls() -> None:
    client = FakeDynamoDBClient()
    client.expect("query")
    with pytest.raises(AssertionError, match="pending expected calls"):
        client.assert_no_pending()


def test_fake_dynamodb_client_rejects_unexpected_calls() -> None:
    client = FakeDynamoDBClient()
    with pytest.raises(AssertionError, match="unexpected call: query"):
        client.query()


def test_fake_dynamodb_client_rejects_wrong_method_order() -> None:
    client = FakeDynamoDBClient()
    client.expect("scan")
    with pytest.raises(AssertionError, match="expected scan, got query"):
        client.query()


@pytest.mark.parametrize(
    ("expected", "req", "match"),
    [
        ({"a": 1}, {"a": 2}, "expected 1"),
        ({"a": 1}, {}, "missing key"),
        ({"a": {"b": 1}}, {"a": "nope"}, "expected dict"),
        ({"a": [1]}, {"a": "nope"}, "expected list"),
        ({"a": [1, 2]}, {"a": [1]}, "expected 2 items"),
        ({"a": [1]}, {"a": [2]}, "expected 1"),
    ],
)
def test_fake_dynamodb_client_strict_matching(expected: dict, req: dict, match: str) -> None:
    client = FakeDynamoDBClient()
    client.expect("query", expected)
    with pytest.raises(AssertionError, match=match):
        client.query(**req)


def test_fake_dynamodb_client_can_inject_errors() -> None:
    client = FakeDynamoDBClient()
    err = RuntimeError("boom")
    client.expect("query", error=err)
    with pytest.raises(RuntimeError, match="boom"):
        client.query()


def test_fake_dynamodb_client_dispatch_helpers() -> None:
    client = FakeDynamoDBClient()
    client.expect("get_item", response={"ok": True})
    client.expect("update_item", response={"ok": True})
    client.expect("delete_item", response={"ok": True})
    client.expect("batch_get_item", response={"ok": True})
    client.expect("batch_write_item", response={"ok": True})
    client.expect("transact_write_items", response={"ok": True})

    assert client.get_item() == {"ok": True}
    assert client.update_item() == {"ok": True}
    assert client.delete_item() == {"ok": True}
    assert client.batch_get_item() == {"ok": True}
    assert client.batch_write_item() == {"ok": True}
    assert client.transact_write_items() == {"ok": True}
    client.assert_no_pending()


def test_fake_kms_client_decrypt_records_calls() -> None:
    plaintext_key = b"\x01" * 32
    kms = FakeKmsClient(plaintext_key=plaintext_key, ciphertext_blob=b"x")
    out = kms.decrypt(CiphertextBlob=b"ciphertext")
    assert out["Plaintext"] == plaintext_key
    assert kms.calls[-1][0] == "decrypt"
