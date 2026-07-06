from __future__ import annotations

import base64
import json
import os
from collections.abc import Callable, Mapping
from typing import Any, cast

from boto3.dynamodb.types import Binary
from botocore.exceptions import ClientError
from cryptography.hazmat.primitives.ciphers.aead import AESGCM

from .errors import AwsError, ValidationError


def _aad(attr_name: str) -> bytes:
    return f"theorydb:encrypted:v1|attr={attr_name}".encode()


def _ensure_single_key_map(value: Any, *, context: str) -> tuple[str, Any]:
    if not isinstance(value, dict) or len(value) != 1:
        raise ValidationError(f"{context}: attribute value must be a single-key map")
    (key, inner), *_ = value.items()
    return str(key), inner


def marshal_attribute_value_json(av: Any) -> dict[str, Any]:
    kind, value = _ensure_single_key_map(av, context="marshal")

    if kind == "S":
        if not isinstance(value, str):
            raise ValidationError("marshal: S must be a string")
        return {"t": "S", "s": value}
    if kind == "N":
        if not isinstance(value, str):
            raise ValidationError("marshal: N must be a string")
        return {"t": "N", "n": value}
    if kind == "B":
        if not isinstance(value, (bytes, bytearray)):
            raise ValidationError("marshal: B must be bytes")
        return {"t": "B", "b": base64.b64encode(bytes(value)).decode("ascii")}
    if kind == "BOOL":
        if not isinstance(value, bool):
            raise ValidationError("marshal: BOOL must be bool")
        return {"t": "BOOL", "bool": value}
    if kind == "NULL":
        if value is not True:
            raise ValidationError("marshal: NULL must be true")
        return {"t": "NULL", "null": True}

    if kind == "SS":
        if not isinstance(value, list) or not all(isinstance(v, str) for v in value):
            raise ValidationError("marshal: SS must be list[str]")
        return {"t": "SS", "ss": value}
    if kind == "NS":
        if not isinstance(value, list) or not all(isinstance(v, str) for v in value):
            raise ValidationError("marshal: NS must be list[str]")
        return {"t": "NS", "ns": value}
    if kind == "BS":
        if not isinstance(value, list) or not all(isinstance(v, (bytes, bytearray)) for v in value):
            raise ValidationError("marshal: BS must be list[bytes]")
        return {
            "t": "BS",
            "bs": [base64.b64encode(bytes(v)).decode("ascii") for v in value],
        }

    if kind == "L":
        if not isinstance(value, list):
            raise ValidationError("marshal: L must be list")
        return {"t": "L", "l": [marshal_attribute_value_json(v) for v in value]}
    if kind == "M":
        if not isinstance(value, dict):
            raise ValidationError("marshal: M must be map")
        return {
            "t": "M",
            "m": {k: marshal_attribute_value_json(value[k]) for k in sorted(value.keys())},
        }

    raise ValidationError(f"marshal: unsupported attribute value type: {kind}")


def unmarshal_attribute_value_json(enc: Any) -> dict[str, Any]:
    if not isinstance(enc, dict) or "t" not in enc:
        raise ValidationError("unmarshal: encoded attribute value must be a map with t")

    kind = enc.get("t")
    if kind == "S":
        value = enc.get("s")
        if not isinstance(value, str):
            raise ValidationError("unmarshal: S must be a string")
        return {"S": value}
    if kind == "N":
        value = enc.get("n")
        if not isinstance(value, str):
            raise ValidationError("unmarshal: N must be a string")
        return {"N": value}
    if kind == "B":
        value = enc.get("b")
        if not isinstance(value, str):
            raise ValidationError("unmarshal: B must be base64 string")
        try:
            return {"B": base64.b64decode(value)}
        except Exception as err:
            raise ValidationError("unmarshal: invalid base64 in B") from err
    if kind == "BOOL":
        value = enc.get("bool")
        if not isinstance(value, bool):
            raise ValidationError("unmarshal: BOOL must be bool")
        return {"BOOL": value}
    if kind == "NULL":
        value = enc.get("null")
        if value is not True:
            raise ValidationError("unmarshal: NULL must be true")
        return {"NULL": True}

    if kind == "SS":
        value = enc.get("ss")
        if not isinstance(value, list) or not all(isinstance(v, str) for v in value):
            raise ValidationError("unmarshal: SS must be list[str]")
        return {"SS": value}
    if kind == "NS":
        value = enc.get("ns")
        if not isinstance(value, list) or not all(isinstance(v, str) for v in value):
            raise ValidationError("unmarshal: NS must be list[str]")
        return {"NS": value}
    if kind == "BS":
        value = enc.get("bs")
        if not isinstance(value, list) or not all(isinstance(v, str) for v in value):
            raise ValidationError("unmarshal: BS must be list[str]")
        try:
            return {"BS": [base64.b64decode(v) for v in value]}
        except Exception as err:
            raise ValidationError("unmarshal: invalid base64 in BS") from err

    if kind == "L":
        value = enc.get("l")
        if not isinstance(value, list):
            raise ValidationError("unmarshal: L must be list")
        return {"L": [unmarshal_attribute_value_json(v) for v in value]}
    if kind == "M":
        value = enc.get("m")
        if not isinstance(value, dict):
            raise ValidationError("unmarshal: M must be map")
        return {"M": {k: unmarshal_attribute_value_json(value[k]) for k in sorted(value.keys())}}

    raise ValidationError(f"unmarshal: unsupported encoded attribute value type: {kind}")


