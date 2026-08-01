from __future__ import annotations

from dataclasses import dataclass

import pytest

from tabletheory_py.model import (
    ModelDefinition,
    ModelDefinitionError,
    Projection,
    Role,
    WritePolicy,
    gsi,
    lsi,
    theorydb_field,
)


@dataclass(frozen=True)
class User:
    pk: str = theorydb_field(name="PK", roles=[Role.PK])
    sk: str = theorydb_field(name="SK", roles=[Role.SK])
    email_hash: str = theorydb_field(name="emailHash", omitempty=True)
    created_at: str = theorydb_field(name="createdAt", roles=[Role.CREATED_AT])
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
    assert model.attributes["payload"].storage_type == "M"
    assert model.attributes["blob"].binary is True
    assert model.attributes["secret"].encrypted is True
    assert "ignored" not in model.attributes

    assert len(model.indexes) == 2
    assert model.indexes[0].type == "GSI" and model.indexes[0].partition == "emailHash"
    assert model.indexes[1].type == "LSI" and model.indexes[1].partition == "PK"


def test_model_definition_accepts_role_constants_and_strings() -> None:
    @dataclass(frozen=True)
    class MixedRoles:
        pk: str = theorydb_field(roles=[Role.PK])
        sk: str = theorydb_field(roles=["sk"])
        ttl: int = theorydb_field(roles=[Role.TTL], default=0)

    model = ModelDefinition.from_dataclass(MixedRoles)

    assert model.pk.roles == ("pk",)
    assert model.sk is not None and model.sk.roles == ("sk",)
    assert model.attributes["ttl"].roles == ("ttl",)


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


def test_model_definition_rejects_duplicate_database_attribute_names() -> None:
    @dataclass(frozen=True)
    class Collision:
        pk: str = theorydb_field(name="PK", roles=["pk"])
        first: str = theorydb_field(name="shared", default="")
        second: str = theorydb_field(name="shared", default="")

    with pytest.raises(
        ModelDefinitionError,
        match="duplicate database attribute name 'shared' for fields first and second",
    ):
        ModelDefinition.from_dataclass(Collision)


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


def test_model_definition_rejects_json_set_and_json_binary() -> None:
    @dataclass(frozen=True)
    class JsonSet:
        pk: str = theorydb_field(roles=["pk"])
        payload: set[str] = theorydb_field(json=True, set_=True, default_factory=set)

    with pytest.raises(ModelDefinitionError, match="json and set-backed"):
        ModelDefinition.from_dataclass(JsonSet)

    @dataclass(frozen=True)
    class JsonBinary:
        pk: str = theorydb_field(roles=["pk"])
        payload: bytes = theorydb_field(json=True, binary=True, default=b"")

    with pytest.raises(ModelDefinitionError, match="json and binary"):
        ModelDefinition.from_dataclass(JsonBinary)


def test_model_definition_rejects_unsupported_union_annotations() -> None:
    @dataclass(frozen=True)
    class BadUnion:
        pk: str = theorydb_field(roles=["pk"])
        value: int | str | None = theorydb_field(default=None)

    with pytest.raises(ModelDefinitionError, match="unsupported union annotation"):
        ModelDefinition.from_dataclass(BadUnion)


def test_model_definition_normalizes_write_policy() -> None:
    model = ModelDefinition.from_dataclass(
        User,
        table_name="users",
        write_policy=WritePolicy(
            mode="",
            protected_attributes=[" emailHash ", "email_hash", "emailHash"],
        ),
    )

    assert model.write_policy.mode == "mutable"
    assert model.write_policy.protected_attributes == ("emailHash",)


def test_model_definition_rejects_invalid_write_policy() -> None:
    with pytest.raises(ModelDefinitionError, match="unsupported write_policy.mode"):
        WritePolicy(mode="frozen")

    with pytest.raises(ModelDefinitionError, match="protected attributes must be a sequence"):
        WritePolicy(protected_attributes="emailHash")  # type: ignore[arg-type]

    with pytest.raises(ModelDefinitionError, match="must contain strings"):
        WritePolicy(protected_attributes=[123])  # type: ignore[list-item]

    with pytest.raises(ModelDefinitionError, match="must be non-empty"):
        WritePolicy(protected_attributes=[" "])

    with pytest.raises(ModelDefinitionError, match="protected attribute not found"):
        ModelDefinition.from_dataclass(User, write_policy=WritePolicy(protected_attributes=["missing"]))
