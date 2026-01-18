from __future__ import annotations

from dataclasses import dataclass
from typing import Any, cast

import pytest

from theorydb_py.model import (
    IndexSpec,
    ModelDefinition,
    ModelDefinitionError,
    Projection,
    gsi,
    theorydb_field,
)


@dataclass(frozen=True)
class _Base:
    pk: str = theorydb_field(roles=["pk"])
    sk: str = theorydb_field(roles=["sk"], default="")
    other: str = theorydb_field(default="")


def test_projection_include_sets_fields() -> None:
    proj = Projection.include("a", "b")
    assert proj.type == "INCLUDE"
    assert proj.fields == ("a", "b")


def test_theorydb_field_rejects_default_and_default_factory() -> None:
    with pytest.raises(ValueError, match="cannot set both default and default_factory"):
        theorydb_field(default=1, default_factory=list)  # type: ignore[arg-type]


def test_model_definition_rejects_model_type_not_dataclass() -> None:
    with pytest.raises(ModelDefinitionError, match="model_type must be a dataclass"):
        ModelDefinition.from_dataclass(cast(Any, int))


def test_model_definition_rejects_multiple_sk_fields() -> None:
    @dataclass(frozen=True)
    class Bad:
        pk: str = theorydb_field(roles=["pk"])
        sk1: str = theorydb_field(roles=["sk"])
        sk2: str = theorydb_field(roles=["sk"])

    with pytest.raises(ModelDefinitionError, match="at most one sk"):
        ModelDefinition.from_dataclass(Bad)


def test_model_definition_rejects_encrypted_index_roles() -> None:
    @dataclass(frozen=True)
    class Bad:
        pk: str = theorydb_field(roles=["pk"])
        secret: str = theorydb_field(encrypted=True, roles=["index_pk:gsi1"])

    with pytest.raises(ModelDefinitionError, match="cannot be indexed"):
        ModelDefinition.from_dataclass(Bad)


def test_model_definition_rejects_duplicate_index_names() -> None:
    with pytest.raises(ModelDefinitionError, match="duplicate index name"):
        ModelDefinition.from_dataclass(
            _Base, indexes=[gsi("dup", partition="pk"), gsi("dup", partition="pk")]
        )


def test_model_definition_rejects_unsupported_index_type() -> None:
    spec = IndexSpec(name="bad", type="BAD", partition="pk")
    with pytest.raises(ModelDefinitionError, match="unsupported index type"):
        ModelDefinition.from_dataclass(_Base, indexes=[spec])


def test_model_definition_rejects_unknown_partition_field() -> None:
    with pytest.raises(ModelDefinitionError, match="unknown partition field"):
        ModelDefinition.from_dataclass(_Base, indexes=[gsi("g", partition="missing")])


def test_model_definition_rejects_lsi_partition_not_table_pk() -> None:
    spec = IndexSpec(name="lsi", type="LSI", partition="other", sort="sk")
    with pytest.raises(ModelDefinitionError, match="LSI partition must be the table pk"):
        ModelDefinition.from_dataclass(_Base, indexes=[spec])


def test_model_definition_rejects_unknown_sort_field() -> None:
    spec = IndexSpec(name="g", type="GSI", partition="pk", sort="missing")
    with pytest.raises(ModelDefinitionError, match="unknown sort field"):
        ModelDefinition.from_dataclass(_Base, indexes=[spec])


def test_model_definition_rejects_encrypted_sort_field() -> None:
    @dataclass(frozen=True)
    class Bad:
        pk: str = theorydb_field(roles=["pk"])
        secret: str = theorydb_field(encrypted=True, default="")

    with pytest.raises(ModelDefinitionError, match="encrypted sort field is not allowed"):
        ModelDefinition.from_dataclass(Bad, indexes=[gsi("g", partition="pk", sort="secret")])
