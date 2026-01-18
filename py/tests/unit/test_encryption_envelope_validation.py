from __future__ import annotations

import json

import pytest
from boto3.dynamodb.types import Binary
from botocore.exceptions import ClientError
from cryptography.hazmat.primitives.ciphers.aead import AESGCM

from theorydb_py.encryption import decrypt_attribute_value, encrypt_attribute_value
from theorydb_py.errors import AwsError, ValidationError


class _GoodKms:
    def __init__(self) -> None:
        self._dek = b"\x02" * 32
        self._edk = b"edk"

    def generate_data_key(self, *, KeyId: str, KeySpec: str):  # noqa: N803
        assert KeyId
        assert KeySpec == "AES_256"
        return {"Plaintext": self._dek, "CiphertextBlob": self._edk}

    def decrypt(self, *, CiphertextBlob: bytes, KeyId: str):  # noqa: N803
        assert KeyId
        assert CiphertextBlob == self._edk
        return {"Plaintext": self._dek}


def test_encrypt_and_decrypt_round_trip_with_aad_binding() -> None:
    kms = _GoodKms()
    envelope = encrypt_attribute_value(
        {"S": "top-secret"},
        attr_name="secret",
        kms_key_arn="arn:aws:kms:us-east-1:000000000000:key/test",
        kms_client=kms,
    )
    assert set(envelope.keys()) == {"v", "edk", "nonce", "ct"}

    plaintext = AESGCM(kms._dek).decrypt(
        envelope["nonce"],
        envelope["ct"],
        b"theorydb:encrypted:v1|attr=secret",
    )
    decoded = json.loads(plaintext.decode("utf-8"))
    assert decoded.get("t") == "S"
    assert decoded.get("s") == "top-secret"

    decrypted = decrypt_attribute_value(
        envelope,
        attr_name="secret",
        kms_key_arn="arn:aws:kms:us-east-1:000000000000:key/test",
        kms_client=kms,
    )
    assert decrypted == {"S": "top-secret"}

    with pytest.raises(ValidationError, match="failed to decrypt"):
        decrypt_attribute_value(
            envelope,
            attr_name="other",
            kms_key_arn="arn:aws:kms:us-east-1:000000000000:key/test",
            kms_client=kms,
        )


def test_decrypt_validates_envelope_version_and_fields() -> None:
    kms = _GoodKms()

    with pytest.raises(ValidationError, match="missing v"):
        decrypt_attribute_value(
            {"edk": b"x", "nonce": b"y", "ct": b"z"},
            attr_name="secret",
            kms_key_arn="arn",
            kms_client=kms,
        )

    with pytest.raises(ValidationError, match="missing v"):
        decrypt_attribute_value(
            {"v": "nope", "edk": b"x", "nonce": b"y", "ct": b"z"},
            attr_name="secret",
            kms_key_arn="arn",
            kms_client=kms,
        )

    with pytest.raises(ValidationError, match="unsupported"):
        decrypt_attribute_value(
            {"v": 2, "edk": b"x", "nonce": b"y", "ct": b"z"},
            attr_name="secret",
            kms_key_arn="arn",
            kms_client=kms,
        )

    with pytest.raises(ValidationError, match="not bytes"):
        decrypt_attribute_value(
            {"v": 1, "edk": "x", "nonce": b"y", "ct": b"z"},
            attr_name="secret",
            kms_key_arn="arn",
            kms_client=kms,
        )


def test_decrypt_accepts_binary_wrapper_fields() -> None:
    kms = _GoodKms()
    envelope = encrypt_attribute_value(
        {"S": "x"},
        attr_name="secret",
        kms_key_arn="arn",
        kms_client=kms,
    )
    wrapped = {
        "v": envelope["v"],
        "edk": Binary(envelope["edk"]),
        "nonce": Binary(envelope["nonce"]),
        "ct": Binary(envelope["ct"]),
    }
    assert decrypt_attribute_value(wrapped, attr_name="secret", kms_key_arn="arn", kms_client=kms) == {
        "S": "x"
    }


def test_kms_errors_are_mapped_and_invalid_key_types_are_rejected() -> None:
    class _FailKms:
        def generate_data_key(self, *, KeyId: str, KeySpec: str):  # noqa: N803
            raise ClientError(
                {"Error": {"Code": "AccessDeniedException", "Message": "nope"}}, "GenerateDataKey"
            )

        def decrypt(self, *, CiphertextBlob: bytes, KeyId: str):  # noqa: N803
            raise ClientError({"Error": {"Code": "AccessDeniedException", "Message": "nope"}}, "Decrypt")

    with pytest.raises(AwsError) as err:
        encrypt_attribute_value({"S": "x"}, attr_name="secret", kms_key_arn="arn", kms_client=_FailKms())
    assert err.value.code == "AccessDeniedException"

    with pytest.raises(AwsError) as derr:
        decrypt_attribute_value(
            {"v": 1, "edk": b"x", "nonce": b"y", "ct": b"z"},
            attr_name="secret",
            kms_key_arn="arn",
            kms_client=_FailKms(),
        )
    assert derr.value.code == "AccessDeniedException"

    class _BadTypesKms:
        def generate_data_key(self, *, KeyId: str, KeySpec: str):  # noqa: N803
            return {"Plaintext": "not-bytes", "CiphertextBlob": b"edk"}

        def decrypt(self, *, CiphertextBlob: bytes, KeyId: str):  # noqa: N803
            return {"Plaintext": "not-bytes"}

    with pytest.raises(ValidationError, match="invalid key types"):
        encrypt_attribute_value({"S": "x"}, attr_name="secret", kms_key_arn="arn", kms_client=_BadTypesKms())

    with pytest.raises(ValidationError, match="invalid key types"):
        decrypt_attribute_value(
            {"v": 1, "edk": b"edk", "nonce": b"\x00" * 12, "ct": b"\x00"},
            attr_name="secret",
            kms_key_arn="arn",
            kms_client=_BadTypesKms(),
        )
