from __future__ import annotations

from dataclasses import dataclass

import pytest

from theorydb_py.model import ModelDefinition, ModelDefinitionError, Projection, gsi, lsi, theorydb_field


@dataclass(frozen=True)
class User:
    pk: str = theorydb_field(name="PK", roles=["pk"])
    sk: str = theorydb_field(name="SK", roles=["sk"])
    email_hash: str = theorydb_field(name="emailHash", omitempty=True)
    created_at: str = theorydb_field(name="createdAt", roles=["created_at"])
    tags: set[str] = theorydb_field(name="tags", set_=True, omitempty=True, default_factory=set)
    payload: dict[str, int] = theorydb_field(name="payload", json=True, omitempty=True, default_factory=dict)
    blob: bytes = theorydb_field(name="blob", binary=True, omitempty=True, default=b"")
    secret: str = theorydb_field(name="secret", encrypted=True, default="")
    ignored: str = theorydb_field(ignore=True, default="ignored")


def test_model_definition_extracts_keys_attributes_and_indexes() -> None:
    model = ModelDefinition.from_dataclass(
        User,
        table_name="users",
        indexes=[
            gsi("gsi-email", partition="email_hash", projection=Projection.keys_only()),
            lsi("lsi-created-at", sort="created_at"),
        ],
    )
    assert model.pk.attribute_name == "PK"
    assert model.sk is not None and model.sk.attribute_name == "SK"
    assert model.attributes["email_hash"].attribute_name == "emailHash"
    assert model.attributes["tags"].set is True
    assert model.attributes["payload"].json is True
    assert model.attributes["blob"].binary is True
    assert model.attributes["secret"].encrypted is True
    assert "ignored" not in model.attributes

    assert len(model.indexes) == 2
    assert model.indexes[0].type == "GSI" and model.indexes[0].partition == "emailHash"
    assert model.indexes[1].type == "LSI" and model.indexes[1].partition == "PK"


def test_model_definition_rejects_missing_pk() -> None:
    @dataclass(frozen=True)
    class Bad:
        sk: str = theorydb_field(roles=["sk"])

    with pytest.raises(ModelDefinitionError, match="exactly one pk"):
        ModelDefinition.from_dataclass(Bad)


def test_model_definition_rejects_multiple_pk() -> None:
    @dataclass(frozen=True)
    class Bad:
        pk1: str = theorydb_field(roles=["pk"])
        pk2: str = theorydb_field(roles=["pk"])

    with pytest.raises(ModelDefinitionError, match="exactly one pk"):
        ModelDefinition.from_dataclass(Bad)


def test_model_definition_rejects_encrypted_key() -> None:
    @dataclass(frozen=True)
    class Bad:
        pk: str = theorydb_field(roles=["pk"], encrypted=True)

    with pytest.raises(ModelDefinitionError, match="encrypted field cannot be a key"):
        ModelDefinition.from_dataclass(Bad)


def test_model_definition_rejects_encrypted_index_key() -> None:
    @dataclass(frozen=True)
    class Bad:
        pk: str = theorydb_field(roles=["pk"])
        secret: str = theorydb_field(encrypted=True)

    with pytest.raises(ModelDefinitionError, match="encrypted partition field is not allowed"):
        ModelDefinition.from_dataclass(Bad, indexes=[gsi("gsi-secret", partition="secret")])
