from __future__ import annotations

from dataclasses import dataclass
from typing import Any, cast

import pytest

import tabletheory_py.schema_migration as sm
from tabletheory_py import MigrationEncryptionError, ModelDefinition, theorydb_field
from tabletheory_py.encryption import decrypt_attribute_value


def _item() -> dict[str, object]:
    return {
        "PK": {"S": "USER#1"},
        "SK": {"S": "v1"},
        "name": {"S": "Ada"},
    }


def test_copy_all_fields_passes_attributes_through_unchanged() -> None:
    item = _item()
    out = sm.copy_all_fields()(item)
    assert out == item
    assert out is not item


def test_rename_field_renames_one_attribute_and_preserves_value() -> None:
    out = sm.rename_field("name", "displayName")(_item())
    assert out == {
        "PK": {"S": "USER#1"},
        "SK": {"S": "v1"},
        "displayName": {"S": "Ada"},
    }


def test_add_field_adds_an_attribute() -> None:
    out = sm.add_field("status", {"S": "active"})(_item())
    assert out["status"] == {"S": "active"}
    assert out["name"] == {"S": "Ada"}


def test_remove_field_drops_an_attribute() -> None:
    out = sm.remove_field("name")(_item())
    assert "name" not in out
    assert out["PK"] == {"S": "USER#1"}


def test_chain_transforms_composes_left_to_right() -> None:
    out = sm.chain_transforms(
        sm.rename_field("name", "displayName"),
        sm.add_field("status", {"S": "active"}),
        sm.remove_field("SK"),
    )(_item())
    assert out == {
        "PK": {"S": "USER#1"},
        "displayName": {"S": "Ada"},
        "status": {"S": "active"},
    }


class _FakeModel:
    def __init__(self, table_name: str) -> None:
        self.table_name = table_name


class _FakeMigrationClient:
    def __init__(self, pages: list[dict[str, object]]) -> None:
        self._pages = pages
        self.scan_calls: list[dict[str, object]] = []
        self.batch_write_calls: list[dict[str, object]] = []
        self.put_items: list[dict[str, object]] = []

    def scan(self, **kwargs: object) -> dict[str, object]:
        self.scan_calls.append(dict(kwargs))
        assert self._pages
        return self._pages.pop(0)

    def batch_write_item(self, **kwargs: object) -> dict[str, object]:
        self.batch_write_calls.append(dict(kwargs))
        request_items = kwargs["RequestItems"]
        assert isinstance(request_items, dict)
        table_name, requests = next(iter(request_items.items()))
        if table_name == "notes_v2":
            return {"UnprocessedItems": {table_name: requests}}
        return {"UnprocessedItems": {}}

    def put_item(self, **kwargs: object) -> None:
        self.put_items.append(dict(kwargs))


def _model(table_name: str) -> ModelDefinition[Any]:
    return cast(ModelDefinition[Any], _FakeModel(table_name))


def test_auto_migrate_ensures_target_without_data_copy(monkeypatch: pytest.MonkeyPatch) -> None:
    calls: list[tuple[str, object]] = []

    def fake_ensure(model: ModelDefinition[Any], *, client: object) -> None:
        calls.append((model.table_name or "", client))

    monkeypatch.setattr(sm, "ensure_table", fake_ensure)
    client = _FakeMigrationClient([])

    sm.auto_migrate(_model("notes"), client=client, sleep=lambda _: None)

    assert calls == [("notes", client)]
    assert client.scan_calls == []


