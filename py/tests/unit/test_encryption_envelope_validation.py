from __future__ import annotations

import json

import pytest
from boto3.dynamodb.types import Binary
from botocore.exceptions import ClientError
from cryptography.hazmat.primitives.ciphers.aead import AESGCM

from tabletheory_py.encryption import (
    RecordBinding,
    build_aad,
    canonical_key_value,
    decrypt_attribute_value,
    encrypt_attribute_value,
)
from tabletheory_py.errors import AwsError, InvalidEncryptedEnvelopeError, ValidationError

_KEY_ARN = "arn:aws:kms:us-east-1:000000000000:key/test"


class _GoodKms:
    def __init__(self) -> None:
        self._dek = b"\x02" * 32
        self._edk = b"edk"
        self.generate_contexts: list[dict[str, str] | None] = []
        self.decrypt_contexts: list[dict[str, str] | None] = []

    def generate_data_key(
        self,
        *,
        KeyId: str,  # noqa: N803
        KeySpec: str,  # noqa: N803
        EncryptionContext: dict[str, str] | None = None,  # noqa: N803
    ):
        assert KeyId
        assert KeySpec == "AES_256"
        self.generate_contexts.append(EncryptionContext)
        return {"Plaintext": self._dek, "CiphertextBlob": self._edk}

    def decrypt(
        self,
        *,
        CiphertextBlob: bytes,  # noqa: N803
        KeyId: str,  # noqa: N803
        EncryptionContext: dict[str, str] | None = None,  # noqa: N803
    ):
        assert KeyId
        assert CiphertextBlob == self._edk
        self.decrypt_contexts.append(EncryptionContext)
        return {"Plaintext": self._dek}


def test_encrypt_and_decrypt_round_trip_with_v1_aad_binding() -> None:
    kms = _GoodKms()
    envelope = encrypt_attribute_value(
        {"S": "top-secret"},
        attr_name="secret",
        kms_key_arn=_KEY_ARN,
        kms_client=kms,
    )
    assert set(envelope.keys()) == {"v", "edk", "nonce", "ct"}
    assert envelope["v"] == 1

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
        kms_key_arn=_KEY_ARN,
        kms_client=kms,
    )
    assert decrypted == {"S": "top-secret"}

    # A v1 envelope keeps decrypting even when a binding is supplied: dispatch
    # is on the envelope version, not on the caller.
    decrypted_with_binding = decrypt_attribute_value(
        envelope,
        attr_name="secret",
        kms_key_arn=_KEY_ARN,
        kms_client=kms,
        binding=RecordBinding(table="notes", partition_key={"S": "USER#1"}),
    )
    assert decrypted_with_binding == {"S": "top-secret"}

    with pytest.raises(ValidationError, match="failed to decrypt"):
        decrypt_attribute_value(
            envelope,
            attr_name="other",
            kms_key_arn=_KEY_ARN,
            kms_client=kms,
        )


def test_canonical_key_value_encodes_type_and_base64() -> None:
    assert canonical_key_value({"S": "USER#1"}) == "S:USER#1"
    assert canonical_key_value({"N": "42"}) == "N:42"
    assert canonical_key_value({"B": b"\x01\x02"}) == "B:AQI="


