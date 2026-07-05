from __future__ import annotations

from collections.abc import Mapping
from typing import Any, Protocol


class DynamoDBClientProtocol(Protocol):
    def query(self, **kwargs: Any) -> Mapping[str, Any]:
        pass

    def scan(self, **kwargs: Any) -> Mapping[str, Any]:
        pass

    def batch_get_item(self, **kwargs: Any) -> Mapping[str, Any]:
        pass

    def batch_write_item(self, **kwargs: Any) -> Mapping[str, Any]:
        pass

    def transact_get_items(self, **kwargs: Any) -> Mapping[str, Any]:
        pass

    def transact_write_items(self, **kwargs: Any) -> Mapping[str, Any]:
        pass

    def put_item(self, **kwargs: Any) -> Mapping[str, Any]:
        pass

    def get_item(self, **kwargs: Any) -> Mapping[str, Any]:
        pass

    def delete_item(self, **kwargs: Any) -> Mapping[str, Any]:
        pass

    def update_item(self, **kwargs: Any) -> Mapping[str, Any]:
        pass
