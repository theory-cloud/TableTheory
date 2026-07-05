from __future__ import annotations

from importlib import import_module
from typing import Any


def reexport(module_name: str, namespace: dict[str, Any], names: tuple[str, ...]) -> list[str]:
    """Populate a legacy shim module with explicit public names from its canonical module."""
    module = import_module(module_name)
    for name in names:
        namespace[name] = getattr(module, name)
    return list(names)
