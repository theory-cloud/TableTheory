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


DEFAULT_LAMBDA_TIMEOUT_BUFFER_SECONDS = 1.0


@dataclass(frozen=True)
class LambdaTimeoutConfig:
    buffer_seconds: float = DEFAULT_LAMBDA_TIMEOUT_BUFFER_SECONDS


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


def _remaining_time_millis(context: Any) -> int | None:
    if context is None:
        return None

    getter = getattr(context, "get_remaining_time_in_millis", None)
    if getter is None:
        getter = getattr(context, "getRemainingTimeInMillis", None)
    if getter is None or not callable(getter):
        return None

    remaining = getter()
    if remaining is None:
        return None
    return int(remaining)


def check_lambda_timeout(
    context: Any, *, buffer_seconds: float = DEFAULT_LAMBDA_TIMEOUT_BUFFER_SECONDS
) -> None:
    remaining_ms = _remaining_time_millis(context)
    if remaining_ms is None:
        return

    buffer_ms = max(0, int(buffer_seconds * 1000))
    if remaining_ms <= buffer_ms:
        raise TimeoutError(f"lambda timeout imminent: only {remaining_ms}ms remaining")


class _LambdaTimeoutClient:
    def __init__(self, client: Any, context: Any, config: LambdaTimeoutConfig) -> None:
        self._client = client
        self._context = context
        self._config = config

    def __getattr__(self, name: str) -> Any:
        attr = getattr(self._client, name)
        if name.startswith("_") or not callable(attr):
            return attr

        def wrapped(*args: Any, **kwargs: Any) -> Any:
            check_lambda_timeout(self._context, buffer_seconds=self._config.buffer_seconds)
            return attr(*args, **kwargs)

        return wrapped


def with_lambda_timeout(
    client: Any,
    context: Any,
    *,
    buffer_seconds: float = DEFAULT_LAMBDA_TIMEOUT_BUFFER_SECONDS,
) -> Any:
    if context is None or _remaining_time_millis(context) is None:
        return client
    return _LambdaTimeoutClient(
        client,
        context,
        LambdaTimeoutConfig(buffer_seconds=max(0.0, buffer_seconds)),
    )


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
