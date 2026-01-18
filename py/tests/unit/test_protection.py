from __future__ import annotations

from theorydb_py.protection import ConcurrencyLimiter, SimpleLimiter


def test_simple_limiter_refills_tokens() -> None:
    now = 0.0

    def clock() -> float:
        return now

    limiter = SimpleLimiter(10, 2, now=clock)  # 10 rps, burst 2
    assert limiter.allow() is True
    assert limiter.allow() is True
    assert limiter.allow() is False

    now += 0.1  # 1 token per 0.1s
    assert limiter.allow() is True
    assert limiter.allow() is False


def test_concurrency_limiter_try_acquire_and_release() -> None:
    limiter = ConcurrencyLimiter(2)
    assert limiter.try_acquire() is True
    assert limiter.try_acquire() is True
    assert limiter.try_acquire() is False
    limiter.release()
    assert limiter.try_acquire() is True
    limiter.release()
    limiter.release()


def test_concurrency_limiter_context_manager() -> None:
    limiter = ConcurrencyLimiter(1)
    with limiter.acquire():
        assert limiter.try_acquire() is False
    assert limiter.try_acquire() is True
    limiter.release()
