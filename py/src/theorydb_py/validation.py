from __future__ import annotations

import re
from collections.abc import Mapping, Sequence
from typing import Any

MaxFieldNameLength = 255
MaxOperatorLength = 20
MaxValueStringLength = 400000  # DynamoDB item size limit (approx)
MaxNestedDepth = 32
MaxExpressionLength = 4096


class SecurityValidationError(Exception):
    def __init__(self, *, type: str, detail: str) -> None:
        super().__init__(f"security validation failed: {type}")
        self.type = type
        self.detail = detail


_DANGEROUS_PATTERNS = (
    "'",
    '"',
    ";",
    "--",
    "/*",
    "*/",
    "<script",
    "</script",
    "eval(",
    "expression(",
    "import(",
    "require(",
)

_SQL_KEYWORDS = (
    "union",
    "select",
    "insert",
    "update",
    "delete",
    "drop",
    "alter",
    "exec",
    "execute",
    "script",
    "javascript",
    "vbscript",
)

_LEGITIMATE_FIELD_PATTERNS = (
    re.compile(r"^(created|updated)at$", re.IGNORECASE),
    re.compile(r"^create(d|r)_?(at|time|date)$", re.IGNORECASE),
    re.compile(r"^update(d|r)_?(at|time|date)$", re.IGNORECASE),
    re.compile(r"^delete(d|r)_?(at|time|date|flag)$", re.IGNORECASE),
    re.compile(r"^insert(ed|er)_?(at|time|date)$", re.IGNORECASE),
    re.compile(r"^select(ed|or)_?(at|time|date)$", re.IGNORECASE),
)

_VALUE_SCRIPT_PATTERNS = (
    "<script",
    "</script",
    "eval(",
    "expression(",
    "import(",
    "require(",
    "javascript:",
    "vbscript:",
    "onload=",
    "onerror=",
    "onclick=",
)

_VALUE_SQL_INJECTION_PATTERNS = (
    "'; drop table",
    "'; delete from",
    "'; update ",
    "'; insert into",
    '"; drop table',
    '"; delete from',
    '"; update ',
    '"; insert into',
    "' or 1=1",
    '" or 1=1',
    "' or '1'='1",
    '" or "1"="1',
    "/**/union/**/select",
    "concat(0x",
    "char(",
    "load_file(",
    "--",
)

_ALLOWED_OPERATORS = {
    "=",
    "!=",
    "<>",
    "<",
    "<=",
    ">",
    ">=",
    "BETWEEN",
    "IN",
    "BEGINS_WITH",
    "CONTAINS",
    "EXISTS",
    "NOT_EXISTS",
    "ATTRIBUTE_EXISTS",
    "ATTRIBUTE_NOT_EXISTS",
    "EQ",
    "NE",
    "LT",
    "LE",
    "GT",
    "GE",
}


def validate_field_name(field: str) -> None:
    _validate_field_name_basics(field)

    lower = field.lower()
    if _contains_any_substring(lower, _DANGEROUS_PATTERNS):
        raise SecurityValidationError(type="InjectionAttempt", detail="field name contains dangerous pattern")

    _validate_field_name_keywords(lower, field)

    if _contains_control_characters(field):
        raise SecurityValidationError(type="InvalidField", detail="field name contains control characters")

    if "." in field:
        _validate_nested_field_path(field)
        return

    _validate_field_part(field)


def _validate_field_name_basics(field: str) -> None:
    if not field:
        raise SecurityValidationError(type="InvalidField", detail="field name cannot be empty")
    if len(field) > MaxFieldNameLength:
        raise SecurityValidationError(type="InvalidField", detail="field name exceeds maximum length")


def _validate_field_name_keywords(field_lower: str, field: str) -> None:
    for kw in _SQL_KEYWORDS:
        if kw not in field_lower:
            continue
        if _is_legitimate_field_name(field):
            continue
        if _is_standalone_or_suspicious_keyword(field_lower, kw):
            raise SecurityValidationError(
                type="InjectionAttempt", detail="field name contains suspicious content"
            )


def _is_legitimate_field_name(field: str) -> bool:
    return any(p.match(field) is not None for p in _LEGITIMATE_FIELD_PATTERNS)


def _contains_control_characters(value: str) -> bool:
    for ch in value:
        code = ord(ch)
        if 0 <= code <= 0x1F or code == 0x7F:
            return True
    return False


def _validate_nested_field_path(field: str) -> None:
    parts = field.split(".")
    if len(parts) > MaxNestedDepth:
        raise SecurityValidationError(type="InvalidField", detail="nested field depth exceeds maximum")

    for part in parts:
        try:
            _validate_field_part(part)
        except SecurityValidationError as err:
            raise SecurityValidationError(type="InvalidField", detail="invalid field part") from err


def _is_standalone_or_suspicious_keyword(field_lower: str, keyword: str) -> bool:
    if field_lower == keyword:
        return True

    suspicious = (
        f"{keyword};",
        f";{keyword}",
        f"{keyword} ",
        f" {keyword}",
        f"{keyword}.",
        f".{keyword}",
        f"{keyword}-",
        f"-{keyword}",
    )
    return _contains_any_substring(field_lower, suspicious)


