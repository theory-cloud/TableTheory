from __future__ import annotations

from dataclasses import replace

import pytest
from botocore.exceptions import ClientError

from theorydb_py import ModelDefinition, Projection, ValidationError, theorydb_field, gsi, lsi
from theorydb_py.errors import AwsError, NotFoundError
from theorydb_py.mocks import FakeDynamoDBClient
from theorydb_py.schema import (
    build_create_table_request,
    create_table,
    delete_table,
    describe_table,
    ensure_table,
)


@pytest.fixture()
def model() -> ModelDefinition[object]:
    from dataclasses import dataclass

    @dataclass(frozen=True)
    class Record:
        pk: str = theorydb_field(name="PK", roles=["pk"])
        sk: str = theorydb_field(name="SK", roles=["sk"])
        email_hash: str = theorydb_field(name="emailHash")
        updated: int = theorydb_field(name="updated")

    return ModelDefinition.from_dataclass(
        Record,
        table_name="tbl",
        indexes=[
            gsi("gsi-email", partition="email_hash", projection=Projection.keys_only()),
            lsi("lsi-updated", sort="updated", projection=Projection.include("emailHash")),
        ],
    )


def test_build_create_table_request_includes_indexes_and_sorted_attributes(
    model: ModelDefinition[object],
) -> None:
    req = build_create_table_request(model)

    assert req["TableName"] == "tbl"
    assert req["BillingMode"] == "PAY_PER_REQUEST"
    assert req["KeySchema"] == [
        {"AttributeName": "PK", "KeyType": "HASH"},
        {"AttributeName": "SK", "KeyType": "RANGE"},
    ]
    assert req["AttributeDefinitions"] == [
        {"AttributeName": "PK", "AttributeType": "S"},
        {"AttributeName": "SK", "AttributeType": "S"},
        {"AttributeName": "emailHash", "AttributeType": "S"},
        {"AttributeName": "updated", "AttributeType": "N"},
    ]

    assert req["GlobalSecondaryIndexes"] == [
        {
            "IndexName": "gsi-email",
            "KeySchema": [{"AttributeName": "emailHash", "KeyType": "HASH"}],
            "Projection": {"ProjectionType": "KEYS_ONLY"},
        }
    ]
    assert req["LocalSecondaryIndexes"] == [
        {
            "IndexName": "lsi-updated",
            "KeySchema": [
                {"AttributeName": "PK", "KeyType": "HASH"},
                {"AttributeName": "updated", "KeyType": "RANGE"},
            ],
            "Projection": {"ProjectionType": "INCLUDE", "NonKeyAttributes": ["emailHash"]},
        }
    ]


def test_build_create_table_request_provisioned_requires_throughput(model: ModelDefinition[object]) -> None:
    with pytest.raises(ValidationError, match="provisioned_throughput is required"):
        build_create_table_request(model, billing_mode="PROVISIONED")

    req = build_create_table_request(
        model,
        billing_mode="PROVISIONED",
        provisioned_throughput={"ReadCapacityUnits": 1, "WriteCapacityUnits": 2},
    )
    assert req["ProvisionedThroughput"] == {"ReadCapacityUnits": 1, "WriteCapacityUnits": 2}
    assert req["GlobalSecondaryIndexes"][0]["ProvisionedThroughput"] == {
        "ReadCapacityUnits": 1,
        "WriteCapacityUnits": 2,
    }


def test_build_create_table_request_rejects_invalid_lsi_partition(model: ModelDefinition[object]) -> None:
    bad = replace(
        model,
        indexes=(
            replace(model.indexes[1], partition="NotTablePK"),
            model.indexes[0],
        ),
    )
    with pytest.raises(ValidationError, match="LSI partition key must match table partition key"):
        build_create_table_request(bad)


def test_create_table_is_idempotent_and_waits_active(model: ModelDefinition[object]) -> None:
    client = FakeDynamoDBClient()
    client.expect(
        "create_table",
        {"TableName": "tbl", "BillingMode": "PAY_PER_REQUEST"},
        error=ClientError({"Error": {"Code": "ResourceInUseException", "Message": "exists"}}, "CreateTable"),
    )
    client.expect(
        "describe_table",
        {"TableName": "tbl"},
        error=ClientError(
            {"Error": {"Code": "ResourceNotFoundException", "Message": "missing"}}, "DescribeTable"
        ),
    )
    client.expect("describe_table", {"TableName": "tbl"}, response={"Table": {"TableStatus": "ACTIVE"}})

    create_table(model, client=client, sleep=lambda _: None)
    client.assert_no_pending()


def test_ensure_table_creates_when_missing(model: ModelDefinition[object]) -> None:
    client = FakeDynamoDBClient()
    client.expect(
        "describe_table",
        {"TableName": "tbl"},
        error=ClientError(
            {"Error": {"Code": "ResourceNotFoundException", "Message": "missing"}}, "DescribeTable"
        ),
    )
    client.expect("create_table", {"TableName": "tbl", "BillingMode": "PAY_PER_REQUEST"}, response={})
    client.expect("describe_table", {"TableName": "tbl"}, response={"Table": {"TableStatus": "ACTIVE"}})

    ensure_table(model, client=client, sleep=lambda _: None)
    client.assert_no_pending()