def test_auto_migrate_backup_and_copy_retries_unprocessed_items(monkeypatch: pytest.MonkeyPatch) -> None:
    calls: list[tuple[str, str, str | None]] = []

    def fake_describe(model: ModelDefinition[Any], *, client: object) -> object:
        calls.append(("describe", model.table_name or "", None))
        return {}

    def fake_create(model: ModelDefinition[Any], *, client: object, table_name: str | None = None) -> None:
        calls.append(("create", model.table_name or "", table_name))

    def fake_ensure(model: ModelDefinition[Any], *, client: object) -> None:
        calls.append(("ensure", model.table_name or "", None))

    monkeypatch.setattr(sm, "describe_table", fake_describe)
    monkeypatch.setattr(sm, "create_table", fake_create)
    monkeypatch.setattr(sm, "ensure_table", fake_ensure)

    item1 = {"PK": {"S": "NOTE#1"}, "SK": {"S": "v1"}}
    item2 = {"PK": {"S": "NOTE#2"}, "SK": {"S": "v1"}}
    pages: list[dict[str, object]] = [
        {"Items": [item1], "LastEvaluatedKey": {"PK": {"S": "NOTE#1"}, "SK": {"S": "v1"}}},
        {"Items": [item2]},
        {"Items": [item1], "LastEvaluatedKey": {"PK": {"S": "NOTE#1"}, "SK": {"S": "v1"}}},
        {"Items": [item2]},
    ]
    client = _FakeMigrationClient(pages)
    sleeps: list[float] = []

    sm.auto_migrate(
        _model("notes_v1"),
        target_model=_model("notes_v2"),
        client=client,
        backup_table="notes_backup",
        data_copy=True,
        batch_size=1,
        transform=sm.add_field("migrated", {"BOOL": True}),
        sleep=sleeps.append,
    )

    assert calls == [
        ("describe", "notes_v1", None),
        ("create", "notes_v1", "notes_backup"),
        ("ensure", "notes_v2", None),
    ]
    assert [call["TableName"] for call in client.scan_calls] == ["notes_v1"] * 4
    assert len(client.batch_write_calls) == 12
    assert sleeps == [0.1, 0.4, 0.9, 1.6, 0.1, 0.4, 0.9, 1.6]
    assert client.put_items == [
        {"TableName": "notes_v2", "Item": {**item1, "migrated": {"BOOL": True}}},
        {"TableName": "notes_v2", "Item": {**item2, "migrated": {"BOOL": True}}},
    ]


