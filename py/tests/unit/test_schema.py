from __future__ import annotations

from dataclasses import replace

import pytest
from botocore.exceptions import ClientError

from theorydb_py import ModelDefinition, Projection, ValidationError, gsi, lsi, theorydb_field
from theorydb_py import schema as schema_module
from theorydb_py.errors import AwsError, NotFoundError
from theorydb_py.mocks import FakeDynamoDBClient
from theorydb_py.schema import (
    build_create_table_request,
    create_table,
    delete_table,
    describe_table,
    ensure_table,
    resolve_ttl_attribute,
    update_time_to_live,
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


@pytest.fixture()
def ttl_model() -> ModelDefinition[object]:
    from dataclasses import dataclass

    @dataclass(frozen=True)
    class Record:
        pk: str = theorydb_field(name="PK", roles=["pk"])
        sk: str = theorydb_field(name="SK", roles=["sk"])
        expires_at: int = theorydb_field(name="expires_at", roles=["ttl"])

    return ModelDefinition.from_dataclass(Record, table_name="ttl_tbl")


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


def test_create_table_syncs_ttl_for_models_with_ttl_role(ttl_model: ModelDefinition[object]) -> None:
    client = FakeDynamoDBClient()
    client.expect("create_table", {"TableName": "ttl_tbl", "BillingMode": "PAY_PER_REQUEST"}, response={})
    client.expect("describe_table", {"TableName": "ttl_tbl"}, response={"Table": {"TableStatus": "ACTIVE"}})
    client.expect(
        "update_time_to_live",
        {
            "TableName": "ttl_tbl",
            "TimeToLiveSpecification": {"AttributeName": "expires_at", "Enabled": True},
        },
        response={"TimeToLiveSpecification": {"AttributeName": "expires_at", "Enabled": True}},
    )

    create_table(ttl_model, client=client, wait_for_active=False, sleep=lambda _: None)
    client.assert_no_pending()


def test_ensure_table_syncs_ttl_for_existing_tables(ttl_model: ModelDefinition[object]) -> None:
    client = FakeDynamoDBClient()
    client.expect("describe_table", {"TableName": "ttl_tbl"}, response={"Table": {"TableStatus": "ACTIVE"}})
    client.expect("describe_table", {"TableName": "ttl_tbl"}, response={"Table": {"TableStatus": "ACTIVE"}})
    client.expect(
        "update_time_to_live",
        {
            "TableName": "ttl_tbl",
            "TimeToLiveSpecification": {"AttributeName": "expires_at", "Enabled": True},
        },
        response={"TimeToLiveSpecification": {"AttributeName": "expires_at", "Enabled": True}},
    )

    ensure_table(ttl_model, client=client, wait_for_active=False, sleep=lambda _: None)
    client.assert_no_pending()


def test_resolve_and_update_time_to_live_helpers(ttl_model: ModelDefinition[object]) -> None:
    assert resolve_ttl_attribute(ttl_model) == "expires_at"

    client = FakeDynamoDBClient()
    client.expect(
        "update_time_to_live",
        {
            "TableName": "ttl_tbl",
            "TimeToLiveSpecification": {"AttributeName": "expires_at", "Enabled": True},
        },
        response={"TimeToLiveSpecification": {"AttributeName": "expires_at", "Enabled": True}},
    )

    update_time_to_live(ttl_model, client=client)
    client.assert_no_pending()


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


def test_schema_helpers_use_default_boto3_client(
    monkeypatch: pytest.MonkeyPatch,
    model: ModelDefinition[object],
    ttl_model: ModelDefinition[object],
) -> None:
    create_client = FakeDynamoDBClient()
    create_client.expect("create_table", {"TableName": "tbl", "BillingMode": "PAY_PER_REQUEST"}, response={})

    ensure_client = FakeDynamoDBClient()
    ensure_client.expect(
        "describe_table", {"TableName": "ttl_tbl"}, response={"Table": {"TableStatus": "ACTIVE"}}
    )
    ensure_client.expect(
        "describe_table", {"TableName": "ttl_tbl"}, response={"Table": {"TableStatus": "ACTIVE"}}
    )
    ensure_client.expect(
        "update_time_to_live",
        {
            "TableName": "ttl_tbl",
            "TimeToLiveSpecification": {"AttributeName": "expires_at", "Enabled": True},
        },
        response={"TimeToLiveSpecification": {"AttributeName": "expires_at", "Enabled": True}},
    )

    delete_client = FakeDynamoDBClient()
    delete_client.expect("delete_table", {"TableName": "tbl"}, response={})

    describe_client = FakeDynamoDBClient()
    describe_client.expect(
        "describe_table", {"TableName": "tbl"}, response={"Table": {"TableStatus": "ACTIVE"}}
    )

    ttl_client = FakeDynamoDBClient()
    ttl_client.expect(
        "update_time_to_live",
        {
            "TableName": "ttl_tbl",
            "TimeToLiveSpecification": {"AttributeName": "expires_at", "Enabled": True},
        },
        response={"TimeToLiveSpecification": {"AttributeName": "expires_at", "Enabled": True}},
    )

    clients = iter([create_client, ensure_client, delete_client, describe_client, ttl_client])
    monkeypatch.setattr(schema_module.boto3, "client", lambda service: next(clients))

    create_table(model, wait_for_active=False)
    ensure_table(ttl_model, wait_for_active=False, sleep=lambda _: None)
    delete_table(model, wait_for_delete=False)
    assert describe_table(model)["Table"]["TableStatus"] == "ACTIVE"
    update_time_to_live(ttl_model)

    create_client.assert_no_pending()
    ensure_client.assert_no_pending()
    delete_client.assert_no_pending()
    describe_client.assert_no_pending()
    ttl_client.assert_no_pending()


def test_update_time_to_live_noops_without_ttl_attribute(model: ModelDefinition[object]) -> None:
    client = FakeDynamoDBClient()

    update_time_to_live(model, client=client)

    assert client.calls == []


def test_update_time_to_live_maps_validation_exception(ttl_model: ModelDefinition[object]) -> None:
    client = FakeDynamoDBClient()
    client.expect(
        "update_time_to_live",
        {
            "TableName": "ttl_tbl",
            "TimeToLiveSpecification": {"AttributeName": "expires_at", "Enabled": True},
        },
        error=ClientError(
            {"Error": {"Code": "ValidationException", "Message": "bad ttl"}}, "UpdateTimeToLive"
        ),
    )

    with pytest.raises(ValidationError, match="bad ttl"):
        update_time_to_live(ttl_model, client=client)


def test_schema_operations_require_table_name_when_model_has_none(model: ModelDefinition[object]) -> None:
    nameless = replace(model, table_name="")
    client = FakeDynamoDBClient()

    with pytest.raises(ValueError, match="table_name is required"):
        build_create_table_request(nameless)
    with pytest.raises(ValueError, match="table_name is required"):
        ensure_table(nameless, client=client)
    with pytest.raises(ValueError, match="table_name is required"):
        delete_table(nameless, client=client)
    with pytest.raises(ValueError, match="table_name is required"):
        describe_table(nameless, client=client)
    with pytest.raises(ValueError, match="table_name is required"):
        update_time_to_live(nameless, client=client)
