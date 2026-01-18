from __future__ import annotations

import base64
import json
import os
from dataclasses import dataclass
from typing import Any

import boto3

from theorydb_py import (
    ModelDefinition,
    NotFoundError,
    Table,
    TransactPut,
    assert_model_definition_equivalent_to_dms,
    theorydb_field,
    get_dms_model,
    parse_dms_document,
)


@dataclass(frozen=True)
class DemoItem:
    pk: str = theorydb_field(name="PK", roles=["pk"])
    sk: str = theorydb_field(name="SK", roles=["sk"])
    value: str = theorydb_field(name="value", omitempty=True, default="")
    lang: str = theorydb_field(name="lang", omitempty=True, default="")
    secret: str = theorydb_field(name="secret", encrypted=True, omitempty=True, default="")


_table: Table[DemoItem] | None = None


def _get_table() -> Table[DemoItem]:
    global _table
    if _table is not None:
        return _table

    table_name = (os.environ.get("TABLE_NAME") or "").strip()
    if not table_name:
        raise RuntimeError("TABLE_NAME is required")

    kms_key_arn = (os.environ.get("KMS_KEY_ARN") or "").strip()
    if not kms_key_arn:
        raise RuntimeError("KMS_KEY_ARN is required")

    dms_b64 = (os.environ.get("DMS_MODEL_B64") or "").strip()
    if not dms_b64:
        raise RuntimeError("DMS_MODEL_B64 is required")

    dms_yaml = base64.b64decode(dms_b64.encode("utf-8")).decode("utf-8")
    dms_doc = parse_dms_document(dms_yaml)
    dms_model = get_dms_model(dms_doc, "DemoItem")

    model = ModelDefinition.from_dataclass(DemoItem, table_name=table_name)
    assert_model_definition_equivalent_to_dms(model, dms_model, ignore_table_name=True)
    client = boto3.client("dynamodb")
    _table = Table(model, client=client, kms_key_arn=kms_key_arn)
    return _table


def _json_response(status_code: int, body: Any) -> dict[str, Any]:
    return {
        "statusCode": status_code,
        "headers": {"content-type": "application/json"},
        "body": json.dumps(body, separators=(",", ":"), sort_keys=True),
    }


def handler(event: dict[str, Any], context: Any) -> dict[str, Any]:
    _ = context
    method = ((event.get("requestContext") or {}).get("http") or {}).get("method") or "GET"
    path = event.get("rawPath") or "/"
    qs = event.get("queryStringParameters") or {}

    body_raw = event.get("body") or ""
    try:
        body = json.loads(body_raw) if body_raw else {}
    except Exception:
        body = {}

    pk = str(body.get("pk") or qs.get("pk") or "")
    sk = str(body.get("sk") or qs.get("sk") or "")
    value = str(body.get("value") or qs.get("value") or "")
    secret = str(body.get("secret") or qs.get("secret") or "")
    sk_prefix = str(body.get("skPrefix") or qs.get("skPrefix") or "")
    count = int(body.get("count") or qs.get("count") or 0)

    table = _get_table()

    if path == "/batch":
        if method == "GET":
            return _json_response(405, {"error": "use POST/PUT"})
        if not pk:
            return _json_response(400, {"error": "pk is required"})

        if count <= 0:
            count = 3
        if count > 25:
            return _json_response(400, {"error": "count must be <= 25"})

        if not sk_prefix:
            sk_prefix = "BATCH#"

        puts = [
            DemoItem(pk=pk, sk=f"{sk_prefix}{i + 1}", value=value, lang="py", secret=secret)
            for i in range(count)
        ]
        table.batch_write(puts=puts)
        got = table.batch_get([(p.pk, p.sk) for p in puts])
        items = [
            {"PK": item.pk, "SK": item.sk, "value": item.value, "lang": item.lang, "secret": item.secret}
            for item in got
        ]
        return _json_response(200, {"ok": True, "count": len(items), "items": items})

    if path == "/tx":
        if method == "GET":
            return _json_response(405, {"error": "use POST/PUT"})
        if not pk:
            return _json_response(400, {"error": "pk is required"})

        if not sk_prefix:
            sk_prefix = "TX#"

        item1 = DemoItem(pk=pk, sk=f"{sk_prefix}1", value=value, lang="py", secret=secret)
        item2 = DemoItem(pk=pk, sk=f"{sk_prefix}2", value=value, lang="py", secret=secret)
        table.transact_write([TransactPut(item=item1), TransactPut(item=item2)])
        got = table.batch_get([(item1.pk, item1.sk), (item2.pk, item2.sk)])
        items = [
            {"PK": item.pk, "SK": item.sk, "value": item.value, "lang": item.lang, "secret": item.secret}
            for item in got
        ]
        return _json_response(200, {"ok": True, "count": len(items), "items": items})

    if not pk or not sk:
        return _json_response(400, {"error": "pk and sk are required"})

    if method == "GET":
        try:
            item = table.get(pk, sk)
        except NotFoundError:
            return _json_response(404, {"error": "not found"})
        return _json_response(
            200,
            {
                "ok": True,
                "item": {
                    "PK": item.pk,
                    "SK": item.sk,
                    "value": item.value,
                    "lang": item.lang,
                    "secret": item.secret,
                },
            },
        )

    table.put(DemoItem(pk=pk, sk=sk, value=value, lang="py", secret=secret))
    item = table.get(pk, sk)
    return _json_response(
        200,
        {
            "ok": True,
            "item": {
                "PK": item.pk,
                "SK": item.sk,
                "value": item.value,
                "lang": item.lang,
                "secret": item.secret,
            },
        },
    )
