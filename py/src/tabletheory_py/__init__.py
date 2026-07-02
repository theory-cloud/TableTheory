from __future__ import annotations

from typing import Any

import theorydb_py as _theorydb_py
from theorydb_py import *  # noqa: F401,F403
from theorydb_py import __all__ as __all__
from theorydb_py import __repo_version__ as __repo_version__
from theorydb_py import __version__ as __version__


def __getattr__(name: str) -> Any:
    return getattr(_theorydb_py, name)
