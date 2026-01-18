from __future__ import annotations

from dataclasses import dataclass, field
from datetime import UTC, datetime
from typing import Any

import pytest

from theorydb_py.multiaccount import AccountConfig, MultiAccountSessions


class FakeSTS:
    def __init__(self) -> None:
        self.calls: list[dict[str, Any]] = []
        self._counter = 0

    def assume_role(self, **kwargs: Any) -> dict[str, Any]:
        self.calls.append(dict(kwargs))
        self._counter += 1
        return {
            "Credentials": {
                "AccessKeyId": f"AKIA{self._counter}",
                "SecretAccessKey": f"SECRET{self._counter}",
                "SessionToken": f"TOKEN{self._counter}",
                "Expiration": datetime.fromtimestamp(1000, tz=UTC),
            }
        }


@dataclass
class FakeSession:
    kwargs: dict[str, Any]
    client_calls: list[tuple[str, dict[str, Any]]] = field(default_factory=list)

    def client(self, service_name: str, **kwargs: Any) -> object:
        self.client_calls.append((service_name, dict(kwargs)))
        return object()


def test_multiaccount_sessions_caches_and_refreshes() -> None:
    now_ms = 0.0

    def now() -> float:
        return now_ms

    sts = FakeSTS()

    def session_factory(**kwargs: Any) -> FakeSession:
        return FakeSession(kwargs=dict(kwargs))

    mac = MultiAccountSessions(
        {
            "p1": AccountConfig(
                role_arn="arn:aws:iam::111111111111:role/Test",
                external_id="ext",
                region="us-east-1",
                refresh_before_seconds=200,
            )
        },
        sts_client=sts,
        now=now,
        session_factory=session_factory,
    )

    now_ms = 0.0
    s1 = mac.session("p1")
    assert isinstance(s1, FakeSession)
    assert len(sts.calls) == 1

    now_ms = 700.0
    s2 = mac.session("p1")
    assert s2 is s1
    assert len(sts.calls) == 1

    now_ms = 850.0
    s3 = mac.session("p1")
    assert s3 is not s1
    assert len(sts.calls) == 2


def test_multiaccount_clients_cache_by_service() -> None:
    sts = FakeSTS()
    now_ms = 0.0

    def now() -> float:
        return now_ms

    mac = MultiAccountSessions(
        {
            "p1": AccountConfig(
                role_arn="arn:aws:iam::111111111111:role/Test",
                region="us-east-1",
            )
        },
        sts_client=sts,
        now=now,
        session_factory=lambda **kwargs: FakeSession(kwargs=dict(kwargs)),
    )

    a = mac.client("p1", "dynamodb")
    b = mac.client("p1", "dynamodb")
    assert a is b


def test_multiaccount_unknown_partner_raises() -> None:
    mac = MultiAccountSessions({})
    with pytest.raises(ValueError, match="unknown partner"):
        mac.session("p1")
