from __future__ import annotations

from dataclasses import dataclass
from typing import Any

from theorydb_py.model import ModelDefinition, theorydb_field
from theorydb_py.table import Table


class UserID:
    def __init__(self, raw: str) -> None:
        self.raw = raw

    def __repr__(self) -> str:  # pragma: no cover
        return f"UserID({self.raw!r})"


class UserIDConverter:
    def to_dynamodb(self, value: Any) -> Any:
        if not isinstance(value, UserID):
            raise TypeError("expected UserID")
        return value.raw

    def from_dynamodb(self, value: Any) -> Any:
        return UserID(str(value))


@dataclass(frozen=True)
class Item:
    pk: str = theorydb_field(name="PK", roles=["pk"])
    user_id: UserID = theorydb_field(name="userId", converter=UserIDConverter())


def test_custom_converter_round_trips_via_table_helpers() -> None:
    model = ModelDefinition.from_dataclass(Item, table_name="tbl")
    table: Table[Item] = Table(model, client=object())

    stored = table._to_item(Item(pk="A", user_id=UserID("u1")))
    assert stored["userId"] == {"S": "u1"}

    loaded = table._from_item(stored)
    assert isinstance(loaded.user_id, UserID)
    assert loaded.user_id.raw == "u1"


@dataclass(frozen=True)
class Keyed:
    pk: UserID = theorydb_field(name="PK", roles=["pk"], converter=UserIDConverter())
    value: str = theorydb_field(name="value", default="")


def test_custom_converter_applies_to_keys() -> None:
    model = ModelDefinition.from_dataclass(Keyed, table_name="tbl")
    table: Table[Keyed] = Table(model, client=object())

    key = table._to_key(UserID("A"), None)
    assert key == {"PK": {"S": "A"}}
