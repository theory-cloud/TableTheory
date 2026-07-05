from __future__ import annotations

import time
from collections.abc import Callable, Mapping
from dataclasses import dataclass, field
from datetime import datetime
from typing import Any

import boto3


@dataclass(frozen=True)
class AccountConfig:
    role_arn: str
    external_id: str | None = None
    region: str | None = None
    session_duration_seconds: int = 3600
    session_name: str | None = None
    refresh_before_seconds: int = 300


@dataclass
class _CacheEntry:
    session: Any
    expires_at: float
    clients: dict[str, Any] = field(default_factory=dict)


class MultiAccountSessions:
    def __init__(
        self,
        accounts: Mapping[str, AccountConfig],
        *,
        base_session: Any | None = None,
        sts_client: Any | None = None,
        now: Callable[[], float] | None = None,
        session_factory: Callable[..., Any] | None = None,
    ) -> None:
        self._accounts = dict(accounts)
        self._base_session: Any = base_session or boto3.session.Session()
        self._sts: Any = sts_client or self._base_session.client("sts")
        self._now = now or time.time
        self._session_factory = session_factory or boto3.session.Session
        self._cache: dict[str, _CacheEntry] = {}

    def session(self, partner_id: str) -> Any:
        if not partner_id:
            return self._base_session

        cfg = self._accounts.get(partner_id)
        if cfg is None:
            raise ValueError(f"unknown partner: {partner_id}")

        cached = self._cache.get(partner_id)
        if cached is not None and self._now() < (cached.expires_at - cfg.refresh_before_seconds):
            return cached.session

        region = cfg.region
        resp = self._sts.assume_role(
            RoleArn=cfg.role_arn,
            RoleSessionName=cfg.session_name or f"theorydb-{partner_id}",
            DurationSeconds=cfg.session_duration_seconds,
            **({"ExternalId": cfg.external_id} if cfg.external_id else {}),
        )
        creds = resp.get("Credentials") or {}

        access_key_id = str(creds.get("AccessKeyId") or "")
        secret_access_key = str(creds.get("SecretAccessKey") or "")
        session_token = str(creds.get("SessionToken") or "")
        expiration = creds.get("Expiration")

        if not access_key_id or not secret_access_key or not session_token:
            raise ValueError("AssumeRole did not return credentials")

        expires_at = self._now() + float(cfg.session_duration_seconds)
        if isinstance(expiration, datetime):
            expires_at = expiration.timestamp()

        session = self._session_factory(
            aws_access_key_id=access_key_id,
            aws_secret_access_key=secret_access_key,
            aws_session_token=session_token,
            region_name=region,
        )
        self._cache[partner_id] = _CacheEntry(session=session, expires_at=expires_at)
        return session

    def client(self, partner_id: str, service: str, *, config: Any | None = None) -> Any:
        if not partner_id:
            return self._base_session.client(service, config=config)

        cfg = self._accounts.get(partner_id)
        if cfg is None:
            raise ValueError(f"unknown partner: {partner_id}")

        entry = self._cache.get(partner_id)
        if entry is None or self._now() >= (entry.expires_at - cfg.refresh_before_seconds):
            self.session(partner_id)
            entry = self._cache.get(partner_id)
            if entry is None:
                raise RuntimeError("assume role session cache missing")

        cached_client = entry.clients.get(service)
        if cached_client is not None:
            return cached_client

        client = entry.session.client(service, region_name=cfg.region, config=config)
        entry.clients[service] = client
        return client
