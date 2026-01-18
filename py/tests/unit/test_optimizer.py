from __future__ import annotations

from dataclasses import dataclass

from theorydb_py.mocks import FakeDynamoDBClient
from theorydb_py.model import ModelDefinition, gsi, lsi, theorydb_field
from theorydb_py.optimizer import QueryOptimizer
from theorydb_py.query import FilterCondition
from theorydb_py.table import Table


@dataclass
class User:
    pk: str = theorydb_field(name="PK", roles=["pk"])
    sk: str = theorydb_field(name="SK", roles=["sk"])
    sk2: str = theorydb_field(name="SK2")
    email_hash: str = theorydb_field(name="emailHash")


def test_optimizer_explain_is_deterministic_and_index_type_aware() -> None:
    model = ModelDefinition.from_dataclass(
        User,
        table_name="users",
        indexes=[gsi("gsi-email", partition="email_hash"), lsi("lsi-sk2", sort="sk2")],
    )
    table: Table[User] = Table(model, client=FakeDynamoDBClient())

    optimizer = QueryOptimizer()

    gsi_shape = table.describe_query(partition="A", index_name="gsi-email", consistent_read=True)
    gsi_plan = optimizer.explain(gsi_shape)
    assert any("Consistent reads are not supported on GSIs" in h for h in gsi_plan.optimization_hints)

    lsi_shape = table.describe_query(partition="A", index_name="lsi-sk2", consistent_read=True)
    lsi_plan = optimizer.explain(lsi_shape)
    assert not any("Consistent reads are not supported on GSIs" in h for h in lsi_plan.optimization_hints)

    again = optimizer.explain(gsi_shape)
    assert gsi_plan.id == again.id

    missing = table.describe_query(
        partition=None,
        filter=FilterCondition.eq("email_hash", "x"),
    )
    missing_plan = optimizer.explain(missing)
    assert any("partition key is not set" in h for h in missing_plan.optimization_hints)
    assert any("Filters are applied after retrieval" in h for h in missing_plan.optimization_hints)


def test_optimizer_suggests_parallel_scan_when_not_configured() -> None:
    model = ModelDefinition.from_dataclass(User, table_name="users")
    table: Table[User] = Table(model, client=FakeDynamoDBClient())

    optimizer = QueryOptimizer(max_parallelism=3)

    shape = table.describe_scan(filter=FilterCondition.eq("email_hash", "x"), consistent_read=False)
    plan = optimizer.explain(shape)
    assert plan.operation == "Scan"
    assert plan.parallel_segments == 3
    assert any("parallel scan with 3 segments" in h for h in plan.optimization_hints)

    configured = table.describe_scan(segment=0, total_segments=2)
    plan2 = optimizer.explain(configured)
    assert plan2.parallel_segments is None
