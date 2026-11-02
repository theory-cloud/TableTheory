"""Item writer-path preflight for the Python runtime.

Unlike Go, the Python runtime serializes every whole item through one method
(``Table._to_item``): ``put``, ``save``, ``batch_write`` puts and
``transact_write`` puts all call it, so both writer paths return the same
representation.
"""

from __future__ import annotations

from copy import copy
from enum import StrEnum
from typing import TYPE_CHECKING, Any

from .errors import ValidationError

if TYPE_CHECKING:
    from .table import Table

__all__ = ["ItemWriterPath", "marshal_item_for_writer_path"]


class ItemWriterPath(StrEnum):
    """A TableTheory writer path whose stored representation a preflight reproduces.

    The two members exist so cross-runtime preflight code and the shared
    nil/empty conformance table name writer paths identically on every runtime.
    Each writer path's stored representation is a fixed compatibility contract,
    frozen by the operator's policy that TableTheory supports its consumers
    rather than changing stored data.
    """

    PUT = "put"
    """The put family: ``put``, ``save`` and ``batch_write`` puts.

    These store the item serializer's representation. In it, a ``None`` value
    written without ``omitempty`` becomes DynamoDB ``NULL``, and an empty list or
    dict becomes an empty ``L`` or ``M``.
    """

    TRANSACT_WRITE = "transact_write"
    """The transactional family: ``transact_write`` puts.

    These call the same item serializer as :attr:`PUT`, so they store the same
    representation.
    """


def marshal_item_for_writer_path[T](
    table: Table[T],
    item: T,
    path: ItemWriterPath,
    *,
    now: str | None = None,
) -> dict[str, Any]:
    """Return the DynamoDB item that the named writer path stores for ``item``.

    The returned mapping is byte-equal to the payload the real writer sends.
    :attr:`ItemWriterPath.PUT` matches a ``put``, ``save`` or ``batch_write``
    capture, and :attr:`ItemWriterPath.TRANSACT_WRITE` matches a
    ``transact_write`` put capture; both call the same item serializer this
    function delegates to. A caller can therefore measure the attributes it is
    about to write instead of predicting them from the model.

    ``table`` supplies the model definition and the serializer, so ``item`` is
    the same dataclass instance ``put`` would receive. Both writer paths are
    pinned by regression tests that capture live payloads through the stateful
    fake client, so neither answer can drift from its writer without failing the
    build.

    ``now`` is the timestamp the writer will stamp when the model declares
    ``created_at`` or ``updated_at`` fields. Both writer paths stamp those from
    the table's clock, so a preflight without that clock cannot support the
    equality claim above; the model is refused instead of being stamped from a
    different clock read. Pass the same value the writer's clock produces.
    Models with no lifecycle field do not need it.

    Scope:

    - Models that declare ``encrypted`` attributes are refused. An encrypted
      attribute is stored as a KMS-wrapped envelope whose bytes depend on key
      material and envelope randomness, so no static preflight can predict them.
    - The preflight reproduces attribute serialization with the boto3 type
      serializer the table uses. Serialization is not identical across boto3
      versions for some edge inputs.

    An unrecognized path is refused. The preflight has no side effects and never
    contacts DynamoDB.
    """
    if path not in (ItemWriterPath.PUT, ItemWriterPath.TRANSACT_WRITE):
        raise ValidationError(f"unknown item writer path: {path!r}")

    if any(attr.encrypted for attr in table._model.attributes.values()):
        raise ValidationError(
            f"item preflight does not cover the encrypted attributes of {table._table_name}"
        )

    if _has_lifecycle_fields(table):
        if now is None:
            raise ValidationError(
                f"item preflight for {table._table_name} needs the writer's lifecycle clock"
            )
        table = _clock_pinned_table(table, now)

    return table._to_item(item)


def _has_lifecycle_fields(table: Table[Any]) -> bool:
    """Report whether a writer stamps a lifecycle field from the table clock."""

    return any(
        "created_at" in attr.roles or "updated_at" in attr.roles for attr in table._model.attributes.values()
    )


def _clock_pinned_table[T](table: Table[T], now: str) -> Table[T]:
    """Return a shallow clone whose clock returns ``now`` for every read.

    The clone shares the model, client and serializer with ``table``; only the
    clock is replaced, so the preflight reads the caller's fixed stamp instead of
    consuming a tick from the table's own clock.
    """

    pinned = copy(table)
    pinned._now = lambda: now
    return pinned
