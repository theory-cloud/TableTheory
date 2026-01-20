from __future__ import annotations

from botocore.exceptions import ClientError

from theorydb_py.errors import LeaseHeldError, LeaseNotOwnedError
from theorydb_py.lease import Lease, LeaseKey, LeaseManager


class _StubClient:
    def __init__(self) -> None:
        self.calls: list[tuple[str, dict]] = []
        self._handlers: dict[str, object] = {}

    def on(self, op: str, handler: object) -> None:
        self._handlers[op] = handler

    def put_item(self, **req):
        self.calls.append(("put_item", req))
        handler = self._handlers.get("put_item")
        if callable(handler):
            return handler(req)
        return {}

    def update_item(self, **req):
        self.calls.append(("update_item", req))
        handler = self._handlers.get("update_item")
        if callable(handler):
            return handler(req)
        return {}

    def delete_item(self, **req):
        self.calls.append(("delete_item", req))
        handler = self._handlers.get("delete_item")
        if callable(handler):
            return handler(req)
        return {}


def test_lease_manager_acquire_builds_conditional_put() -> None:
    client = _StubClient()
    mgr = LeaseManager(
        client=client,
        table_name="tbl",
        now=lambda: 1000.0,
        token=lambda: "tok",
        ttl_buffer_seconds=10,
    )

    lease = mgr.acquire(LeaseKey(pk="CACHE#A", sk="LOCK"), lease_seconds=30)
    assert lease == Lease(key=LeaseKey(pk="CACHE#A", sk="LOCK"), token="tok", expires_at=1030)

    op, req = client.calls[0]
    assert op == "put_item"
    assert req["TableName"] == "tbl"
    assert req["ConditionExpression"] == "attribute_not_exists(#pk) OR #exp <= :now"
    assert req["ExpressionAttributeNames"] == {"#pk": "pk", "#exp": "lease_expires_at"}
    assert req["ExpressionAttributeValues"] == {":now": {"N": "1000"}}
    assert req["Item"]["pk"] == {"S": "CACHE#A"}
    assert req["Item"]["sk"] == {"S": "LOCK"}
    assert req["Item"]["lease_token"] == {"S": "tok"}
    assert req["Item"]["lease_expires_at"] == {"N": "1030"}
    assert req["Item"]["ttl"] == {"N": "1040"}


def test_lease_manager_acquire_raises_lease_held() -> None:
    client = _StubClient()
    err = ClientError(
        {"Error": {"Code": "ConditionalCheckFailedException", "Message": "no"}},
        "PutItem",
    )

    def handler(_: dict) -> object:
        raise err

    client.on("put_item", handler)
    mgr = LeaseManager(client=client, table_name="tbl", now=lambda: 1000.0, token=lambda: "tok")

    try:
        mgr.acquire(LeaseKey(pk="CACHE#A", sk="LOCK"), lease_seconds=30)
        raise AssertionError("expected LeaseHeldError")
    except LeaseHeldError:
        pass


def test_lease_manager_refresh_updates_when_owned() -> None:
    client = _StubClient()
    mgr = LeaseManager(client=client, table_name="tbl", now=lambda: 1000.0, ttl_buffer_seconds=10)

    out = mgr.refresh(
        Lease(key=LeaseKey(pk="CACHE#A", sk="LOCK"), token="tok", expires_at=0),
        lease_seconds=60,
    )
    assert out.expires_at == 1060

    op, req = client.calls[0]
    assert op == "update_item"
    assert req["TableName"] == "tbl"
    assert req["UpdateExpression"] == "SET #exp = :exp, #ttl = :ttl"
    assert req["ConditionExpression"] == "#tok = :tok AND #exp > :now"
    assert req["ExpressionAttributeNames"]["#tok"] == "lease_token"
    assert req["ExpressionAttributeNames"]["#exp"] == "lease_expires_at"
    assert req["ExpressionAttributeNames"]["#ttl"] == "ttl"
    assert req["ExpressionAttributeValues"][":tok"] == {"S": "tok"}
    assert req["ExpressionAttributeValues"][":now"] == {"N": "1000"}
    assert req["ExpressionAttributeValues"][":exp"] == {"N": "1060"}
    assert req["ExpressionAttributeValues"][":ttl"] == {"N": "1070"}


def test_lease_manager_refresh_raises_not_owned() -> None:
    client = _StubClient()
    err = ClientError(
        {"Error": {"Code": "ConditionalCheckFailedException", "Message": "no"}},
        "UpdateItem",
    )

    def handler(_: dict) -> object:
        raise err

    client.on("update_item", handler)
    mgr = LeaseManager(client=client, table_name="tbl", now=lambda: 1000.0)

    try:
        mgr.refresh(
            Lease(key=LeaseKey(pk="CACHE#A", sk="LOCK"), token="tok", expires_at=0),
            lease_seconds=60,
        )
        raise AssertionError("expected LeaseNotOwnedError")
    except LeaseNotOwnedError:
        pass


def test_lease_manager_release_is_best_effort() -> None:
    client = _StubClient()
    err = ClientError(
        {"Error": {"Code": "ConditionalCheckFailedException", "Message": "no"}},
        "DeleteItem",
    )

    def handler(_: dict) -> object:
        raise err

    client.on("delete_item", handler)
    mgr = LeaseManager(client=client, table_name="tbl", now=lambda: 1000.0)

    mgr.release(Lease(key=LeaseKey(pk="CACHE#A", sk="LOCK"), token="tok", expires_at=0))

