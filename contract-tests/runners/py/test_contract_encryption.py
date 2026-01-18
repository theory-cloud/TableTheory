from __future__ import annotations

import pytest

from theorydb_py.encryption import decrypt_attribute_value, encrypt_attribute_value
from theorydb_py.errors import ValidationError


class _StubKms:
    def __init__(self, *, dek: bytes) -> None:
        self._dek = dek

    def generate_data_key(self, *, KeyId: str, KeySpec: str):  # noqa: N803
        assert KeyId
        assert KeySpec
        return {"Plaintext": self._dek, "CiphertextBlob": b"edk"}

    def decrypt(self, *, CiphertextBlob: bytes, KeyId: str):  # noqa: N803
        assert CiphertextBlob
        assert KeyId
        return {"Plaintext": self._dek}


def test_encryption_envelope_shape_and_aad_binding() -> None:
    kms = _StubKms(dek=b"\x00" * 32)
    av = {"S": "top-secret"}

    env = encrypt_attribute_value(
        av,
        attr_name="secret",
        kms_key_arn="arn:aws:kms:us-east-1:1:key/x",
        kms_client=kms,
    )
    assert env["v"] == 1
    assert isinstance(env["edk"], (bytes, bytearray))
    assert isinstance(env["nonce"], (bytes, bytearray))
    assert isinstance(env["ct"], (bytes, bytearray))

    out = decrypt_attribute_value(
        env,
        attr_name="secret",
        kms_key_arn="arn:aws:kms:us-east-1:1:key/x",
        kms_client=kms,
    )
    assert out == {"S": "top-secret"}

    with pytest.raises(ValidationError):
        decrypt_attribute_value(
            env,
            attr_name="other",
            kms_key_arn="arn:aws:kms:us-east-1:1:key/x",
            kms_client=kms,
        )

