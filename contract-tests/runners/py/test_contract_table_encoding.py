from __future__ import annotations

from dataclasses import dataclass

from theorydb_py import ModelDefinition, Table, theorydb_field


@dataclass(frozen=True)
class _SetThing:
    pk: str = theorydb_field(roles=["pk"])
    tags: set[str] = theorydb_field(set_=True, default_factory=set)


def test_empty_set_encodes_as_null_when_not_omitempty() -> None:
    model = ModelDefinition.from_dataclass(_SetThing, table_name="tbl")
    table: Table[_SetThing] = Table(model, client=object())

    item = table._to_item(_SetThing(pk="A"))
    assert item["tags"] == {"NULL": True}


@dataclass(frozen=True)
class _ReservedWordThing:
    pk: str = theorydb_field(roles=["pk"])
    sk: str = theorydb_field(roles=["sk"])
    size: int = theorydb_field(name="size", default=0)


def test_reserved_word_attributes_are_referenced_via_names_map() -> None:
    model = ModelDefinition.from_dataclass(_ReservedWordThing, table_name="tbl")
    table: Table[_ReservedWordThing] = Table(model, client=object())

    req = table._build_update_request("A", "1", {"size": 1})
    assert "#d_size" in req["UpdateExpression"]
    assert req["ExpressionAttributeNames"]["#d_size"] == "size"

