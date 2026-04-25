from __future__ import annotations

from collections.abc import Mapping, Sequence
from dataclasses import MISSING, dataclass, field, fields, is_dataclass
from typing import Any, Protocol, cast, get_type_hints, overload

from .attr_types import infer_storage_type, validate_json_storage_type
from .errors import ValidationError


class ModelDefinitionError(ValueError):
    pass


class AttributeConverter(Protocol):
    def to_dynamodb(self, value: Any) -> Any:
        pass

    def from_dynamodb(self, value: Any) -> Any:
        pass


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
    storage_type: str | None = None


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


@dataclass(frozen=True)
class WritePolicy:
    mode: str = "mutable"
    protected_attributes: Sequence[str] = ()

    def __post_init__(self) -> None:
        mode = self.mode or "mutable"
        if mode not in {"mutable", "write_once"}:
            raise ModelDefinitionError(f"unsupported write_policy.mode: {self.mode}")
        object.__setattr__(self, "mode", mode)
        if isinstance(self.protected_attributes, str):
            raise ModelDefinitionError("write_policy protected attributes must be a sequence of names")
        protected: set[str] = set()
        for attr in self.protected_attributes:
            if not isinstance(attr, str):
                raise ModelDefinitionError("write_policy protected attributes must contain strings")
            attr = attr.strip()
            if not attr:
                raise ModelDefinitionError("write_policy protected attributes must be non-empty")
            protected.add(attr)
        object.__setattr__(self, "protected_attributes", tuple(sorted(protected)))


def _resolve_write_policy(
    policy: WritePolicy | None, attributes: Mapping[str, AttributeDefinition]
) -> WritePolicy:
    resolved = policy or WritePolicy()
    protected: set[str] = set()
    by_attribute_name = {attr.attribute_name: attr for attr in attributes.values()}
    for attr in resolved.protected_attributes:
        attr_def = attributes.get(attr) or by_attribute_name.get(attr)
        if attr_def is None:
            raise ModelDefinitionError(f"write_policy protected attribute not found: {attr}")
        protected.add(attr_def.attribute_name)
    return WritePolicy(mode=resolved.mode, protected_attributes=tuple(sorted(protected)))


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
) -> Any:
    pass


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
) -> Any:
    pass


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
) -> Any:
    pass


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
    write_policy: WritePolicy = field(default_factory=WritePolicy)

    @classmethod
    def from_dataclass(
        cls,
        model_type: type[T],
        *,
        table_name: str | None = None,
        indexes: Sequence[IndexSpec] = (),
        write_policy: WritePolicy | None = None,
    ) -> ModelDefinition[T]:
        if not is_dataclass(model_type):
            raise ModelDefinitionError("model_type must be a dataclass")

        try:
            annotations = get_type_hints(model_type, include_extras=True)
        except Exception:
            annotations = cast(dict[str, Any], getattr(model_type, "__annotations__", {}))

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
            set_flag = bool(opts.get("set", False))
            json_flag = bool(opts.get("json", False))
            binary_flag = bool(opts.get("binary", False))

            if json_flag and binary_flag:
                raise ModelDefinitionError(f"field cannot be both json and binary: {dc_field.name}")
            if json_flag and set_flag:
                raise ModelDefinitionError(f"field cannot be both json and set-backed: {dc_field.name}")

            annotation = annotations.get(dc_field.name, Any)
            try:
                storage_type = infer_storage_type(
                    annotation,
                    is_set=set_flag,
                    is_json=json_flag,
                    is_binary=binary_flag,
                )
            except ValidationError as err:
                raise ModelDefinitionError(str(err)) from err
            if json_flag:
                try:
                    validate_json_storage_type(storage_type, field_name=dc_field.name)
                except ValidationError as err:
                    raise ModelDefinitionError(str(err)) from err

            attributes[dc_field.name] = AttributeDefinition(
                python_name=dc_field.name,
                attribute_name=attribute_name,
                roles=roles,
                omitempty=bool(opts.get("omitempty", False)),
                set=set_flag,
                json=json_flag,
                binary=binary_flag,
                encrypted=encrypted,
                converter=converter,
                storage_type=storage_type,
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

        resolved_write_policy = _resolve_write_policy(write_policy, attributes)

        return cls(
            model_type=model_type,
            table_name=table_name,
            pk=pk,
            sk=sk,
            attributes=attributes,
            indexes=tuple(resolved_indexes),
            write_policy=resolved_write_policy,
        )
