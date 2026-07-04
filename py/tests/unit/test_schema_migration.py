from __future__ import annotations

from theorydb_py.schema_migration import (
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
