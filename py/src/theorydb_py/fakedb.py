from __future__ import annotations

from copy import deepcopy
from dataclasses import dataclass, field
from decimal import Decimal
from typing import Any

from botocore.exceptions import ClientError

Item = dict[str, Any]


@dataclass
class _TableState:
    pk: str = "PK"
    sk: str | None = "SK"
    items: dict[str, Item] = field(default_factory=dict)
    ttl_attr: str | None = None


class StatefulDynamoDBClient:
    """State-backed in-memory DynamoDB client for TableTheory tests.

    This is a deterministic local testing aid, not a DynamoDB emulator. Its
    supported behavior is bounded to the TableTheory fake/testkit scenarios:
    key-based writes/reads, basic query/scan filters, optimistic-lock
    conditions, batches, transactions, and TTL attribute persistence.
    """

    def __init__(self) -> None:
        self._tables: dict[str, _TableState] = {}
        self.calls: list[tuple[str, dict[str, Any]]] = []

    def reset(self) -> None:
        self._tables.clear()
        self.calls.clear()

    def seed(self, table_name: str, *items: Item) -> None:
        table = self._table(table_name)
        for item in items:
            table.items[_item_key(table, item)] = deepcopy(item)

    def items(self, table_name: str) -> list[Item]:
        table = self._tables.get(table_name)
        if table is None:
            return []
        return [deepcopy(item) for _, item in sorted(table.items.items())]

    def create_table(self, **kwargs: Any) -> dict[str, Any]:
        self._record("create_table", kwargs)
        table_name = str(kwargs.get("TableName") or "default")
        if table_name in self._tables:
            raise _client_error("ResourceInUseException", f"table already exists: {table_name}")
        state = _TableState()
        for key in kwargs.get("KeySchema", []):
            if key.get("KeyType") == "HASH":
                state.pk = str(key.get("AttributeName"))
            if key.get("KeyType") == "RANGE":
                state.sk = str(key.get("AttributeName"))
        self._tables[table_name] = state
        return {"TableDescription": _table_description(table_name, state)}

    def describe_table(self, **kwargs: Any) -> dict[str, Any]:
        self._record("describe_table", kwargs)
        table_name = str(kwargs.get("TableName") or "default")
        table = self._tables.get(table_name)
        if table is None:
            raise _client_error("ResourceNotFoundException", f"table not found: {table_name}")
        return {"Table": _table_description(table_name, table)}

    def delete_table(self, **kwargs: Any) -> dict[str, Any]:
        self._record("delete_table", kwargs)
        table_name = str(kwargs.get("TableName") or "default")
        table = self._tables.get(table_name)
        if table is None:
            raise _client_error("ResourceNotFoundException", f"table not found: {table_name}")
        del self._tables[table_name]
        return {"TableDescription": _table_description(table_name, table)}

    def update_time_to_live(self, **kwargs: Any) -> dict[str, Any]:
        self._record("update_time_to_live", kwargs)
        table_name = str(kwargs.get("TableName") or "default")
        table = self._tables.get(table_name)
        if table is None:
            raise _client_error("ResourceNotFoundException", f"table not found: {table_name}")
        spec = dict(kwargs.get("TimeToLiveSpecification") or {})
        attr = spec.get("AttributeName")
        if attr:
            table.ttl_attr = str(attr)
        return {"TimeToLiveSpecification": spec}

    def put_item(self, **kwargs: Any) -> dict[str, Any]:
        self._record("put_item", kwargs)
        table = self._table(str(kwargs.get("TableName") or "default"))
        item = deepcopy(dict(kwargs.get("Item") or {}))
        key = _item_key(table, item)
        existing = table.items.get(key)
        if not _matches(
            kwargs.get("ConditionExpression"),
            existing,
            kwargs.get("ExpressionAttributeNames") or {},
            kwargs.get("ExpressionAttributeValues") or {},
        ):
            raise _client_error("ConditionalCheckFailedException", "conditional request failed")
        table.items[key] = item
        return {}

    def get_item(self, **kwargs: Any) -> dict[str, Any]:
        self._record("get_item", kwargs)
        table = self._table(str(kwargs.get("TableName") or "default"))
        item = table.items.get(_key_from_map(table, dict(kwargs.get("Key") or {})))
        if item is None:
            return {}
        return {
            "Item": _project(
                item,
                kwargs.get("ProjectionExpression"),
                kwargs.get("ExpressionAttributeNames") or {},
            )
        }

    def update_item(self, **kwargs: Any) -> dict[str, Any]:
        self._record("update_item", kwargs)
        table = self._table(str(kwargs.get("TableName") or "default"))
        key = _key_from_map(table, dict(kwargs.get("Key") or {}))
        current = table.items.get(key)
        if not _matches(
            kwargs.get("ConditionExpression"),
            current,
            kwargs.get("ExpressionAttributeNames") or {},
            kwargs.get("ExpressionAttributeValues") or {},
        ):
            raise _client_error("ConditionalCheckFailedException", "conditional request failed")
        item = deepcopy(current or kwargs.get("Key") or {})
        _apply_update(
            item,
            str(kwargs.get("UpdateExpression") or ""),
            kwargs.get("ExpressionAttributeNames") or {},
            kwargs.get("ExpressionAttributeValues") or {},
        )
        table.items[key] = item
        if kwargs.get("ReturnValues") in {"ALL_NEW", "UPDATED_NEW"}:
            return {"Attributes": deepcopy(item)}
        return {}

    def delete_item(self, **kwargs: Any) -> dict[str, Any]:
        self._record("delete_item", kwargs)
        table = self._table(str(kwargs.get("TableName") or "default"))
        key = _key_from_map(table, dict(kwargs.get("Key") or {}))
        current = table.items.get(key)
        if not _matches(
            kwargs.get("ConditionExpression"),
            current,
            kwargs.get("ExpressionAttributeNames") or {},
            kwargs.get("ExpressionAttributeValues") or {},
        ):
            raise _client_error("ConditionalCheckFailedException", "conditional request failed")
        table.items.pop(key, None)
        return {}

    def query(self, **kwargs: Any) -> dict[str, Any]:
        self._record("query", kwargs)
        table = self._table(str(kwargs.get("TableName") or "default"))
        return _read_response(table, list(table.items.values()), kwargs, query=True)

    def scan(self, **kwargs: Any) -> dict[str, Any]:
        self._record("scan", kwargs)
        table = self._table(str(kwargs.get("TableName") or "default"))
        return _read_response(table, list(table.items.values()), kwargs, query=False)

    def batch_get_item(self, **kwargs: Any) -> dict[str, Any]:
        self._record("batch_get_item", kwargs)
        responses: dict[str, list[Item]] = {}
        for table_name, req in (kwargs.get("RequestItems") or {}).items():
            table = self._table(str(table_name))
            found: list[Item] = []
            for key in req.get("Keys", []):
                item = table.items.get(_key_from_map(table, key))
                if item is not None:
                    found.append(
                        _project(
                            item, req.get("ProjectionExpression"), req.get("ExpressionAttributeNames") or {}
                        )
                    )
            responses[str(table_name)] = found
        return {"Responses": responses}

    def batch_write_item(self, **kwargs: Any) -> dict[str, Any]:
        self._record("batch_write_item", kwargs)
        for table_name, requests in (kwargs.get("RequestItems") or {}).items():
            table = self._table(str(table_name))
            for req in requests:
                if "PutRequest" in req:
                    item = deepcopy(req["PutRequest"].get("Item") or {})
                    table.items[_item_key(table, item)] = item
                if "DeleteRequest" in req:
                    table.items.pop(_key_from_map(table, req["DeleteRequest"].get("Key") or {}), None)
        return {}

    def transact_get_items(self, **kwargs: Any) -> dict[str, Any]:
        self._record("transact_get_items", kwargs)
        responses: list[dict[str, Any]] = []
        for req in kwargs.get("TransactItems") or []:
            get = req.get("Get") or {}
            table = self._table(str(get.get("TableName") or "default"))
            item = table.items.get(_key_from_map(table, get.get("Key") or {}))
            responses.append(
                {
                    "Item": _project(
                        item or {},
                        get.get("ProjectionExpression"),
                        get.get("ExpressionAttributeNames") or {},
                    )
                }
                if item is not None
                else {}
            )
        return {"Responses": responses}

    def transact_write_items(self, **kwargs: Any) -> dict[str, Any]:
        self._record("transact_write_items", kwargs)
        snapshot = deepcopy(self._tables)
        try:
            for req in kwargs.get("TransactItems") or []:
                _apply_transact(snapshot, req)
        except ClientError as err:
            raise _transaction_error(err) from err
        self._tables = snapshot
        return {}

    def _table(self, table_name: str) -> _TableState:
        table = self._tables.get(table_name)
        if table is None:
            table = _TableState()
            self._tables[table_name] = table
        return table

    def _record(self, method: str, kwargs: dict[str, Any]) -> None:
        self.calls.append((method, deepcopy(kwargs)))


