from __future__ import annotations

from dataclasses import dataclass
from typing import Literal

type QueryOperation = Literal["Query", "Scan"]
type IndexType = Literal["TABLE", "GSI", "LSI"]


@dataclass(frozen=True)
class QueryPlan:
    id: str
    operation: QueryOperation
    index_name: str | None = None
    projections: tuple[str, ...] = ()
    parallel_segments: int | None = None
    optimization_hints: tuple[str, ...] = ()


@dataclass(frozen=True)
class QueryShape:
    kind: Literal["query"]
    model_name: str
    table_name: str
    index_name: str | None
    index_type: IndexType
    has_partition_key: bool
    has_sort_key: bool
    has_sort_condition: bool
    has_filter: bool
    projections: tuple[str, ...]
    consistent_read: bool
    scan_forward: bool


@dataclass(frozen=True)
class ScanShape:
    kind: Literal["scan"]
    model_name: str
    table_name: str
    index_name: str | None
    index_type: IndexType
    has_filter: bool
    projections: tuple[str, ...]
    consistent_read: bool
    parallel_scan_configured: bool
    total_segments: int | None


class QueryOptimizer:
    def __init__(self, *, enable_parallel: bool = True, max_parallelism: int = 4) -> None:
        self._enable_parallel = bool(enable_parallel)
        self._max_parallelism = max(1, int(max_parallelism))

    def explain(self, shape: QueryShape | ScanShape) -> QueryPlan:
        projections = tuple(sorted(shape.projections))
        hints: list[str] = []

        if shape.index_type == "GSI" and shape.consistent_read:
            hints.append("ERROR: Consistent reads are not supported on GSIs")

        if isinstance(shape, QueryShape):
            if not shape.has_partition_key:
                hints.append("ERROR: partition key is not set (query will fail)")
            if shape.has_sort_key and not shape.has_sort_condition:
                hints.append("TIP: Add a sort key condition for more efficient queries when possible")
            if shape.has_filter:
                hints.append("INFO: Filters are applied after retrieval; prefer key conditions when possible")
            if not projections:
                hints.append("TIP: Use projection to select only needed attributes and reduce transfer")

            return QueryPlan(
                id=_plan_id(shape, projections),
                operation="Query",
                index_name=shape.index_name,
                projections=projections,
                optimization_hints=tuple(hints),
            )

        # Scan
        hints.append("WARNING: Scan reads the full table/index; prefer Query when possible")
        if shape.has_filter:
            hints.append("INFO: Filters are applied after retrieval; consider narrowing with keys or indexes")
        if not projections:
            hints.append("TIP: Use projection to select only needed attributes and reduce transfer")

        suggested_segments: int | None = None
        if self._enable_parallel and not shape.parallel_scan_configured:
            suggested_segments = max(1, min(self._max_parallelism, 16))
            if suggested_segments > 1:
                hints.append(f"TIP: Use parallel scan with {suggested_segments} segments for large scans")

        return QueryPlan(
            id=_plan_id(shape, projections),
            operation="Scan",
            index_name=shape.index_name,
            projections=projections,
            parallel_segments=suggested_segments,
            optimization_hints=tuple(hints),
        )


def _plan_id(shape: QueryShape | ScanShape, projections: tuple[str, ...]) -> str:
    parts = [
        shape.kind,
        shape.model_name,
        shape.table_name,
        f"idx={shape.index_name or ''}",
        f"it={shape.index_type}",
        f"proj={','.join(projections)}",
        f"filter={'1' if shape.has_filter else '0'}",
        f"cr={'1' if shape.consistent_read else '0'}",
    ]

    if isinstance(shape, QueryShape):
        parts.extend(
            [
                f"pk={'1' if shape.has_partition_key else '0'}",
                f"sk={'1' if shape.has_sort_key else '0'}",
                f"skc={'1' if shape.has_sort_condition else '0'}",
                f"sf={'1' if shape.scan_forward else '0'}",
            ]
        )
    else:
        parts.extend(
            [
                f"psc={'1' if shape.parallel_scan_configured else '0'}",
                f"ts={shape.total_segments or ''}",
            ]
        )

    return "|".join(parts)