def test_delete_table_ignore_missing(model: ModelDefinition[object]) -> None:
    client = FakeDynamoDBClient()
    client.expect(
        "delete_table",
        {"TableName": "tbl"},
        error=ClientError(
            {"Error": {"Code": "ResourceNotFoundException", "Message": "missing"}}, "DeleteTable"
        ),
    )

    delete_table(model, client=client, ignore_missing=True)
    client.assert_no_pending()


def test_delete_table_waits_for_deleted(model: ModelDefinition[object]) -> None:
    client = FakeDynamoDBClient()
    client.expect("delete_table", {"TableName": "tbl"}, response={})
    client.expect(
        "describe_table",
        {"TableName": "tbl"},
        error=ClientError(
            {"Error": {"Code": "ResourceNotFoundException", "Message": "missing"}}, "DescribeTable"
        ),
    )

    delete_table(model, client=client, sleep=lambda _: None)
    client.assert_no_pending()


def test_describe_table_maps_errors(model: ModelDefinition[object]) -> None:
    client = FakeDynamoDBClient()
    client.expect(
        "describe_table",
        {"TableName": "tbl"},
        error=ClientError(
            {"Error": {"Code": "ResourceNotFoundException", "Message": "missing"}}, "DescribeTable"
        ),
    )
    with pytest.raises(NotFoundError):
        describe_table(model, client=client)

    client = FakeDynamoDBClient()
    client.expect(
        "describe_table",
        {"TableName": "tbl"},
        error=ClientError({"Error": {"Code": "Nope", "Message": "x"}}, "DescribeTable"),
    )
    with pytest.raises(AwsError):
        describe_table(model, client=client)


def test_describe_table_returns_response(model: ModelDefinition[object]) -> None:
    client = FakeDynamoDBClient()
    client.expect("describe_table", {"TableName": "tbl"}, response={"Table": {"TableStatus": "ACTIVE"}})
    resp = describe_table(model, client=client)
    assert resp["Table"]["TableStatus"] == "ACTIVE"
    client.assert_no_pending()


def test_create_table_maps_validation_exception(model: ModelDefinition[object]) -> None:
    client = FakeDynamoDBClient()
    client.expect(
        "create_table",
        {"TableName": "tbl", "BillingMode": "PAY_PER_REQUEST"},
        error=ClientError({"Error": {"Code": "ValidationException", "Message": "bad"}}, "CreateTable"),
    )

    with pytest.raises(ValidationError):
        create_table(model, client=client, wait_for_active=False)
    client.assert_no_pending()


def test_ensure_table_waits_for_active_when_table_exists(model: ModelDefinition[object]) -> None:
    client = FakeDynamoDBClient()
    client.expect("describe_table", {"TableName": "tbl"}, response={"Table": {"TableStatus": "CREATING"}})
    client.expect("describe_table", {"TableName": "tbl"}, response={"Table": {"TableStatus": "ACTIVE"}})

    ensure_table(model, client=client, sleep=lambda _: None)
    client.assert_no_pending()


def test_build_create_table_request_validates_billing_mode(model: ModelDefinition[object]) -> None:
    with pytest.raises(ValidationError, match="unsupported billing_mode"):
        build_create_table_request(model, billing_mode="ON_DEMAND")


def test_build_create_table_request_requires_dataclass_model_type(model: ModelDefinition[object]) -> None:
    bad = replace(model, model_type=int)  # type: ignore[arg-type]
    with pytest.raises(ValidationError, match="model_type must be a dataclass"):
        build_create_table_request(bad)


def test_build_create_table_request_key_type_inference_variants() -> None:
    from dataclasses import dataclass

    @dataclass(frozen=True)
    class BinKey:
        pk: bytes = theorydb_field(roles=["pk"])

    bin_model = ModelDefinition.from_dataclass(BinKey, table_name="tbl_bin")
    req = build_create_table_request(bin_model)
    assert req["AttributeDefinitions"] == [{"AttributeName": "pk", "AttributeType": "B"}]

    @dataclass(frozen=True)
    class OptionalKey:
        pk: str | None = theorydb_field(roles=["pk"])

    opt_model = ModelDefinition.from_dataclass(OptionalKey, table_name="tbl_opt")
    req = build_create_table_request(opt_model)
    assert req["AttributeDefinitions"] == [{"AttributeName": "pk", "AttributeType": "S"}]

    @dataclass(frozen=True)
    class JsonKey:
        pk: dict[str, int] = theorydb_field(roles=["pk"], json=True)

    json_model = ModelDefinition.from_dataclass(JsonKey, table_name="tbl_json")
    req = build_create_table_request(json_model)
    assert req["AttributeDefinitions"] == [{"AttributeName": "pk", "AttributeType": "S"}]

    @dataclass(frozen=True)
    class UnsupportedKey:
        pk: list[str] = theorydb_field(roles=["pk"])

    bad_model = ModelDefinition.from_dataclass(UnsupportedKey, table_name="tbl_bad")
    with pytest.raises(ValidationError, match="key attribute must be S/N/B"):
        build_create_table_request(bad_model)
