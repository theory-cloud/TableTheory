from __future__ import annotations

import time
import uuid
from collections.abc import Callable
from dataclasses import dataclass
from typing import Any

from botocore.exceptions import ClientError

from .aws_errors import map_client_error
from .errors import ConditionFailedError, LeaseHeldError, LeaseNotOwnedError


@dataclass(frozen=True)
class LeaseKey:
    pk: str
    sk: str


@dataclass(frozen=True)
class Lease:
    key: LeaseKey
    token: str
    expires_at: int


class LeaseManager:
    def __init__(
        self,
        *,
        client: Any,
        table_name: str,
        now: Callable[[], float] | None = None,
        token: Callable[[], str] | None = None,
        pk_attr: str = "pk",
        sk_attr: str = "sk",
        token_attr: str = "lease_token",
        expires_at_attr: str = "lease_expires_at",
        ttl_attr: str = "ttl",
        ttl_buffer_seconds: int = 60 * 60,
    ) -> None:
        if client is None:
            raise ValueError("client is required")
        if not table_name:
            raise ValueError("table_name is required")

        self._client = client
        self._table_name = table_name

        self._now = now or time.time
        self._token = token or (lambda: str(uuid.uuid4()))

        self._pk_attr = pk_attr
        self._sk_attr = sk_attr
        self._token_attr = token_attr
        self._expires_at_attr = expires_at_attr
        self._ttl_attr = ttl_attr
        self._ttl_buffer_seconds = int(ttl_buffer_seconds)

    def lock_key(self, pk: str, sk: str = "LOCK") -> LeaseKey:
        return LeaseKey(pk=pk, sk=sk)

    def acquire(self, key: LeaseKey, *, lease_seconds: int) -> Lease:
        if not key.pk or not key.sk:
            raise ValueError("key.pk and key.sk are required")
        if lease_seconds <= 0:
            raise ValueError("lease_seconds must be > 0")

        now = int(self._now())
        expires_at = now + int(lease_seconds)
        token = self._token()

        item: dict[str, Any] = {
            self._pk_attr: {"S": key.pk},
            self._sk_attr: {"S": key.sk},
            self._token_attr: {"S": token},
            self._expires_at_attr: {"N": str(expires_at)},
        }
        if self._ttl_attr and self._ttl_buffer_seconds > 0:
            ttl = expires_at + self._ttl_buffer_seconds
            item[self._ttl_attr] = {"N": str(ttl)}

        try:
            self._client.put_item(
                TableName=self._table_name,
                Item=item,
                ConditionExpression="attribute_not_exists(#pk) OR #exp <= :now",
                ExpressionAttributeNames={"#pk": self._pk_attr, "#exp": self._expires_at_attr},
                ExpressionAttributeValues={":now": {"N": str(now)}},
            )
        except ClientError as err:  # pragma: no cover (depends on AWS error shape)
            mapped = map_client_error(err)
            if isinstance(mapped, ConditionFailedError):
                raise LeaseHeldError(str(mapped)) from err
            raise mapped from err

        return Lease(key=key, token=token, expires_at=expires_at)

    def refresh(self, lease: Lease, *, lease_seconds: int) -> Lease:
        if not lease.key.pk or not lease.key.sk:
            raise ValueError("lease.key.pk and lease.key.sk are required")
        if not lease.token:
            raise ValueError("lease.token is required")
        if lease_seconds <= 0:
            raise ValueError("lease_seconds must be > 0")

        now = int(self._now())
        expires_at = now + int(lease_seconds)

        names: dict[str, str] = {
            "#tok": self._token_attr,
            "#exp": self._expires_at_attr,
        }
        values: dict[str, Any] = {
            ":tok": {"S": lease.token},
            ":now": {"N": str(now)},
            ":exp": {"N": str(expires_at)},
        }

        update_expression = "SET #exp = :exp"
        if self._ttl_attr and self._ttl_buffer_seconds > 0:
            ttl = expires_at + self._ttl_buffer_seconds
            names["#ttl"] = self._ttl_attr
            values[":ttl"] = {"N": str(ttl)}
            update_expression = update_expression + ", #ttl = :ttl"

        try:
            self._client.update_item(
                TableName=self._table_name,
                Key={self._pk_attr: {"S": lease.key.pk}, self._sk_attr: {"S": lease.key.sk}},
                UpdateExpression=update_expression,
                ConditionExpression="#tok = :tok AND #exp > :now",
                ExpressionAttributeNames=names,
                ExpressionAttributeValues=values,
            )
        except ClientError as err:  # pragma: no cover (depends on AWS error shape)
            mapped = map_client_error(err)
            if isinstance(mapped, ConditionFailedError):
                raise LeaseNotOwnedError(str(mapped)) from err
            raise mapped from err

        return Lease(key=lease.key, token=lease.token, expires_at=expires_at)

    def release(self, lease: Lease) -> None:
        if not lease.key.pk or not lease.key.sk:
            raise ValueError("lease.key.pk and lease.key.sk are required")
        if not lease.token:
            raise ValueError("lease.token is required")

        try:
            self._client.delete_item(
                TableName=self._table_name,
                Key={self._pk_attr: {"S": lease.key.pk}, self._sk_attr: {"S": lease.key.sk}},
                ConditionExpression="#tok = :tok",
                ExpressionAttributeNames={"#tok": self._token_attr},
                ExpressionAttributeValues={":tok": {"S": lease.token}},
            )
        except ClientError as err:  # pragma: no cover (depends on AWS error shape)
            mapped = map_client_error(err)
            if isinstance(mapped, ConditionFailedError):
                return  # best-effort
            raise mapped from err

