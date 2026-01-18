from __future__ import annotations

from collections.abc import Callable, Sequence
from decimal import Decimal
from typing import TYPE_CHECKING, Any

from botocore.exceptions import ClientError

from .aws_errors import map_client_error as _map_client_error
from .errors import ValidationError
from .model import AttributeDefinition

if TYPE_CHECKING:
    from .table import Table


class UpdateBuilder[T]:
    def __init__(self, table: Table[T], pk: Any, sk: Any | None) -> None:
        self._table = table
        self._pk = pk
        self._sk = sk
        self._return_values: str = "NONE"
        self._updates: list[tuple[str, tuple[Any, ...]]] = []
        self._conditions: list[tuple[str, str, str, Any]] = []

    def set(self, field: str, value: Any) -> UpdateBuilder[T]:
        self._updates.append(("SET", (field, value)))
        return self

    def set_if_not_exists(
        self,
        field: str,
        _value: Any,
        default_value: Any,
    ) -> UpdateBuilder[T]:
        self._updates.append(("SET_IF_NOT_EXISTS", (field, default_value)))
        return self

    def add(self, field: str, value: Any) -> UpdateBuilder[T]:
        self._updates.append(("ADD", (field, value)))
        return self

    def increment(self, field: str) -> UpdateBuilder[T]:
        return self.add(field, 1)

    def decrement(self, field: str) -> UpdateBuilder[T]:
        return self.add(field, -1)

    def remove(self, field: str) -> UpdateBuilder[T]:
        self._updates.append(("REMOVE", (field,)))
        return self

    def delete(self, field: str, value: Any) -> UpdateBuilder[T]:
        self._updates.append(("DELETE", (field, value)))
        return self

    def append_to_list(self, field: str, values: list[Any]) -> UpdateBuilder[T]:
        self._updates.append(("APPEND_LIST", (field, list(values))))
        return self

    def prepend_to_list(self, field: str, values: list[Any]) -> UpdateBuilder[T]:
        self._updates.append(("PREPEND_LIST", (field, list(values))))
        return self

    def remove_from_list_at(self, field: str, index: int) -> UpdateBuilder[T]:
        self._updates.append(("REMOVE_LIST_AT", (field, index)))
        return self

    def set_list_element(self, field: str, index: int, value: Any) -> UpdateBuilder[T]:
        self._updates.append(("SET_LIST_ELEMENT", (field, index, value)))
        return self

    def condition(self, field: str, operator: str, value: Any = None) -> UpdateBuilder[T]:
        self._conditions.append(("AND", field, operator, value))
        return self

    def or_condition(self, field: str, operator: str, value: Any = None) -> UpdateBuilder[T]:
        self._conditions.append(("OR", field, operator, value))
        return self

    def condition_exists(self, field: str) -> UpdateBuilder[T]:
        return self.condition(field, "attribute_exists", None)

    def condition_not_exists(self, field: str) -> UpdateBuilder[T]:
        return self.condition(field, "attribute_not_exists", None)

    def condition_version(self, current_version: int) -> UpdateBuilder[T]:
        version_field: str | None = None
        for name, attr in self._table._model.attributes.items():
            if "version" in attr.roles:
                version_field = name
                break
        if version_field is None:
            if "version" in self._table._model.attributes:
                version_field = "version"
        if version_field is None:
            raise ValidationError("model does not define a version field")
        return self.condition(version_field, "=", current_version)

    def return_values(self, option: str) -> UpdateBuilder[T]:
        self._return_values = option
        return self

    def execute(self) -> T | None:
        if not self._updates:
            raise ValidationError("no updates provided")

        req = self._build_request()

        try:
            resp = self._table._client.update_item(**req)
        except ClientError as err:  # pragma: no cover
            raise _map_client_error(err) from err

        attrs = resp.get("Attributes")
        if not attrs:
            return None
        return self._table._from_item(attrs)

    def _build_request(self) -> dict[str, Any]:
        key = self._table._to_key(self._pk, self._sk)

        names: dict[str, str] = {}
        update_values: dict[str, Any] = {}
        condition_values: dict[str, Any] = {}
        set_parts: list[str] = []
        remove_parts: list[str] = []
        add_parts: list[str] = []
        delete_parts: list[str] = []

        update_counter = 0
        condition_counter = 0

        def update_name_ref(field_name: str) -> tuple[str, AttributeDefinition]:
            if field_name not in self._table._model.attributes:
                raise ValidationError(f"unknown field: {field_name}")

            if field_name == self._table._model.pk.python_name or (
                self._table._model.sk is not None and field_name == self._table._model.sk.python_name
            ):
                raise ValidationError(f"cannot update key field: {field_name}")

            attr_def = self._table._model.attributes[field_name]
            ref = f"#u_{field_name}"
            names.setdefault(ref, attr_def.attribute_name)
            return ref, attr_def

        def condition_name_ref(field_name: str) -> tuple[str, AttributeDefinition]:
            if field_name not in self._table._model.attributes:
                raise ValidationError(f"unknown field: {field_name}")

            attr_def = self._table._model.attributes[field_name]
            if attr_def.encrypted:
                raise ValidationError(f"encrypted fields cannot be used in conditions: {field_name}")

            ref = f"#c_{field_name}"
            names.setdefault(ref, attr_def.attribute_name)
            return ref, attr_def

        def update_value_ref(attr_def: AttributeDefinition, field_name: str, value: Any) -> str:
            nonlocal update_counter
            update_counter += 1
            ref = f":u{update_counter}"
            update_values[ref] = self._table._serialize_attr_value(attr_def, value)
            return ref

        def raw_value_ref(value: Any) -> str:
            nonlocal update_counter
            update_counter += 1
            ref = f":u{update_counter}"
            update_values[ref] = self._table._serializer.serialize(value)
            return ref

        def condition_value_ref(attr_def: AttributeDefinition, value: Any) -> str:
            nonlocal condition_counter
            condition_counter += 1
            ref = f":c{condition_counter}"
            condition_values[ref] = self._table._serialize_attr_value(attr_def, value)
            return ref

        def normalize_set(value: Any) -> set[Any]:
            if isinstance(value, set):
                return value
            if isinstance(value, (list, tuple)):
                return set(value)
            return {value}

        for kind, args in self._updates:
            if kind == "SET":
                field_name, value = args
                ref, attr_def = update_name_ref(str(field_name))
                set_parts.append(f"{ref} = {update_value_ref(attr_def, str(field_name), value)}")
                continue

            if kind == "SET_IF_NOT_EXISTS":
                field_name, default_value = args
                ref, attr_def = update_name_ref(str(field_name))
                dv = update_value_ref(attr_def, str(field_name), default_value)
                set_parts.append(f"{ref} = if_not_exists({ref}, {dv})")
                continue

            if kind == "REMOVE":
                (field_name,) = args
                ref, _ = update_name_ref(str(field_name))
                remove_parts.append(ref)
                continue

            if kind == "ADD":
                field_name, value = args
                ref, attr_def = update_name_ref(str(field_name))
                if attr_def.encrypted:
                    raise ValidationError(f"encrypted fields cannot be used in ADD: {field_name}")
                if attr_def.set:
                    sv = normalize_set(value)
                    add_parts.append(f"{ref} {update_value_ref(attr_def, str(field_name), sv)}")
                else:
                    if not isinstance(value, (int, float, Decimal)):
                        raise ValidationError("ADD requires a numeric value for non-set fields")
                    add_parts.append(f"{ref} {raw_value_ref(value)}")
                continue

            if kind == "DELETE":
                field_name, value = args
                ref, attr_def = update_name_ref(str(field_name))
                if attr_def.encrypted:
                    raise ValidationError(f"encrypted fields cannot be used in DELETE: {field_name}")
                if not attr_def.set:
                    raise ValidationError("DELETE requires a set field")
                sv = normalize_set(value)
                delete_parts.append(f"{ref} {update_value_ref(attr_def, str(field_name), sv)}")
                continue

            if kind in {"APPEND_LIST", "PREPEND_LIST"}:
                field_name, values_list = args
                ref, attr_def = update_name_ref(str(field_name))
                if attr_def.encrypted:
                    raise ValidationError(f"encrypted fields cannot be used in list operations: {field_name}")
                if attr_def.set or attr_def.json or attr_def.binary:
                    raise ValidationError("list operations require a plain list attribute")
                if not isinstance(values_list, list):
                    raise ValidationError("list operations require list values")
                vref = raw_value_ref(values_list)
                if kind == "APPEND_LIST":
                    set_parts.append(f"{ref} = list_append({ref}, {vref})")
                else:
                    set_parts.append(f"{ref} = list_append({vref}, {ref})")
                continue

            if kind == "REMOVE_LIST_AT":
                field_name, index = args
                ref, attr_def = update_name_ref(str(field_name))
                if attr_def.encrypted:
                    raise ValidationError(f"encrypted fields cannot be used in list operations: {field_name}")
                if attr_def.set or attr_def.json or attr_def.binary:
                    raise ValidationError("list operations require a plain list attribute")
                if not isinstance(index, int) or index < 0:
                    raise ValidationError("list index must be a non-negative integer")
                remove_parts.append(f"{ref}[{index}]")
                continue

            if kind == "SET_LIST_ELEMENT":
                field_name, index, value = args
                ref, attr_def = update_name_ref(str(field_name))
                if attr_def.encrypted:
                    raise ValidationError(f"encrypted fields cannot be used in list operations: {field_name}")
                if attr_def.set or attr_def.json or attr_def.binary:
                    raise ValidationError("list operations require a plain list attribute")
                if not isinstance(index, int) or index < 0:
                    raise ValidationError("list index must be a non-negative integer")
                set_parts.append(f"{ref}[{index}] = {raw_value_ref(value)}")
                continue

            raise ValidationError(f"unsupported update operation: {kind}")

        expr_parts: list[str] = []
        if set_parts:
            expr_parts.append("SET " + ", ".join(set_parts))
        if remove_parts:
            expr_parts.append("REMOVE " + ", ".join(remove_parts))
        if add_parts:
            expr_parts.append("ADD " + ", ".join(add_parts))
        if delete_parts:
            expr_parts.append("DELETE " + ", ".join(delete_parts))
        update_expr = " ".join(expr_parts)
        if not update_expr:
            raise ValidationError("no updates provided")

        condition_expr: str | None = None
        if self._conditions:
            parts: list[str] = []
            ops: list[str] = []
            for logic, field_name, operator, value in self._conditions:
                name_ref, attr_def = condition_name_ref(field_name)
                built = _build_condition_term(
                    name_ref,
                    attr_def,
                    operator,
                    value,
                    condition_value_ref,
                )
                parts.append(built)
                if len(parts) > 1:
                    ops.append(logic)

            out = parts[0]
            for i in range(1, len(parts)):
                out += f" {ops[i - 1]} {parts[i]}"
            condition_expr = out

        req: dict[str, Any] = {
            "TableName": self._table._table_name,
            "Key": key,
            "UpdateExpression": update_expr,
            "ExpressionAttributeNames": names,
            "ReturnValues": self._return_values,
        }
        merged_values: dict[str, Any] = {}
        merged_values.update(update_values)
        merged_values.update(condition_values)
        if merged_values:
            req["ExpressionAttributeValues"] = merged_values
        if condition_expr is not None:
            req["ConditionExpression"] = condition_expr

        return req


