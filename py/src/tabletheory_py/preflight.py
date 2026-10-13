"""Item writer-path preflight surface for the Python runtime.

The public entry point is :meth:`tabletheory_py.table.Table.marshal_item_for_writer_path`.
This module defines the closed writer-path selector it takes.
"""

from enum import StrEnum

__all__ = ["ItemWriterPath"]


class ItemWriterPath(StrEnum):
    """A TableTheory writer path whose stored representation a preflight reproduces.

    Unlike Go, the Python runtime serializes every whole item through one
    method (``Table._to_item``): ``put``, ``save``, ``batch_write`` puts and
    ``transact_write`` puts all call it, so both paths currently return the same
    representation. The two members exist so cross-runtime preflight code and
    the shared nil/empty conformance table name writer paths identically on
    every runtime, and so a future divergence stays expressible without an API
    change.
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
    representation today.
    """