def _apply_transact(tables: dict[str, _TableState], req: dict[str, Any]) -> None:
    if "Put" in req:
        put = req["Put"]
        table = tables.setdefault(str(put.get("TableName") or "default"), _TableState())
        item = deepcopy(put.get("Item") or {})
        key = _item_key(table, item)
        if not _matches(
            put.get("ConditionExpression"),
            table.items.get(key),
            put.get("ExpressionAttributeNames") or {},
            put.get("ExpressionAttributeValues") or {},
        ):
            raise _client_error("ConditionalCheckFailedException", "conditional request failed")
        table.items[key] = item
    if "Update" in req:
        update = req["Update"]
        table = tables.setdefault(str(update.get("TableName") or "default"), _TableState())
        key = _key_from_map(table, update.get("Key") or {})
        if not _matches(
            update.get("ConditionExpression"),
            table.items.get(key),
            update.get("ExpressionAttributeNames") or {},
            update.get("ExpressionAttributeValues") or {},
        ):
            raise _client_error("ConditionalCheckFailedException", "conditional request failed")
        item = deepcopy(table.items.get(key) or update.get("Key") or {})
        _apply_update(
            item,
            str(update.get("UpdateExpression") or ""),
            update.get("ExpressionAttributeNames") or {},
            update.get("ExpressionAttributeValues") or {},
        )
        table.items[key] = item
    if "Delete" in req:
        delete = req["Delete"]
        table = tables.setdefault(str(delete.get("TableName") or "default"), _TableState())
        key = _key_from_map(table, delete.get("Key") or {})
        if not _matches(
            delete.get("ConditionExpression"),
            table.items.get(key),
            delete.get("ExpressionAttributeNames") or {},
            delete.get("ExpressionAttributeValues") or {},
        ):
            raise _client_error("ConditionalCheckFailedException", "conditional request failed")
        table.items.pop(key, None)
    if "ConditionCheck" in req:
        check = req["ConditionCheck"]
        table = tables.setdefault(str(check.get("TableName") or "default"), _TableState())
        key = _key_from_map(table, check.get("Key") or {})
        if not _matches(
            check.get("ConditionExpression"),
            table.items.get(key),
            check.get("ExpressionAttributeNames") or {},
            check.get("ExpressionAttributeValues") or {},
        ):
            raise _client_error("ConditionalCheckFailedException", "conditional request failed")


