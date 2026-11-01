"""Python reader for the local cross-language no-drift check.

Reads the item written by the Go writer against the same local DynamoDB table
using the in-repo Python runtime, decrypts the encrypted secret through the
local KMS, and fails if the shape or the plaintext drifted.
"""

from __future__ import annotations

import os
from dataclasses import dataclass

import boto3

from tabletheory_py import ModelDefinition, Table, theorydb_field


@dataclass(frozen=True)
class DemoItem:
    pk: str = theorydb_field(name="PK", roles=["pk"])
    sk: str = theorydb_field(name="SK", roles=["sk"])
    value: str = theorydb_field(default="")
    lang: str = theorydb_field(default="")
    secret: str = theorydb_field(name="secret", encrypted=True, default="")


def main() -> None:
    kms_key_arn = os.environ.get("KMS_KEY_ARN", "")
    if not kms_key_arn:
        raise SystemExit("KMS_KEY_ARN is required for the encrypted local variant")

    region = os.environ.get("AWS_REGION", "us-east-1")
    access_key = os.environ.get("AWS_ACCESS_KEY_ID", "dummy")
    secret_key = os.environ.get("AWS_SECRET_ACCESS_KEY", "dummy")

    client = boto3.client(
        "dynamodb",
        endpoint_url=os.environ.get("DYNAMODB_ENDPOINT", "http://localhost:8020"),
        region_name=region,
        aws_access_key_id=access_key,
        aws_secret_access_key=secret_key,
    )
    kms = boto3.client(
        "kms",
        endpoint_url=(
            os.environ.get("KMS_ENDPOINT")
            or os.environ.get("AWS_ENDPOINT_URL_KMS")
            or "http://localhost:8021"
        ),
        region_name=region,
        aws_access_key_id=access_key,
        aws_secret_access_key=secret_key,
    )
    table_name = os.environ.get("DEMO_TABLE_NAME", "demo_multilang_local")
    model = ModelDefinition.from_dataclass(DemoItem, table_name=table_name)
    table = Table(model, client=client, kms_key_arn=kms_key_arn, kms_client=kms)

    item = table.get("demo#1", "v1")
    if item.value != "shared-value" or item.lang != "go":
        raise SystemExit(f"python: drift detected: {item}")
    if item.secret != "shared-secret-v1":
        raise SystemExit(f"python: secret did not decrypt: {item.secret!r}")
    print(
        f"python: read demo#1/v1 value={item.value!r} lang={item.lang!r} secret={item.secret!r}"
    )


if __name__ == "__main__":
    main()
