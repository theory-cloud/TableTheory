from __future__ import annotations

import pytest

from theorydb_py.testkit import fixed_rand_bytes, no_sleep


def test_no_sleep_is_noop() -> None:
    no_sleep(0.0)
    no_sleep(1.0)


def test_fixed_rand_bytes_repeats_seed_to_requested_length() -> None:
    rand = fixed_rand_bytes(b"\x01\x02")
    assert rand(0) == b""
    assert rand(1) == b"\x01"
    assert rand(2) == b"\x01\x02"
    assert rand(3) == b"\x01\x02\x01"


def test_fixed_rand_bytes_rejects_empty_seed() -> None:
    with pytest.raises(ValueError, match="non-empty"):
        fixed_rand_bytes(b"")


def test_fixed_rand_bytes_rejects_negative_n() -> None:
    rand = fixed_rand_bytes(b"a")
    with pytest.raises(ValueError, match=">= 0"):
        rand(-1)
