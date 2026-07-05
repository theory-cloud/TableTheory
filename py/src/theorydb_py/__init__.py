from __future__ import annotations

import warnings
from importlib import import_module
from typing import Any

_tabletheory_py = import_module("tabletheory_py")
__all__ = list(_tabletheory_py.__all__)
__repo_version__ = str(_tabletheory_py.__repo_version__)
__version__ = str(_tabletheory_py.__version__)
del _tabletheory_py

warnings.warn(
    "theorydb_py is deprecated and will be removed after the v2 transition; import tabletheory_py instead.",
    DeprecationWarning,
    stacklevel=2,
)


def __getattr__(name: str) -> Any:
    return getattr(import_module("tabletheory_py"), name)


def __dir__() -> list[str]:
    return sorted(set(globals()) | set(__all__))
