from __future__ import annotations

from datetime import UTC, datetime

import pytest

from theorydb_py.mocks import FakeDynamoDBClient
from theorydb_py.runtime import (
    _reset_lambda_clients_for_tests,
    create_lambda_boto3_config,
    get_lambda_boto3_client,
    instrument_boto3_client,
    is_lambda_environment,
)


def test_is_lambda_environment() -> None:
    assert is_lambda_environment({}) is False
    assert is_lambda_environment({"AWS_LAMBDA_FUNCTION_NAME": "fn"}) is True
    assert is_lambda_environment({"AWS_EXECUTION_ENV": "AWS_Lambda_python3.14"}) is True


def test_create_lambda_boto3_config() -> None:
    cfg = create_lambda_boto3_config(connect_timeout=2.0, read_timeout=4.0, max_attempts=3)
    assert cfg.connect_timeout == 2.0
    assert cfg.read_timeout == 4.0
    assert cfg.retries["max_attempts"] == 3


def test_instrument_boto3_client_records_calls() -> None:
    metrics: list[object] = []

    client = FakeDynamoDBClient()
    client.expect("put_item", response={})
    wrapped = instrument_boto3_client(client, service="dynamodb", on_call=metrics.append)
    wrapped.put_item(TableName="t", Item={})
    assert len(metrics) == 1

    client2 = FakeDynamoDBClient()
    client2.expect("get_item", error=RuntimeError("boom"), response=None)
    wrapped2 = instrument_boto3_client(client2, service="dynamodb", on_call=metrics.append)
    with pytest.raises(RuntimeError, match="boom"):
        wrapped2.get_item(TableName="t", Key={})
    assert len(metrics) == 2


def test_get_lambda_boto3_client_caches() -> None:
    _reset_lambda_clients_for_tests()

    class FakeSession:
        def __init__(self) -> None:
            self.calls = 0

        def client(self, service_name: str, **kwargs: object) -> object:
            _ = service_name, kwargs
            self.calls += 1
            return {"created_at": datetime.now(tz=UTC).isoformat()}

    sess = FakeSession()
    c1 = get_lambda_boto3_client("dynamodb", region="us-east-1", session=sess)
    c2 = get_lambda_boto3_client("dynamodb", region="us-east-1", session=sess)
    assert c1 is c2
    assert sess.calls == 1
