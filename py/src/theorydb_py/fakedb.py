from __future__ import annotations

from ._compat import reexport as _reexport

__all__ = _reexport(
    "tabletheory_py.fakedb",
    globals(),
    (
        "Item",
        "StatefulDynamoDBClient",
    ),
)
