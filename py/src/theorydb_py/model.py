from __future__ import annotations

from collections.abc import Mapping, Sequence
from dataclasses import MISSING, dataclass, field, fields, is_dataclass
from typing import Any, Protocol, cast, overload


class ModelDefinitionError(ValueError):
    pass


class AttributeConverter(Protocol):
    def to_dynamodb(self, value: Any) -> Any: ...

    def from_dynamodb(self, value: Any) -> Any: ...


@dataclass(frozen=True)
class AttributeDefinition:
    python_name: str
    attribute_name: str
    roles: tuple[str, ...]
    omitempty: bool
    set: bool
    json: bool
    binary: bool
    encrypted: bool
    converter: AttributeConverter | None = None


@dataclass(frozen=True)
class Projection:
    type: str
    fields: tuple[str, ...] = ()

    @staticmethod
    def all() -> Projection:
        return Projection(type="ALL")

    @staticmethod
    def keys_only() -> Projection:
        return Projection(type="KEYS_ONLY")

    @staticmethod
    def include(*fields: str) -> Projection:
        return Projection(type="INCLUDE", fields=tuple(fields))


@dataclass(frozen=True)
class IndexSpec:
    name: str
    type: str
    partition: str
    sort: str | None = None
    projection: Projection = field(default_factory=Projection.all)


@dataclass(frozen=True)
class IndexDefinition:
    name: str
    type: str
    partition: str
    sort: str | None = None
    projection: Projection = field(default_factory=Projection.all)


@overload
def theorydb_field(
    *,
    name: str | None = None,
    roles: Sequence[str] | None = None,
    omitempty: bool = False,
    set_: bool = False,
    json: bool = False,
    binary: bool = False,
    encrypted: bool = False,
    converter: AttributeConverter | None = None,
    ignore: bool = False,
) -> Any: ...


@overload
def theorydb_field(
    *,
    name: str | None = None,
    roles: Sequence[str] | None = None,
    omitempty: bool = False,
    set_: bool = False,
    json: bool = False,
    binary: bool = False,
    encrypted: bool = False,
    converter: AttributeConverter | None = None,
    ignore: bool = False,
    default: Any,
) -> Any: ...


@overload
def theorydb_field(
    *,
    name: str | None = None,
    roles: Sequence[str] | None = None,
    omitempty: bool = False,
    set_: bool = False,
    json: bool = False,
    binary: bool = False,
    encrypted: bool = False,
    converter: AttributeConverter | None = None,
    ignore: bool = False,
    default_factory: Any,
) -> Any: ...


def theorydb_field(
    *,
    name: str | None = None,
    roles: Sequence[str] | None = None,
    omitempty: bool = False,
    set_: bool = False,
    json: bool = False,
    binary: bool = False,
    encrypted: bool = False,
    converter: AttributeConverter | None = None,
    ignore: bool = False,
    default: Any = MISSING,
    default_factory: Any = MISSING,
) -> Any:
    if default is not MISSING and default_factory is not MISSING:
        raise ValueError("theorydb_field: cannot set both default and default_factory")

    theorydb: dict[str, Any] = {
        "omitempty": omitempty,
        "set": set_,
        "json": json,
        "binary": binary,
        "encrypted": encrypted,
        "converter": converter,
        "ignore": ignore,
    }
    if name is not None:
        theorydb["name"] = name
    if roles is not None:
        theorydb["roles"] = list(roles)

    return field(default=default, default_factory=default_factory, metadata={"theorydb": theorydb})


def gsi(
    name: str,
    *,
    partition: str,
    sort: str | None = None,
    projection: Projection | None = None,
) -> IndexSpec:
    return IndexSpec(
        name=name, type="GSI", partition=partition, sort=sort, projection=projection or Projection.all()
    )


def lsi(name: str, *, sort: str, projection: Projection | None = None) -> IndexSpec:
    return IndexSpec(
        name=name, type="LSI", partition="__TABLE_PK__", sort=sort, projection=projection or Projection.all()
    )


