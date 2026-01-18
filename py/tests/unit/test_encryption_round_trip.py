from __future__ import annotations

from dataclasses import dataclass

import pytest

from theorydb_py import EncryptionNotConfiguredError, ModelDefinition, Table, theorydb_field


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
        kms_key_arn="arn:aws:kms:us-east-1:000000000000:key/test",
        kms_client=_FakeKms(),
    )

    item = SecretThing(pk="A", sk="B", secret="top-secret")
    stored = table._to_item(item)
    assert "M" in stored["secret"]
    assert set(stored["secret"]["M"].keys()) == {"v", "edk", "nonce", "ct"}

    got = table._from_item(stored)
    assert got == item