def _build_condition_term(
    name_ref: str,
    attr_def: AttributeDefinition,
    operator: str,
    value: Any,
    value_ref: Callable[[AttributeDefinition, Any], str],
) -> str:
    op = str(operator or "").strip().upper()

    def require_value() -> Any:
        if value is None:
            raise ValidationError(f"{operator} requires one value")
        return value

    if op in {"ATTRIBUTE_EXISTS", "EXISTS"}:
        if value is not None:
            raise ValidationError("EXISTS does not take a value")
        return f"attribute_exists({name_ref})"

    if op in {"ATTRIBUTE_NOT_EXISTS", "NOT_EXISTS"}:
        if value is not None:
            raise ValidationError("NOT_EXISTS does not take a value")
        return f"attribute_not_exists({name_ref})"

    if op in {"=", "EQ"}:
        return f"{name_ref} = {value_ref(attr_def, require_value())}"
    if op in {"!=", "<>", "NE"}:
        return f"{name_ref} <> {value_ref(attr_def, require_value())}"
    if op in {"<", "LT"}:
        return f"{name_ref} < {value_ref(attr_def, require_value())}"
    if op in {"<=", "LE"}:
        return f"{name_ref} <= {value_ref(attr_def, require_value())}"
    if op in {">", "GT"}:
        return f"{name_ref} > {value_ref(attr_def, require_value())}"
    if op in {">=", "GE"}:
        return f"{name_ref} >= {value_ref(attr_def, require_value())}"
    if op == "BETWEEN":
        if not isinstance(value, (list, tuple)) or len(value) != 2:
            raise ValidationError("BETWEEN requires two values")
        left = value_ref(attr_def, value[0])
        right = value_ref(attr_def, value[1])
        return f"{name_ref} BETWEEN {left} AND {right}"
    if op == "IN":
        if value is None:
            raise ValidationError("IN requires a sequence of values")
        if not isinstance(value, Sequence) or isinstance(value, (str, bytes, bytearray, dict)):
            raise ValidationError("IN requires a sequence of values")
        if len(value) > 100:
            raise ValidationError("IN supports maximum 100 values")
        refs = [value_ref(attr_def, v) for v in value]
        return f"{name_ref} IN (" + ", ".join(refs) + ")"
    if op == "BEGINS_WITH":
        return f"begins_with({name_ref}, {value_ref(attr_def, require_value())})"
    if op == "CONTAINS":
        return f"contains({name_ref}, {value_ref(attr_def, require_value())})"

    raise ValidationError(f"unsupported condition operator: {operator}")