def _read_response(
    table: _TableState, source: list[Item], req: dict[str, Any], *, query: bool
) -> dict[str, Any]:
    names = req.get("ExpressionAttributeNames") or {}
    values = req.get("ExpressionAttributeValues") or {}
    items = [
        deepcopy(item)
        for item in source
        if (not query or _matches(req.get("KeyConditionExpression"), item, names, values))
        and _matches(req.get("FilterExpression"), item, names, values)
    ]
    if table.sk is not None:
        sort_key = table.sk
        items.sort(key=lambda item: _key_part(item.get(sort_key)))
    if req.get("ScanIndexForward") is False:
        items.reverse()
    if req.get("ExclusiveStartKey"):
        start = _key_from_map(table, req["ExclusiveStartKey"])
        for idx, item in enumerate(items):
            if _item_key(table, item) == start:
                items = items[idx + 1 :]
                break
    scanned = len(items)
    last_key: Item | None = None
    limit = req.get("Limit")
    if isinstance(limit, int) and limit > 0 and len(items) > limit:
        last_key = _key_map(table, items[limit - 1])
        items = items[:limit]
    if req.get("Select") == "COUNT":
        out: dict[str, Any] = {"Count": scanned, "ScannedCount": scanned}
    else:
        out = {
            "Items": [
                _project(item, req.get("ProjectionExpression"), req.get("ExpressionAttributeNames") or {})
                for item in items
            ],
            "Count": len(items),
            "ScannedCount": scanned,
        }
    if last_key is not None:
        out["LastEvaluatedKey"] = last_key
    return out


