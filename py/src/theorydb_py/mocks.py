from __future__ import annotations

from collections.abc import Callable, Mapping
from dataclasses import dataclass
from typing import Any


class _AnySentinel:
    def __repr__(self) -> str:  # pragma: no cover
        return "ANY"


ANY: Any = _AnySentinel()


def _assert_match(expected: Any, actual: Any, *, path: str) -> None:
    if expected is ANY:
        return

    if isinstance(expected, dict):
        if not isinstance(actual, dict):
            raise AssertionError(f"{path}: expected dict, got {type(actual).__name__}")
        for k, v in expected.items():
            if k not in actual:
                raise AssertionError(f"{path}: missing key {k!r}")
            _assert_match(v, actual[k], path=f"{path}.{k}")
        return

    if isinstance(expected, list):
        if not isinstance(actual, list):
            raise AssertionError(f"{path}: expected list, got {type(actual).__name__}")
        if len(expected) != len(actual):
            raise AssertionError(f"{path}: expected {len(expected)} items, got {len(actual)}")
        for i, (e, a) in enumerate(zip(expected, actual, strict=True)):
            _assert_match(e, a, path=f"{path}[{i}]")
        return

    if expected != actual:
        raise AssertionError(f"{path}: expected {expected!r}, got {actual!r}")


@dataclass(frozen=True)
class ExpectedCall:
    method: str
    expected: Mapping[str, Any] | Callable[[Mapping[str, Any]], None] | None = None
    response: Mapping[str, Any] | None = None
    error: Exception | None = None


class FakeDynamoDBClient:
    def __init__(self) -> None:
        self._expected: list[ExpectedCall] = []
        self.calls: list[tuple[str, dict[str, Any]]] = []

    def expect(
        self,
        method: str,
        expected: Mapping[str, Any] | Callable[[Mapping[str, Any]], None] | None = None,
        *,
        response: Mapping[str, Any] | None = None,
        error: Exception | None = None,
    ) -> None:
        self._expected.append(ExpectedCall(method=method, expected=expected, response=response, error=error))

    def assert_no_pending(self) -> None:
        if self._expected:
            raise AssertionError(f"pending expected calls: {self._expected!r}")

    def _handle(self, method: str, req: dict[str, Any]) -> Mapping[str, Any]:
        self.calls.append((method, dict(req)))
        if not self._expected:
            raise AssertionError(f"unexpected call: {method}")

        call = self._expected.pop(0)
        if call.method != method:
            raise AssertionError(f"expected {call.method}, got {method}")

        if callable(call.expected):
            call.expected(req)
        elif call.expected is not None:
            _assert_match(dict(call.expected), req, path=method)

        if call.error is not None:
            raise call.error

        return dict(call.response or {})

    def put_item(self, **kwargs: Any) -> Mapping[str, Any]:
        return self._handle("put_item", kwargs)

    def get_item(self, **kwargs: Any) -> Mapping[str, Any]:
        return self._handle("get_item", kwargs)

    def update_item(self, **kwargs: Any) -> Mapping[str, Any]:
        return self._handle("update_item", kwargs)

    def delete_item(self, **kwargs: Any) -> Mapping[str, Any]:
        return self._handle("delete_item", kwargs)

    def query(self, **kwargs: Any) -> Mapping[str, Any]:
        return self._handle("query", kwargs)

    def scan(self, **kwargs: Any) -> Mapping[str, Any]:
        return self._handle("scan", kwargs)

    def batch_get_item(self, **kwargs: Any) -> Mapping[str, Any]:
        return self._handle("batch_get_item", kwargs)

    def batch_write_item(self, **kwargs: Any) -> Mapping[str, Any]:
        return self._handle("batch_write_item", kwargs)

    def transact_write_items(self, **kwargs: Any) -> Mapping[str, Any]:
        return self._handle("transact_write_items", kwargs)

    def create_table(self, **kwargs: Any) -> Mapping[str, Any]:
        return self._handle("create_table", kwargs)

    def delete_table(self, **kwargs: Any) -> Mapping[str, Any]:
        return self._handle("delete_table", kwargs)

    def describe_table(self, **kwargs: Any) -> Mapping[str, Any]:
        return self._handle("describe_table", kwargs)


class FakeKmsClient:
    def __init__(self, *, plaintext_key: bytes, ciphertext_blob: bytes) -> None:
        self.plaintext_key = plaintext_key
        self.ciphertext_blob = ciphertext_blob
        self.calls: list[tuple[str, dict[str, Any]]] = []

    def generate_data_key(self, **kwargs: Any) -> dict[str, Any]:
        self.calls.append(("generate_data_key", dict(kwargs)))
        return {
            "Plaintext": self.plaintext_key,
            "CiphertextBlob": self.ciphertext_blob,
        }

    def decrypt(self, **kwargs: Any) -> dict[str, Any]:
        self.calls.append(("decrypt", dict(kwargs)))
        return {"Plaintext": self.plaintext_key}
