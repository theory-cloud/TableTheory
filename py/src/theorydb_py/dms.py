from __future__ import annotations

from ._compat import reexport as _reexport

__all__ = _reexport(
    "tabletheory_py.dms",
    globals(),
    (
        "assert_model_definition_equivalent_to_dms",
        "assert_models_equivalent",
        "get_dms_model",
        "model_definition_to_dms_model",
        "parse_dms_document",
    ),
)
