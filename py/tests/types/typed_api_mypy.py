from __future__ import annotations

from dataclasses import dataclass
from typing import Any

from tabletheory_py.model import ModelDefinition, Role, theorydb_field
from tabletheory_py.table import DynamoDBClientProtocol, Table


@dataclass(frozen=True)
class User:
    pk: str = theorydb_field(name="PK", roles=[Role.PK])
    sk: str = theorydb_field(name="SK", roles=[Role.SK])
    ttl: int = theorydb_field(name="ttl", roles=[Role.TTL], default=0)


class FakeDynamoDBClient:
    def query(self, **kwargs: Any) -> dict[str, Any]:
        return {}

    def scan(self, **kwargs: Any) -> dict[str, Any]:
        return {}

    def batch_get_item(self, **kwargs: Any) -> dict[str, Any]:
        return {}

    def batch_write_item(self, **kwargs: Any) -> dict[str, Any]:
        return {}

    def transact_get_items(self, **kwargs: Any) -> dict[str, Any]:
        return {}

    def transact_write_items(self, **kwargs: Any) -> dict[str, Any]:
        return {}

    def put_item(self, **kwargs: Any) -> dict[str, Any]:
        return {}

    def get_item(self, **kwargs: Any) -> dict[str, Any]:
        return {}

    def delete_item(self, **kwargs: Any) -> dict[str, Any]:
        return {}

    def update_item(self, **kwargs: Any) -> dict[str, Any]:
        return {}


def type_assertions(client: DynamoDBClientProtocol) -> None:
    model = ModelDefinition.from_dataclass(User, table_name="users")
    Table(model, client=client)
    Table(model, client=FakeDynamoDBClient())

    # These ignores must remain used: Role typos and non-DynamoDB clients should fail type checking.
    theorydb_field(roles=[Role.PKK])  # type: ignore[attr-defined]
    Table(model, client=object())  # type: ignore[arg-type]


__all__ = ["FakeDynamoDBClient", "User", "type_assertions"]