def _matches(expression: Any, item: Item | None, names: dict[str, str], values: dict[str, Any]) -> bool:
    expr = _strip_parens(str(expression or "").strip())
    if not expr:
        return True
    parts = _split_logical(expr, "OR")
    if len(parts) > 1:
        return any(_matches(part, item, names, values) for part in parts)
    parts = _split_logical(expr, "AND")
    if len(parts) > 1:
        return all(_matches(part, item, names, values) for part in parts)
    if expr.startswith("attribute_not_exists("):
        return item is None or item.get(_name(expr[len("attribute_not_exists(") : -1], names)) is None
    if expr.startswith("attribute_exists("):
        return item is not None and item.get(_name(expr[len("attribute_exists(") : -1], names)) is not None
    if expr.startswith("begins_with("):
        left, right = _split_csv(expr[len("begins_with(") : -1])
        return _string_value((item or {}).get(_name(left, names))).startswith(
            _string_value(values.get(right.strip()))
        )
    for op in ("<>", ">=", "<=", "=", ">", "<"):
        marker = f" {op} "
        if marker not in expr:
            continue
        left, right = expr.split(marker, 1)
        cmp = _compare_av((item or {}).get(_name(left, names)), values.get(right.strip()))
        return {
            "=": cmp == 0,
            "<>": cmp != 0,
            ">": cmp > 0,
            ">=": cmp >= 0,
            "<": cmp < 0,
            "<=": cmp <= 0,
        }[op]
    return False


def _apply_update(item: Item, expression: str, names: dict[str, str], values: dict[str, Any]) -> None:
    for action, body in _update_sections(expression):
        if action == "SET":
            for part in _split_csv(body):
                left, right = part.split("=", 1)
                item[_name(left, names)] = deepcopy(values[right.strip()])
        if action == "ADD":
            for part in _split_csv(body):
                left, right = part.split()
                item[_name(left, names)] = _add_av(item.get(_name(left, names)), values.get(right))
        if action == "REMOVE":
            for part in _split_csv(body):
                item.pop(_name(part, names), None)


def _update_sections(expression: str) -> list[tuple[str, str]]:
    markers: list[tuple[int, str]] = []
    for action in ("SET", "ADD", "REMOVE", "DELETE"):
        idx = expression.find(action)
        if idx >= 0:
            markers.append((idx, action))
    markers.sort()
    out: list[tuple[str, str]] = []
    for idx, (start, action) in enumerate(markers):
        end = markers[idx + 1][0] if idx + 1 < len(markers) else len(expression)
        out.append((action, expression[start + len(action) : end].strip()))
    return out


def _split_logical(expr: str, op: str) -> list[str]:
    target = f" {op} "
    depth = 0
    start = 0
    out: list[str] = []
    i = 0
    while i < len(expr):
        if expr[i] == "(":
            depth += 1
        elif expr[i] == ")":
            depth -= 1
        elif depth == 0 and expr.startswith(target, i):
            out.append(expr[start:i].strip())
            i += len(target)
            start = i
            continue
        i += 1
    if not out:
        return [expr]
    out.append(expr[start:].strip())
    return out


