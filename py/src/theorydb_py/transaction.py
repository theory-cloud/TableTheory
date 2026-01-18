from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass
from typing import Any


@dataclass(frozen=True)
class TransactPut[T]:
    item: T
    condition_expression: str | None = None
    expression_attribute_names: Mapping[str, str] | None = None
    expression_attribute_values: Mapping[str, Any] | None = None


@dataclass(frozen=True)
class TransactDelete:
    pk: Any
    sk: Any | None = None
    condition_expression: str | None = None
    expression_attribute_names: Mapping[str, str] | None = None
    expression_attribute_values: Mapping[str, Any] | None = None


@dataclass(frozen=True)
class TransactUpdate:
    pk: Any
    sk: Any | None
    updates: Mapping[str, Any]
    condition_expression: str | None = None
    expression_attribute_names: Mapping[str, str] | None = None
    expression_attribute_values: Mapping[str, Any] | None = None


@dataclass(frozen=True)
class TransactConditionCheck:
    pk: Any
    sk: Any | None
    condition_expression: str
    expression_attribute_names: Mapping[str, str] | None = None
    expression_attribute_values: Mapping[str, Any] | None = None


type TransactWriteAction[T] = TransactPut[T] | TransactDelete | TransactUpdate | TransactConditionCheck