@dataclass(frozen=True)
class ModelDefinition[T]:
    model_type: type[T]
    table_name: str | None
    pk: AttributeDefinition
    sk: AttributeDefinition | None
    attributes: Mapping[str, AttributeDefinition]
    indexes: tuple[IndexDefinition, ...]

    @classmethod
    def from_dataclass(
        cls,
        model_type: type[T],
        *,
        table_name: str | None = None,
        indexes: Sequence[IndexSpec] = (),
    ) -> ModelDefinition[T]:
        if not is_dataclass(model_type):
            raise ModelDefinitionError("model_type must be a dataclass")

        attributes: dict[str, AttributeDefinition] = {}
        pk_fields: list[str] = []
        sk_fields: list[str] = []

        for dc_field in fields(model_type):
            opts = cast(dict[str, Any], dc_field.metadata.get("theorydb", {}))
            ignore = bool(opts.get("ignore", False))
            if ignore:
                continue

            roles = tuple(cast(list[str], opts.get("roles", [])))
            if "pk" in roles:
                pk_fields.append(dc_field.name)
            if "sk" in roles:
                sk_fields.append(dc_field.name)

            encrypted = bool(opts.get("encrypted", False))
            if encrypted:
                if "pk" in roles or "sk" in roles:
                    raise ModelDefinitionError(f"encrypted field cannot be a key: {dc_field.name}")
                for role in roles:
                    if role.startswith("index_pk:") or role.startswith("index_sk:"):
                        raise ModelDefinitionError(f"encrypted field cannot be indexed: {dc_field.name}")

            converter = cast(AttributeConverter | None, opts.get("converter"))
            attribute_name = cast(str, opts.get("name", dc_field.name))
            attributes[dc_field.name] = AttributeDefinition(
                python_name=dc_field.name,
                attribute_name=attribute_name,
                roles=roles,
                omitempty=bool(opts.get("omitempty", False)),
                set=bool(opts.get("set", False)),
                json=bool(opts.get("json", False)),
                binary=bool(opts.get("binary", False)),
                encrypted=encrypted,
                converter=converter,
            )

        if len(pk_fields) != 1:
            raise ModelDefinitionError(f"model must define exactly one pk field (found {len(pk_fields)})")

        if len(sk_fields) > 1:
            raise ModelDefinitionError(f"model must define at most one sk field (found {len(sk_fields)})")

        pk = attributes[pk_fields[0]]
        sk = attributes[sk_fields[0]] if sk_fields else None

        resolved_indexes: list[IndexDefinition] = []
        seen_index_names: set[str] = set()

        for spec in indexes:
            if spec.name in seen_index_names:
                raise ModelDefinitionError(f"duplicate index name: {spec.name}")
            seen_index_names.add(spec.name)

            if spec.type not in {"GSI", "LSI"}:
                raise ModelDefinitionError(f"unsupported index type: {spec.type}")

            partition_field = (
                pk.python_name if spec.type == "LSI" and spec.partition == "__TABLE_PK__" else spec.partition
            )
            if partition_field not in attributes:
                raise ModelDefinitionError(f"index {spec.name}: unknown partition field: {partition_field}")
            if attributes[partition_field].encrypted:
                raise ModelDefinitionError(
                    f"index {spec.name}: encrypted partition field is not allowed: {partition_field}"
                )

            if spec.type == "LSI" and partition_field != pk.python_name:
                raise ModelDefinitionError(
                    f"index {spec.name}: LSI partition must be the table pk ({pk.python_name})"
                )

            sort_attr: str | None = None
            if spec.sort is not None:
                if spec.sort not in attributes:
                    raise ModelDefinitionError(f"index {spec.name}: unknown sort field: {spec.sort}")
                if attributes[spec.sort].encrypted:
                    raise ModelDefinitionError(
                        f"index {spec.name}: encrypted sort field is not allowed: {spec.sort}"
                    )
                sort_attr = attributes[spec.sort].attribute_name

            resolved_indexes.append(
                IndexDefinition(
                    name=spec.name,
                    type=spec.type,
                    partition=attributes[partition_field].attribute_name,
                    sort=sort_attr,
                    projection=spec.projection,
                )
            )

        return cls(
            model_type=model_type,
            table_name=table_name,
            pk=pk,
            sk=sk,
            attributes=attributes,
            indexes=tuple(resolved_indexes),
        )