def _split_csv(text: str) -> list[str]:
    out: list[str] = []
    depth = 0
    start = 0
    for idx, ch in enumerate(text):
        if ch == "(":
            depth += 1
        elif ch == ")":
            depth -= 1
        elif ch == "," and depth == 0:
            out.append(text[start:idx].strip())
            start = idx + 1
    tail = text[start:].strip()
    if tail:
        out.append(tail)
    return out


def _project(item: Item, expression: Any, names: dict[str, str]) -> Item:
    if not expression:
        return deepcopy(item)
    out: Item = {}
    for token in _split_csv(str(expression)):
        attr = _name(token, names)
        if attr in item:
            out[attr] = deepcopy(item[attr])
    return out


def _name(token: str, names: dict[str, str]) -> str:
    token = token.strip()
    return names.get(token, token)


def _item_key(table: _TableState, item: Item) -> str:
    if table.pk not in item:
        raise ValueError(f"missing partition key {table.pk}")
    sk = table.sk
    return _key_part(item[table.pk]) + ("|" + _key_part(item.get(sk)) if sk else "")


def _key_from_map(table: _TableState, key: Item) -> str:
    return _item_key(table, key)


def _key_map(table: _TableState, item: Item) -> Item:
    out = {table.pk: deepcopy(item[table.pk])}
    if table.sk is not None:
        out[table.sk] = deepcopy(item[table.sk])
    return out


def _key_part(av: Any) -> str:
    if isinstance(av, dict):
        if "S" in av:
            return "S:" + str(av.get("S", ""))
        if "N" in av:
            return "N:" + str(av.get("N", ""))
        if "B" in av:
            return "B:" + repr(av.get("B", b""))
    return repr(av)


def _string_value(av: Any) -> str:
    if isinstance(av, dict):
        if "S" in av:
            return str(av.get("S", ""))
        if "N" in av:
            return str(av.get("N", ""))
    return _key_part(av)


def _compare_av(left: Any, right: Any) -> int:
    if isinstance(left, dict) and isinstance(right, dict) and "N" in left and "N" in right:
        left_decimal = Decimal(str(left["N"]))
        right_decimal = Decimal(str(right["N"]))
        return (left_decimal > right_decimal) - (left_decimal < right_decimal)
    lkey = _key_part(left)
    rkey = _key_part(right)
    return (lkey > rkey) - (lkey < rkey)


def _add_av(left: Any, right: Any) -> Any:
    if isinstance(left, dict) and isinstance(right, dict) and "N" in left and "N" in right:
        total = Decimal(str(left["N"])) + Decimal(str(right["N"]))
        return {"N": str(int(total)) if total == total.to_integral() else str(total)}
    return deepcopy(right)


def _strip_parens(expr: str) -> str:
    while expr.startswith("(") and expr.endswith(")"):
        expr = expr[1:-1].strip()
    return expr


def _table_description(table_name: str, table: _TableState) -> dict[str, Any]:
    key_schema = [{"AttributeName": table.pk, "KeyType": "HASH"}]
    if table.sk is not None:
        key_schema.append({"AttributeName": table.sk, "KeyType": "RANGE"})
    return {
        "TableName": table_name,
        "TableStatus": "ACTIVE",
        "KeySchema": key_schema,
        "ItemCount": len(table.items),
    }


def _client_error(code: str, message: str) -> ClientError:
    return ClientError({"Error": {"Code": code, "Message": message}}, code)


def _transaction_error(err: ClientError) -> ClientError:
    return ClientError(
        {
            "Error": {"Code": "TransactionCanceledException", "Message": str(err)},
            "CancellationReasons": [{"Code": "ConditionalCheckFailed", "Message": str(err)}],
        },
        "TransactWriteItems",
    )
