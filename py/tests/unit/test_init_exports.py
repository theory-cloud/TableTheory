from __future__ import annotations

import theorydb_py as theorydb


def test_init_exposes_lazy_exports_via_getattr() -> None:
    assert theorydb._normalize_repo_version("1.2.3") == "1.2.3"

    assert callable(theorydb.ensure_table)
    assert callable(theorydb.aggregate_field)
    assert callable(theorydb.QueryOptimizer)
    assert callable(theorydb.is_lambda_environment)
    assert callable(theorydb.MultiAccountSessions)
    assert callable(theorydb.SimpleLimiter)
    assert callable(theorydb.validate_expression)
