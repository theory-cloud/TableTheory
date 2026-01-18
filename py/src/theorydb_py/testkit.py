from __future__ import annotations

from collections.abc import Callable

from .mocks import ANY, FakeDynamoDBClient, FakeKmsClient


def fixed_rand_bytes(seed: bytes) -> Callable[[int], bytes]:
    if not seed:
        raise ValueError("seed must be non-empty")

    def rand(n: int) -> bytes:
        if n < 0:
            raise ValueError("n must be >= 0")
        if n == 0:
            return b""
        repeats = (n + len(seed) - 1) // len(seed)
        return (seed * repeats)[:n]

    return rand


def no_sleep(_: float) -> None:
    return None


__all__ = [
    "ANY",
    "FakeDynamoDBClient",
    "FakeKmsClient",
    "fixed_rand_bytes",
    "no_sleep",
]
