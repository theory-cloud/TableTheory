from __future__ import annotations

from dataclasses import dataclass

import pytest

from tabletheory_py import (
    EncryptionNotConfiguredError,
    ModelDefinition,
    Table,
    ValidationError,
    theorydb_field,
)
from tabletheory_py.encryption import encrypt_attribute_value

_KEY_ARN = "arn:aws:kms:us-east-1:000000000000:key/test"


class _FakeKms:
    def __init__(self) -> None:
        self._dek = b"\x01" * 32
        self._edk = b"edk"

    def generate_data_key(self, *, KeyId: str, KeySpec: str):  # noqa: N803
        assert KeyId
        assert KeySpec == "AES_256"
        return {"Plaintext": self._dek, "CiphertextBlob": self._edk}

    def decrypt(self, *, CiphertextBlob: bytes, KeyId: str):  # noqa: N803
        assert KeyId
        assert CiphertextBlob == self._edk
        return {"Plaintext": self._dek}


@dataclass(frozen=True)
class SecretThing:
    pk: str = theorydb_field(roles=["pk"])
    sk: str = theorydb_field(roles=["sk"])
    secret: str = theorydb_field(encrypted=True)


def test_encrypted_model_requires_kms_key_arn() -> None:
    model = ModelDefinition.from_dataclass(SecretThing, table_name="tbl")

    with pytest.raises(EncryptionNotConfiguredError):
        Table(model, client=object())


def test_encrypted_field_round_trip_encrypts_and_decrypts() -> None:
    model = ModelDefinition.from_dataclass(SecretThing, table_name="tbl")
    table: Table[SecretThing] = Table(
        model,
        client=object(),
        kms_key_arn=_KEY_ARN,
        kms_client=_FakeKms(),
    )

    item = SecretThing(pk="A", sk="B", secret="top-secret")
    stored = table._to_item(item)
    assert "M" in stored["secret"]
    assert set(stored["secret"]["M"].keys()) == {"v", "edk", "nonce", "ct"}
    assert stored["secret"]["M"]["v"] == {"N": "2"}

    got = table._from_item(stored)
    assert got == item

    replayed = {"pk": {"S": "C"}, "sk": {"S": "D"}, "secret": stored["secret"]}
    with pytest.raises(ValidationError, match="failed to decrypt"):
        table._from_item(replayed)


def test_legacy_v1_envelope_still_decrypts_through_table() -> None:
    model = ModelDefinition.from_dataclass(SecretThing, table_name="tbl")
    kms = _FakeKms()
    table: Table[SecretThing] = Table(
        model,
        client=object(),
        kms_key_arn=_KEY_ARN,
        kms_client=kms,
    )

    legacy = encrypt_attribute_value(
        {"S": "legacy"},
        attr_name="secret",
        kms_key_arn=_KEY_ARN,
        kms_client=kms,
    )
    assert legacy["v"] == 1
    raw = {
        "pk": {"S": "A"},
        "sk": {"S": "B"},
        "secret": {
            "M": {
                "v": {"N": str(legacy["v"])},
                "edk": {"B": legacy["edk"]},
                "nonce": {"B": legacy["nonce"]},
                "ct": {"B": legacy["ct"]},
            }
        },
    }

    assert table._from_item(raw) == SecretThing(pk="A", sk="B", secret="legacy")