def _validate_field_part(part: str) -> None:
    if not part:
        raise SecurityValidationError(type="InvalidField", detail="field part cannot be empty")

    if "[" in part and "]" in part:
        open_bracket = part.find("[")
        close_bracket = part.rfind("]")
        if close_bracket <= open_bracket:
            raise SecurityValidationError(type="InvalidField", detail="invalid bracket syntax in field part")

        field_name = part[:open_bracket]
        index_part = part[open_bracket + 1 : close_bracket]
        remaining = part[close_bracket + 1 :]

        if re.match(r"^[a-zA-Z_][a-zA-Z0-9_]*$", field_name) is None:
            raise SecurityValidationError(
                type="InvalidField",
                detail=(
                    "field name part must start with letter or underscore and contain only alphanumeric characters "
                    "and underscores"
                ),
            )

        if re.match(r"^[0-9]+$", index_part) is None:
            raise SecurityValidationError(type="InvalidField", detail="list index must be a number")

        if remaining:
            raise SecurityValidationError(
                type="InvalidField", detail="unexpected characters after list index"
            )
        return

    if re.match(r"^[a-zA-Z_][a-zA-Z0-9_]*$", part) is None:
        raise SecurityValidationError(
            type="InvalidField",
            detail="field part must start with letter or underscore and contain only alphanumeric characters and underscores",
        )


def validate_operator(op: str) -> None:
    if not op:
        raise SecurityValidationError(type="InvalidOperator", detail="operator cannot be empty")
    if len(op) > MaxOperatorLength:
        raise SecurityValidationError(type="InvalidOperator", detail="operator exceeds maximum length")

    op_upper = op.strip().upper()
    if op_upper not in _ALLOWED_OPERATORS:
        raise SecurityValidationError(type="InvalidOperator", detail="operator not allowed")

    op_lower = op.lower()
    if _contains_any_substring(op_lower, _DANGEROUS_PATTERNS):
        raise SecurityValidationError(type="InjectionAttempt", detail="operator contains dangerous pattern")


def validate_value(value: Any) -> None:
    if value is None:
        return

    if isinstance(value, str):
        _validate_string_value(value)
        return

    if isinstance(value, (bytes, bytearray, memoryview)):
        return

    if isinstance(value, Sequence) and not isinstance(value, (str, bytes, bytearray, dict)):
        _validate_sequence_value(value)
        return

    if isinstance(value, Mapping):
        _validate_mapping_value(value)
        return

    if callable(value) or isinstance(value, complex):
        raise SecurityValidationError(type="InvalidValue", detail="unsupported value type")


def _validate_string_value(value: str) -> None:
    if len(value) > MaxValueStringLength:
        raise SecurityValidationError(type="InvalidValue", detail="string value exceeds maximum length")

    lower = value.lower()

    if (
        _contains_any_substring(lower, _VALUE_SCRIPT_PATTERNS)
        or ("/*" in value and "*/" in value)
        or _contains_any_substring(lower, _VALUE_SQL_INJECTION_PATTERNS)
        or _looks_like_union_select_injection(lower, value)
    ):
        raise SecurityValidationError(
            type="InjectionAttempt", detail="string value contains dangerous pattern"
        )


def _validate_sequence_value(value: Sequence[Any]) -> None:
    if len(value) > 100:
        raise SecurityValidationError(
            type="InvalidValue", detail="slice value exceeds maximum length of 100 items"
        )

    for item in value:
        try:
            validate_value(item)
        except SecurityValidationError as err:
            raise SecurityValidationError(type="InvalidValue", detail="invalid item in collection") from err


def _validate_mapping_value(value: Mapping[Any, Any]) -> None:
    if len(value) > 100:
        raise SecurityValidationError(type="InvalidValue", detail="map value exceeds maximum keys")

    for k, v in value.items():
        try:
            validate_field_name(str(k))
        except SecurityValidationError as err:
            raise SecurityValidationError(type="InvalidValue", detail="invalid map key") from err
        try:
            validate_value(v)
        except SecurityValidationError as err:
            raise SecurityValidationError(type="InvalidValue", detail="invalid map value") from err


def validate_expression(expression: str) -> None:
    if len(expression) > MaxExpressionLength:
        raise SecurityValidationError(type="InvalidExpression", detail="expression exceeds maximum length")

    lower = expression.lower()
    if _contains_any_substring(lower, _DANGEROUS_PATTERNS):
        raise SecurityValidationError(type="InjectionAttempt", detail="expression contains dangerous pattern")

    sql_injection_patterns = (
        "union select",
        "insert into",
        "update set",
        "delete from",
        "drop table",
        "alter table",
        "exec ",
        "execute ",
    )
    if _contains_any_substring(lower, sql_injection_patterns):
        raise SecurityValidationError(type="InjectionAttempt", detail="expression contains dangerous pattern")


def validate_table_name(name: str) -> None:
    if len(name) < 3 or len(name) > 255:
        raise SecurityValidationError(type="InvalidTableName", detail="table name length invalid")

    if re.match(r"^[a-zA-Z0-9_.-]+$", name) is None:
        raise SecurityValidationError(
            type="InvalidTableName", detail="table name contains invalid characters"
        )

    lower = name.lower()
    if _contains_any_substring(lower, _DANGEROUS_PATTERNS):
        raise SecurityValidationError(type="InjectionAttempt", detail="table name contains dangerous pattern")


def validate_index_name(name: str) -> None:
    if not name:
        return

    if len(name) < 3 or len(name) > 255:
        raise SecurityValidationError(type="InvalidIndexName", detail="index name length invalid")

    if re.match(r"^[a-zA-Z0-9_.-]+$", name) is None:
        raise SecurityValidationError(
            type="InvalidIndexName", detail="index name contains invalid characters"
        )


def _contains_any_substring(haystack: str, needles: Sequence[str]) -> bool:
    return any(n in haystack for n in needles)


def _looks_like_union_select_injection(lower: str, raw: str) -> bool:
    if "union" not in lower or "select" not in lower:
        return False

    if "union select" not in lower and "union all select" not in lower and "union/**/select" not in lower:
        return False

    return "from" in lower or "*" in lower or raw.endswith("--") or raw.endswith(";")
