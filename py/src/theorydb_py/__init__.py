from __future__ import annotations

import warnings
from typing import Any

import tabletheory_py as _tabletheory_py
from tabletheory_py import *  # noqa: F401,F403
from tabletheory_py import __all__ as __all__
from tabletheory_py import __repo_version__ as __repo_version__
from tabletheory_py import __version__ as __version__

warnings.warn(
    "theorydb_py is deprecated and will be removed after the v2 transition; import tabletheory_py instead.",
    DeprecationWarning,
    stacklevel=2,
)


def __getattr__(name: str) -> Any:
    return getattr(_tabletheory_py, name)