def test_auto_migrate_rejects_missing_table_name(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(sm, "ensure_table", lambda model, *, client: None)

    with pytest.raises(ValueError, match="table_name is required"):
        sm.auto_migrate(_model(""), client=_FakeMigrationClient([]), data_copy=True)


@dataclass(frozen=True)
class _PlainSource:
    pk: str = theorydb_field(roles=["pk"])
    sk: str = theorydb_field(roles=["sk"])
    data: str = theorydb_field()


@dataclass(frozen=True)
class _PlainTarget:
    pk: str = theorydb_field(roles=["pk"])
    sk: str = theorydb_field(roles=["sk"])
    data: str = theorydb_field()


@dataclass(frozen=True)
class _EncryptedTarget:
    pk: str = theorydb_field(roles=["pk"])
    sk: str = theorydb_field(roles=["sk"])
    data: str = theorydb_field(encrypted=True)


@dataclass(frozen=True)
class _EncryptedOldSource:
    pk: str = theorydb_field(roles=["pk"])
    sk: str = theorydb_field(roles=["sk"])
    old: str = theorydb_field(encrypted=True)


@dataclass(frozen=True)
class _EncryptedNewTarget:
    pk: str = theorydb_field(roles=["pk"])
    sk: str = theorydb_field(roles=["sk"])
    new: str = theorydb_field(encrypted=True)


@dataclass(frozen=True)
class _EncryptedXSource:
    pk: str = theorydb_field(roles=["pk"])
    sk: str = theorydb_field(roles=["sk"])
    x: str = theorydb_field(encrypted=True)


@dataclass(frozen=True)
class _PlainXTarget:
    pk: str = theorydb_field(roles=["pk"])
    sk: str = theorydb_field(roles=["sk"])
    x: str = theorydb_field()


class _FakeKms:
    def __init__(self) -> None:
        self._dek = b"\x02" * 32
        self._edk = b"deterministic-edk"

    def generate_data_key(self, *, KeyId: str, KeySpec: str) -> dict[str, bytes]:  # noqa: N803
        assert KeyId
        assert KeySpec == "AES_256"
        return {"Plaintext": self._dek, "CiphertextBlob": self._edk}

    def decrypt(self, *, CiphertextBlob: bytes, KeyId: str) -> dict[str, bytes]:  # noqa: N803
        assert KeyId
        assert CiphertextBlob == self._edk
        return {"Plaintext": self._dek}


class _RecordingClient:
    def __init__(self, pages: list[dict[str, object]]) -> None:
        self._pages = list(pages)
        self.scan_calls: list[dict[str, object]] = []
        self.batch_write_calls: list[dict[str, object]] = []
        self.put_items: list[dict[str, object]] = []

    def scan(self, **kwargs: object) -> dict[str, object]:
        self.scan_calls.append(dict(kwargs))
        assert self._pages
        return self._pages.pop(0)

    def batch_write_item(self, **kwargs: object) -> dict[str, object]:
        self.batch_write_calls.append(dict(kwargs))
        return {"UnprocessedItems": {}}

    def put_item(self, **kwargs: object) -> None:
        self.put_items.append(dict(kwargs))


def _real_model(model_type: type[object], table_name: str) -> ModelDefinition[Any]:
    return ModelDefinition.from_dataclass(cast(Any, model_type), table_name=table_name)


def _envelope() -> dict[str, object]:
    return {
        "M": {
            "v": {"N": "1"},
            "edk": {"B": b"edk"},
            "nonce": {"B": b"0123456789ab"},
            "ct": {"B": b"ct"},
        }
    }


def _batched_items(client: _RecordingClient) -> list[dict[str, Any]]:
    items: list[dict[str, Any]] = []
    for call in client.batch_write_calls:
        request_items = call["RequestItems"]
        assert isinstance(request_items, dict)
        for requests in request_items.values():
            for request in requests:
                items.append(request["PutRequest"]["Item"])
    return items


def test_auto_migrate_requires_encryption_for_new_encrypted_target_attribute(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    lifecycle: list[tuple[str, str]] = []

    def fake_describe(model: ModelDefinition[Any], *, client: object) -> object:
        lifecycle.append(("describe", model.table_name or ""))
        return {}

    def fake_create(model: ModelDefinition[Any], *, client: object, table_name: str | None = None) -> None:
        lifecycle.append(("create", table_name or ""))

    def fake_ensure(model: ModelDefinition[Any], *, client: object) -> None:
        lifecycle.append(("ensure", model.table_name or ""))

    monkeypatch.setattr(sm, "describe_table", fake_describe)
    monkeypatch.setattr(sm, "create_table", fake_create)
    monkeypatch.setattr(sm, "ensure_table", fake_ensure)

    client = _RecordingClient([])
    with pytest.raises(
        MigrationEncryptionError,
        match=r"requires encryption for attribute\(s\) data; supply kms_key_arn",
    ):
        sm.auto_migrate(
            _real_model(_PlainSource, "plain_src"),
            target_model=_real_model(_EncryptedTarget, "enc_dst"),
            client=client,
            backup_table="plain_src_backup",
            data_copy=True,
        )

    assert lifecycle == []
    assert client.scan_calls == []
    assert client.batch_write_calls == []
    assert client.put_items == []


def test_auto_migrate_encrypts_new_target_attribute_with_configured_kms(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(sm, "ensure_table", lambda model, *, client: None)
    kms = _FakeKms()
    key_arn = "arn:aws:kms:us-east-1:000000000000:key/test"
    item = {"PK": {"S": "USER#1"}, "SK": {"S": "v1"}, "data": {"S": "123-45-6789"}}
    client = _RecordingClient([{"Items": [item]}])

    sm.auto_migrate(
        _real_model(_PlainSource, "plain_src"),
        target_model=_real_model(_EncryptedTarget, "enc_dst"),
        client=client,
        data_copy=True,
        kms_key_arn=key_arn,
        kms_client=kms,
        rand_bytes=lambda size: b"\x07" * size,
    )

    written = _batched_items(client)
    assert len(written) == 1
    stored = written[0]
    assert stored["PK"] == {"S": "USER#1"}
    assert stored["SK"] == {"S": "v1"}
    assert "S" not in stored["data"]
    envelope = stored["data"]["M"]
    assert set(envelope.keys()) == {"v", "edk", "nonce", "ct"}
    assert envelope["v"] == {"N": "1"}
    assert client.put_items == []

    decrypted = decrypt_attribute_value(
        {
            "v": int(envelope["v"]["N"]),
            "edk": envelope["edk"]["B"],
            "nonce": envelope["nonce"]["B"],
            "ct": envelope["ct"]["B"],
        },
        attr_name="data",
        kms_key_arn=key_arn,
        kms_client=kms,
    )
    assert decrypted == {"S": "123-45-6789"}


def test_auto_migrate_rejects_encrypted_rename_without_reencryption(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(sm, "ensure_table", lambda model, *, client: None)
    item = {"PK": {"S": "USER#1"}, "SK": {"S": "v1"}, "old": _envelope()}
    client = _RecordingClient([{"Items": [item]}])

    with pytest.raises(MigrationEncryptionError, match='cannot re-encrypt attribute "new"'):
        sm.auto_migrate(
            _real_model(_EncryptedOldSource, "src"),
            target_model=_real_model(_EncryptedNewTarget, "dst"),
            client=client,
            transform=sm.rename_field("old", "new"),
            data_copy=True,
            kms_key_arn="arn:aws:kms:us-east-1:000000000000:key/test",
            kms_client=_FakeKms(),
            rand_bytes=lambda size: b"\x07" * size,
        )

    assert client.batch_write_calls == []
    assert client.put_items == []


def test_auto_migrate_rejects_dropping_encryption_on_copied_envelope(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(sm, "ensure_table", lambda model, *, client: None)
    item = {"PK": {"S": "USER#1"}, "SK": {"S": "v1"}, "x": _envelope()}
    client = _RecordingClient([{"Items": [item]}])

    with pytest.raises(MigrationEncryptionError, match='cannot copy encrypted attribute "x"'):
        sm.auto_migrate(
            _real_model(_EncryptedXSource, "src"),
            target_model=_real_model(_PlainXTarget, "dst"),
            client=client,
            transform=sm.copy_all_fields(),
            data_copy=True,
        )

    assert client.batch_write_calls == []
    assert client.put_items == []


def test_auto_migrate_plain_models_copy_items_verbatim(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(sm, "ensure_table", lambda model, *, client: None)
    items = [
        {"PK": {"S": f"USER#{index}"}, "SK": {"S": "v1"}, "data": {"S": f"n{index}"}} for index in range(30)
    ]
    client = _RecordingClient([{"Items": items}])

    sm.auto_migrate(
        _real_model(_PlainSource, "plain_src"),
        target_model=_real_model(_PlainTarget, "plain_dst"),
        client=client,
        data_copy=True,
    )

    assert _batched_items(client) == items
    assert client.put_items == []
    batch_sizes: list[int] = []
    for call in client.batch_write_calls:
        request_items = call["RequestItems"]
        assert isinstance(request_items, dict)
        batch_sizes.append(sum(len(requests) for requests in request_items.values()))
    assert batch_sizes == [25, 5]


def test_auto_migrate_encrypted_target_without_data_copy_is_not_refused(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    ensured: list[str] = []

    def fake_ensure(model: ModelDefinition[Any], *, client: object) -> None:
        ensured.append(model.table_name or "")

    monkeypatch.setattr(sm, "ensure_table", fake_ensure)

    client = _RecordingClient([])
    sm.auto_migrate(
        _real_model(_PlainSource, "plain_src"),
        target_model=_real_model(_EncryptedTarget, "enc_dst"),
        client=client,
        data_copy=False,
    )

    assert ensured == ["enc_dst"]
    assert client.scan_calls == []
    assert client.batch_write_calls == []
    assert client.put_items == []
