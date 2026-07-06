from __future__ import annotations

from tabletheory_py import (
    ImmutableModelMutationError,
    ProtectedFieldMutationError,
    RejectedDeployAuthorityEvidenceError,
    TheorydbPyError,
)


def test_release_state_errors_are_public_theorydb_errors() -> None:
    for err_type in (
        ImmutableModelMutationError,
        ProtectedFieldMutationError,
        RejectedDeployAuthorityEvidenceError,
    ):
        err = err_type(err_type.__name__)
        assert isinstance(err, TheorydbPyError)
        assert err_type.__name__ in str(err)
