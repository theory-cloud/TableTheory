from __future__ import annotations

import threading
import time
from collections.abc import Callable, Iterator
from contextlib import contextmanager


class SimpleLimiter:
    def __init__(self, rps: float, burst: int, *, now: Callable[[], float] | None = None) -> None:
        if not isinstance(rps, (int, float)) or rps <= 0:
            raise ValueError("rps must be > 0")
        if not isinstance(burst, int) or burst <= 0:
            raise ValueError("burst must be > 0")

        self._now = now or time.monotonic
        self._tokens = burst
        self._max_tokens = burst
        self._refill_interval = 1.0 / float(rps)
        self._last_refill = self._now()
        self._lock = threading.Lock()

    def allow(self) -> bool:
        with self._lock:
            now = self._now()
            elapsed = now - self._last_refill
            tokens_to_add = int(elapsed / self._refill_interval)

            if tokens_to_add > 0:
                self._tokens = min(self._max_tokens, self._tokens + tokens_to_add)
                self._last_refill = now

            if self._tokens > 0:
                self._tokens -= 1
                return True

            return False


class ConcurrencyLimiter:
    def __init__(self, max_concurrent: int) -> None:
        if not isinstance(max_concurrent, int) or max_concurrent <= 0:
            raise ValueError("max_concurrent must be > 0")
        self._sem = threading.BoundedSemaphore(max_concurrent)

    def try_acquire(self) -> bool:
        return self._sem.acquire(blocking=False)

    def release(self) -> None:
        self._sem.release()

    @contextmanager
    def acquire(self) -> Iterator[None]:
        self._sem.acquire()
        try:
            yield
        finally:
            self._sem.release()
