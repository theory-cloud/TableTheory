from __future__ import annotations

import os
import time
from collections.abc import Callable, Mapping
from dataclasses import dataclass
from typing import Any, cast

import boto3
from botocore.config import Config


@dataclass(frozen=True)
class AwsCallMetric:
    service: str
    operation: str
    seconds: float
    ok: bool


def is_lambda_environment(environ: Mapping[str, str] = os.environ) -> bool:
    return bool(
        environ.get("AWS_LAMBDA_FUNCTION_NAME") or "AWS_Lambda" in (environ.get("AWS_EXECUTION_ENV") or "")
    )


def create_lambda_boto3_config(
    *,
    connect_timeout: float = 1.0,
    read_timeout: float = 3.0,
    max_attempts: int = 3,
) -> Config:
    return Config(
        connect_timeout=connect_timeout,
        read_timeout=read_timeout,
        retries={"max_attempts": max_attempts, "mode": "adaptive"},
    )


class _InstrumentedClient:
    def __init__(self, client: Any, service: str, on_call: Callable[[AwsCallMetric], None]) -> None:
        self._client = client
        self._service = service
        self._on_call = on_call

    def __getattr__(self, name: str) -> Any:
        attr = getattr(self._client, name)
        if name.startswith("_") or not callable(attr):
            return attr

        def wrapped(*args: Any, **kwargs: Any) -> Any:
            start = time.monotonic()
            try:
                out = attr(*args, **kwargs)
            except Exception:
                self._on_call(
                    AwsCallMetric(
                        service=self._service,
                        operation=name,
                        seconds=time.monotonic() - start,
                        ok=False,
                    )
                )
                raise

            self._on_call(
                AwsCallMetric(
                    service=self._service,
                    operation=name,
                    seconds=time.monotonic() - start,
                    ok=True,
                )
            )
            return out

        return wrapped


def instrument_boto3_client(
    client: Any,
    *,
    service: str,
    on_call: Callable[[AwsCallMetric], None],
) -> Any:
    return _InstrumentedClient(client, service, on_call)


_lambda_clients: dict[tuple[str, str | None], Any] = {}


def get_lambda_boto3_client(
    service: str,
    *,
    region: str | None = None,
    config: Config | None = None,
    session: Any | None = None,
    metrics: Callable[[AwsCallMetric], None] | None = None,
) -> Any:
    key = (service, region)
    existing = _lambda_clients.get(key)
    if existing is not None:
        return existing

    sess = session or boto3.session.Session(region_name=region)
    client = cast(Any, sess).client(service, region_name=region, config=config)
    if metrics is not None:
        client = instrument_boto3_client(client, service=service, on_call=metrics)

    _lambda_clients[key] = client
    return client


def get_lambda_dynamodb_client(
    *,
    region: str | None = None,
    config: Config | None = None,
    metrics: Callable[[AwsCallMetric], None] | None = None,
) -> Any:
    return get_lambda_boto3_client("dynamodb", region=region, config=config, metrics=metrics)


def get_lambda_kms_client(
    *,
    region: str | None = None,
    config: Config | None = None,
    metrics: Callable[[AwsCallMetric], None] | None = None,
) -> Any:
    return get_lambda_boto3_client("kms", region=region, config=config, metrics=metrics)


def _reset_lambda_clients_for_tests() -> None:
    _lambda_clients.clear()