def encrypt_attribute_value(
    av: Any,
    *,
    attr_name: str,
    kms_key_arn: str,
    kms_client: Any,
    rand_bytes: Callable[[int], bytes] = os.urandom,
) -> dict[str, Any]:
    try:
        data_key = kms_client.generate_data_key(KeyId=kms_key_arn, KeySpec="AES_256")
        dek = data_key["Plaintext"]
        edk = data_key["CiphertextBlob"]
    except ClientError as err:
        code = str(err.response.get("Error", {}).get("Code", ""))
        message = str(err.response.get("Error", {}).get("Message", ""))
        raise AwsError(code=code or "KMSGenerateDataKeyError", message=message or str(err)) from err

    if not isinstance(dek, (bytes, bytearray)) or not isinstance(edk, (bytes, bytearray)):
        raise ValidationError("kms GenerateDataKey returned invalid key types")

    plaintext = json.dumps(
        marshal_attribute_value_json(av),
        separators=(",", ":"),
        ensure_ascii=False,
    ).encode("utf-8")
    nonce = rand_bytes(12)
    ct = AESGCM(bytes(dek)).encrypt(nonce, plaintext, _aad(attr_name))

    return {"v": 1, "edk": bytes(edk), "nonce": nonce, "ct": ct}


def decrypt_attribute_value(
    envelope: Mapping[str, Any],
    *,
    attr_name: str,
    kms_key_arn: str,
    kms_client: Any,
) -> dict[str, Any]:
    version = envelope.get("v")
    if version is None:
        raise ValidationError("encrypted envelope is missing v")
    try:
        version_int = int(version)
    except Exception as err:
        raise ValidationError("encrypted envelope is missing v") from err
    if version_int != 1:
        raise ValidationError(f"unsupported encrypted envelope version: {version_int}")

    def _as_bytes(value: Any, *, field: str) -> bytes:
        if isinstance(value, (bytes, bytearray)):
            return bytes(value)
        if isinstance(value, Binary):
            return bytes(cast(Any, value))
        raise ValidationError(f"encrypted envelope field is not bytes: {field}")

    edk = _as_bytes(envelope.get("edk"), field="edk")
    nonce = _as_bytes(envelope.get("nonce"), field="nonce")
    ct = _as_bytes(envelope.get("ct"), field="ct")

    try:
        resp = kms_client.decrypt(CiphertextBlob=edk, KeyId=kms_key_arn)
        dek = resp["Plaintext"]
    except ClientError as err:
        code = str(err.response.get("Error", {}).get("Code", ""))
        message = str(err.response.get("Error", {}).get("Message", ""))
        raise AwsError(code=code or "KMSDecryptError", message=message or str(err)) from err

    if not isinstance(dek, (bytes, bytearray)):
        raise ValidationError("kms Decrypt returned invalid key types")

    try:
        plaintext = AESGCM(bytes(dek)).decrypt(nonce, ct, _aad(attr_name))
    except Exception as err:
        raise ValidationError("failed to decrypt encrypted envelope") from err

    try:
        enc = json.loads(plaintext.decode("utf-8"))
    except Exception as err:
        raise ValidationError("failed to parse decrypted attribute payload") from err

    return unmarshal_attribute_value_json(enc)
