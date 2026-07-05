from tabletheory_py.protocols import DynamoDBClientProtocol


class _ProtocolStub(DynamoDBClientProtocol):
    pass


def test_dynamodb_client_protocol_methods_are_typing_only() -> None:
    stub = _ProtocolStub()
    for method_name in (
        "query",
        "scan",
        "batch_get_item",
        "batch_write_item",
        "transact_get_items",
        "transact_write_items",
        "put_item",
        "get_item",
        "delete_item",
        "update_item",
    ):
        assert getattr(stub, method_name)() is None
