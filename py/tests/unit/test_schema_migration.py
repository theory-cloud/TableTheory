from __future__ import annotations

from typing import Any, cast

import pytest

import tabletheory_py.schema_migration as sm
from tabletheory_py import ModelDefinition
from tabletheory_py.schema_migration import (
    add_field,
    chain_transforms,
    copy_all_fields,
    remove_field,
    rename_field,
)


def _item() -> dict[str, object]:
    return {
        "PK": {"S": "USER#1"},
        "SK": {"S": "v1"},
        "name": {"S": "Ada"},
    }


def test_copy_all_fields_passes_attributes_through_unchanged() -> None:
    item = _item()
    out = copy_all_fields()(item)
    assert out == item
    assert out is not item


def test_rename_field_renames_one_attribute_and_preserves_value() -> None:
    out = rename_field("name", "displayName")(_item())
    assert out == {
        "PK": {"S": "USER#1"},
        "SK": {"S": "v1"},
        "displayName": {"S": "Ada"},
    }


def test_add_field_adds_an_attribute() -> None:
    out = add_field("status", {"S": "active"})(_item())
    assert out["status"] == {"S": "active"}
    assert out["name"] == {"S": "Ada"}


def test_remove_field_drops_an_attribute() -> None:
    out = remove_field("name")(_item())
    assert "name" not in out
    assert out["PK"] == {"S": "USER#1"}


def test_chain_transforms_composes_left_to_right() -> None:
    out = chain_transforms(
        rename_field("name", "displayName"),
        add_field("status", {"S": "active"}),
        remove_field("SK"),
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
        transform=add_field("migrated", {"BOOL": True}),
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
