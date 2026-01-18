from __future__ import annotations

import pytest

from theorydb_py.validation import (
    MaxExpressionLength,
    MaxFieldNameLength,
    MaxNestedDepth,
    MaxOperatorLength,
    MaxValueStringLength,
    SecurityValidationError,
    validate_expression,
    validate_field_name,
    validate_index_name,
    validate_operator,
    validate_table_name,
    validate_value,
)


def test_security_validation_error_str_is_sanitized() -> None:
    err = SecurityValidationError(type="InvalidField", detail="detail")
    assert str(err) == "security validation failed: InvalidField"
    assert err.type == "InvalidField"
    assert err.detail == "detail"


def test_validate_field_name_valid_values() -> None:
    for name in [
        "UserID",
        "user_id",
        "_internal",
        "Name",
        "nested.field",
        "deeply.nested.field.name",
        "listField[0]",
    ]:
        validate_field_name(name)


def test_validate_field_name_rejects_empty() -> None:
    with pytest.raises(SecurityValidationError) as exc:
        validate_field_name("")
    assert exc.value.type == "InvalidField"


def test_validate_field_name_rejects_too_long() -> None:
    with pytest.raises(SecurityValidationError) as exc:
        validate_field_name("a" * (MaxFieldNameLength + 1))
    assert exc.value.type == "InvalidField"


def test_validate_field_name_rejects_too_deep() -> None:
    deep = ".".join(["a"] * (MaxNestedDepth + 1))
    with pytest.raises(SecurityValidationError) as exc:
        validate_field_name(deep)
    assert exc.value.type == "InvalidField"


def test_validate_field_name_rejects_injection_and_does_not_leak() -> None:
    bad = [
        "field'; DROP TABLE users; --",
        'field"; DELETE FROM table; --',
        "field/*comment*/",
        "field UNION SELECT",
        "field<script>alert('xss')</script>",
    ]
    for name in bad:
        with pytest.raises(SecurityValidationError) as exc:
            validate_field_name(name)
        assert exc.value.type == "InjectionAttempt"
        assert "DROP TABLE" not in str(exc.value)
        assert "<script" not in str(exc.value)


def test_validate_field_name_rejects_control_chars() -> None:
    with pytest.raises(SecurityValidationError) as exc:
        validate_field_name("ok\0bad")
    assert exc.value.type == "InvalidField"


def test_validate_operator() -> None:
    for op in ["=", "!=", "<>", "<", "<=", ">", ">=", "between", "IN"]:
        validate_operator(op)

    with pytest.raises(SecurityValidationError) as exc:
        validate_operator("")
    assert exc.value.type == "InvalidOperator"

    with pytest.raises(SecurityValidationError) as exc:
        validate_operator("X" * (MaxOperatorLength + 1))
    assert exc.value.type == "InvalidOperator"

    with pytest.raises(SecurityValidationError) as exc:
        validate_operator("INVALID_OP")
    assert exc.value.type == "InvalidOperator"


def test_validate_value() -> None:
    validate_value(None)
    validate_value("hello")
    validate_value({"a": 1, "b": "ok", "c": True})
    validate_value([1, 2, 3])

    with pytest.raises(SecurityValidationError) as exc:
        validate_value("a" * (MaxValueStringLength + 1))
    assert exc.value.type == "InvalidValue"

    with pytest.raises(SecurityValidationError) as exc:
        validate_value("<script>alert('x')</script>")
    assert exc.value.type == "InjectionAttempt"
    assert "<script" not in str(exc.value)

    with pytest.raises(SecurityValidationError) as exc:
        validate_value([1] * 101)
    assert exc.value.type == "InvalidValue"


def test_validate_expression() -> None:
    validate_expression("attribute_exists(#a) AND #b = :b")

    with pytest.raises(SecurityValidationError) as exc:
        validate_expression("a" * (MaxExpressionLength + 1))
    assert exc.value.type == "InvalidExpression"

    with pytest.raises(SecurityValidationError) as exc:
        validate_expression("name = 1; DROP TABLE users; --")
    assert exc.value.type == "InjectionAttempt"
    assert "DROP TABLE" not in str(exc.value)


def test_validate_table_and_index_names() -> None:
    validate_table_name("users_table")
    validate_table_name("users-table")
    validate_table_name("users.table")

    with pytest.raises(SecurityValidationError) as exc:
        validate_table_name("bad name")
    assert exc.value.type == "InvalidTableName"

    with pytest.raises(SecurityValidationError) as exc:
        validate_table_name("users;drop")
    assert exc.value.type == "InvalidTableName"

    validate_index_name("")
    validate_index_name("gsi-email")

    with pytest.raises(SecurityValidationError) as exc:
        validate_index_name("bad name")
    assert exc.value.type == "InvalidIndexName"