def test_v2_envelope_binds_record_identity_and_rejects_replay() -> None:
    kms = _GoodKms()
    binding = RecordBinding(
        table="notes",
        partition_key={"S": "USER#1"},
        sort_key={"S": "v1"},
    )
    expected_aad = b"theorydb:encrypted:v2|attr=6:secret|table=5:notes|pk=8:S:USER#1|sk=4:S:v1"

    envelope = encrypt_attribute_value(
        {"S": "top-secret"},
        attr_name="secret",
        kms_key_arn=_KEY_ARN,
        kms_client=kms,
        binding=binding,
    )
    assert envelope["v"] == 2
    assert build_aad("secret", binding) == expected_aad
    assert kms.generate_contexts[-1] == {"theorydb:encryption:aad": expected_aad.decode("utf-8")}

    plaintext = AESGCM(kms._dek).decrypt(envelope["nonce"], envelope["ct"], expected_aad)
    assert json.loads(plaintext.decode("utf-8")) == {"t": "S", "s": "top-secret"}

    assert decrypt_attribute_value(
        envelope,
        attr_name="secret",
        kms_key_arn=_KEY_ARN,
        kms_client=kms,
        binding=binding,
    ) == {"S": "top-secret"}
    assert kms.decrypt_contexts[-1] == kms.generate_contexts[-1]

    with pytest.raises(ValidationError, match="failed to decrypt"):
        decrypt_attribute_value(
            envelope,
            attr_name="secret",
            kms_key_arn=_KEY_ARN,
            kms_client=kms,
            binding=RecordBinding(
                table="notes",
                partition_key={"S": "USER#2"},
                sort_key={"S": "v1"},
            ),
        )

    with pytest.raises(ValidationError, match="failed to decrypt"):
        decrypt_attribute_value(
            envelope,
            attr_name="secret",
            kms_key_arn=_KEY_ARN,
            kms_client=kms,
            binding=RecordBinding(
                table="other_table",
                partition_key={"S": "USER#1"},
                sort_key={"S": "v1"},
            ),
        )

    with pytest.raises(ValidationError, match="requires the record binding"):
        decrypt_attribute_value(
            envelope,
            attr_name="secret",
            kms_key_arn=_KEY_ARN,
            kms_client=kms,
        )


def test_v2_aad_omits_sort_key_when_absent() -> None:
    binding = RecordBinding(table="notes", partition_key={"N": "42"})
    assert build_aad("secret", binding) == b"theorydb:encrypted:v2|attr=6:secret|table=5:notes|pk=4:N:42"


def test_v2_aad_length_prefixes_prevent_delimiter_collisions() -> None:
    first_binding = RecordBinding(
        table="T",
        partition_key={"S": "x|sk=S:y"},
        sort_key={"S": "z"},
    )
    second_binding = RecordBinding(
        table="T",
        partition_key={"S": "x"},
        sort_key={"S": "y|sk=S:z"},
    )
    first = build_aad("secret", first_binding)
    second = build_aad("secret", second_binding)

    assert first != second
    assert first == b"theorydb:encrypted:v2|attr=6:secret|table=1:T|pk=10:S:x|sk=S:y|sk=3:S:z"
    assert second == b"theorydb:encrypted:v2|attr=6:secret|table=1:T|pk=3:S:x|sk=10:S:y|sk=S:z"

    kms = _GoodKms()
    envelope = encrypt_attribute_value(
        {"S": "payload"},
        attr_name="secret",
        kms_key_arn=_KEY_ARN,
        kms_client=kms,
        binding=first_binding,
    )
    with pytest.raises(InvalidEncryptedEnvelopeError, match="failed to decrypt"):
        decrypt_attribute_value(
            envelope,
            attr_name="secret",
            kms_key_arn=_KEY_ARN,
            kms_client=kms,
            binding=second_binding,
        )


def test_unsupported_key_types_fail_closed() -> None:
    with pytest.raises(InvalidEncryptedEnvelopeError, match="unsupported key attribute value type"):
        build_aad("secret", RecordBinding(table="T", partition_key={"NULL": True}))
    with pytest.raises(InvalidEncryptedEnvelopeError, match="unsupported key attribute value type"):
        build_aad("secret", RecordBinding(table="T", partition_key={"BOOL": True}))


def test_v2_encrypt_requires_a_valid_binding() -> None:
    with pytest.raises(ValidationError, match="record binding"):
        encrypt_attribute_value(
            {"S": "x"},
            attr_name="secret",
            kms_key_arn=_KEY_ARN,
            kms_client=_GoodKms(),
            binding=RecordBinding(table="", partition_key={"S": "USER#1"}),
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
            {"v": 3, "edk": b"x", "nonce": b"y", "ct": b"z"},
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
