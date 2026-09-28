#!/usr/bin/env python3
"""AST detached-work scanner for Python sources (detached-work guard).

Invoked by TestPython_NoDetachedWork and the Python detector self-tests in
tests/detached_work_python_test.go, so the check runs inside `make test-unit`
and `make rubric` with the repository's own Python interpreter.

Protocol (one request/response per process):

    request  := {"files": [{"path": "<repo-relative>", "source": "<text>"}]}
    response := {"results": [{"path": ..., "findings": [
                    {"line": <1-based>, "rule": "<rule>", "text": "<source line>"}
                ], "error": null | "<message>"}]}

Proof, exactly: a launch is reported unless a join on the same target executes
on every path from the launch to every exit of the function that contains it.
The source is parsed with `ast`, and each function scope (module bodies
included) gets a control-flow graph whose nodes are statements and whose edges
are fallthrough, branches, loop back-edges, `break`/`continue` targets, and
exception transfers into `except`/`finally` clauses. `return` of the owning
function and an unhandled `raise` are exits; a nested `def` is an opaque
statement, so its body's `return` is not this function's exit and its joins are
not this function's joins.

A launch is accepted only when no path from its statement to an exit avoids
every join statement, where a join statement is one that executes the join
unconditionally whenever it executes. A join nested in an `if` branch, a loop
body, a `try` body without a joining `finally`, a nested `def`/`lambda`/
comprehension, a conditional expression (`x if c else y`), or a boolean operator
(`c and x`) is not such a statement; neither is a join on a line that opens a
compound statement (`if cond: t.join()`, `for x in xs: t.join()`). A join in a
`finally` clause is reached from every exit and so dominates. A join split
across both branches of an `if/else` is accepted, because the graph merges the
branches and every path still runs a join.

Recognized launches: `Thread`/`Timer`/`Process` `.start()`, `create_task` /
`ensure_future`, `asyncio.to_thread` / `run_in_executor`, `ThreadPoolExecutor`
`.submit(` outside a `with` block, a thread built inside a comprehension, and a
daemon thread (either `daemon=True` on the constructor or a later
`x.daemon = True`). A constructor expression in a default argument or a
decorator is evaluated where the `def`/`class` statement sits, so it becomes its
own one-statement scope and a launch there is reported.

Targets are tracked by name, attribute (`self.worker`), container element
(`pool["a"]`), and walrus binding (`(t := Thread(...))`), with simple `u = t`
aliases resolved. A plain name is tracked inside the scope that binds it. An
attribute or a container element is tracked across the whole file, because the
target belongs to the instance or container rather than to the frame that
assigned it: `self.worker` assigned in `__init__` and started in a sibling
method names the same target. The join proof stays per function, though — a
launch is reported unless a join in its own function dominates it, so such a
start with no dominating join in the method that starts it is still reported.

A task or offload is joined by `await target` (or `await asyncio.gather`/`wait`
over it); a thread by `target.join()`; `task.cancel()` is not a join, because
cancellation is cooperative and the task can still be running when the caller
returns.

Deliberately conservative, and documented as such in
docs/development-guidelines.md:

  * Only exceptions raised inside a `try` body are modeled, and they transfer to
    that statement's handlers; an exception anywhere else is not an exit. This
    mirrors the Go detector, which treats `panic` as the only escape it models.
  * A `try` whose handlers are not exhaustive keeps a transfer edge to the
    enclosing exit, so a join after such a `try` is reported even though a
    catch-all would have proven it.
  * `with` does not join its body in general; an executor `submit` inside its
    own `with` block is joined lexically, which is the one context-manager join
    the guard recognizes.
  * `await`ing the same target twice, or awaiting it in one branch and the other
    branch exiting, is reported; both branches must join.
  * An attribute or container element is tracked file-wide, so a scope that
    rebinds the same attribute to something else does not un-track it: a later
    `.start()` on that name is still reported. The broader tracking errs toward
    reporting.
  * A constructor expression in a default argument, a decorator, or a class base
    is always reported, because no function's joins can dominate it: the
    expression runs when the `def`/`class` statement executes, outside every body
    the guard proves dominance on.
"""

from __future__ import annotations

import ast
import json
import sys
from typing import Any, Callable

THREAD_CTORS = ("threading.Thread", "threading.Timer", "multiprocessing.Process", "mp.Process")
TASK_METHODS = {"create_task", "ensure_future"}
OFFLOAD_METHODS = {"to_thread", "run_in_executor"}
GATHER_METHODS = {"gather", "wait"}

# Expressions whose value is only conditionally evaluated when the enclosing
# statement executes, plus the compound statements whose own node must never be
# treated as a join. A join inside one of these is not a dominating join.
_CONDITIONAL: tuple[type, ...] = (
    ast.IfExp,
    ast.BoolOp,
    ast.Lambda,
    ast.ListComp,
    ast.SetComp,
    ast.DictComp,
    ast.GeneratorExp,
    ast.If,
    ast.While,
    ast.For,
    ast.AsyncFor,
    ast.With,
    ast.AsyncWith,
    ast.Try,
    ast.Match,
    ast.FunctionDef,
    ast.AsyncFunctionDef,
    ast.ClassDef,
)
_TRYSTAR = getattr(ast, "TryStar", None)
if _TRYSTAR is not None:
    _CONDITIONAL = _CONDITIONAL + (_TRYSTAR,)

_COMPOUND = (
    ast.If,
    ast.While,
    ast.For,
    ast.AsyncFor,
    ast.With,
    ast.AsyncWith,
    ast.Try,
    ast.Match,
    ast.FunctionDef,
    ast.AsyncFunctionDef,
    ast.ClassDef,
) + ((_TRYSTAR,) if _TRYSTAR is not None else ())


# ---------------------------------------------------------------------------
# Small textual helpers
# ---------------------------------------------------------------------------


def dotted(expr: ast.AST) -> str | None:
    """Return the dotted name of a Name/Attribute chain, or None."""
    parts: list[str] = []
    cur = expr
    while isinstance(cur, ast.Attribute):
        parts.append(cur.attr)
        cur = cur.value
    if isinstance(cur, ast.Name):
        parts.append(cur.id)
        return ".".join(reversed(parts))
    return None


def key_of(expr: ast.AST) -> str:
    """A stable textual key for an assignable expression."""
    try:
        return ast.unparse(expr)
    except Exception:  # pragma: no cover - unparse is total for parsed input
        return ""


def receiver_key(expr: ast.AST) -> str:
    """The key a `.start()`/`.join()` receiver names.

    `(t := Thread(...)).start()` names the same target as `t.start()`, so the
    walrus wrapper is unwrapped before the key is built.
    """
    if isinstance(expr, ast.NamedExpr):
        return key_of(expr.target)
    return key_of(expr)


def call_short(call: ast.AST) -> str | None:
    if not isinstance(call, ast.Call):
        return None
    name = dotted(call.func)
    if name is None:
        return None
    return name.split(".")[-1]


def is_thread_ctor(call: ast.AST, thread_names: set[str]) -> bool:
    if not isinstance(call, ast.Call):
        return False
    if dotted(call.func) in THREAD_CTORS:
        return True
    return isinstance(call.func, ast.Name) and call.func.id in thread_names


def is_task_ctor(call: ast.AST) -> bool:
    return call_short(call) in TASK_METHODS


def is_offload_ctor(call: ast.AST) -> bool:
    return call_short(call) in OFFLOAD_METHODS


def is_executor_ctor(expr: ast.AST) -> bool:
    if not isinstance(expr, ast.Call):
        return False
    name = dotted(expr.func)
    return name is not None and name.split(".")[-1].endswith("Executor")


def construct_kind(value: ast.AST, thread_names: set[str]) -> str | None:
    """The launch kind a constructor expression creates, or None."""
    if is_thread_ctor(value, thread_names):
        return "thread"
    if is_task_ctor(value):
        return "task"
    if is_offload_ctor(value):
        return "offload"
    return None


def thread_names_from_imports(tree: ast.Module) -> set[str]:
    names: set[str] = set()
    for node in ast.walk(tree):
        if isinstance(node, ast.ImportFrom) and node.module in ("threading", "multiprocessing"):
            for alias in node.names:
                names.add(alias.asname or alias.name)
    return names


def is_true_literal(node: ast.AST) -> bool:
    return isinstance(node, ast.Constant) and node.value is True


# ---------------------------------------------------------------------------
# Control-flow graph
# ---------------------------------------------------------------------------


class Node:
    __slots__ = ("succ", "stmt", "kind")

    def __init__(self, stmt: ast.AST | None = None, kind: str = "plain") -> None:
        self.succ: list[Node] = []
        self.stmt = stmt
        self.kind = kind  # "plain" | "exit"


class Ctx:
    __slots__ = ("exit", "exc", "brk", "cont", "guarded")

    def __init__(
        self,
        exit_: Node | None,
        exc: Node | None,
        brk: Node | None = None,
        cont: Node | None = None,
        guarded: bool = False,
    ) -> None:
        self.exit = exit_
        self.exc = exc
        self.brk = brk
        self.cont = cont
        self.guarded = guarded


def dedupe(nodes: list[Node | None]) -> list[Node]:
    out: list[Node] = []
    for n in nodes:
        if n is not None and n not in out:
            out.append(n)
    return out


class Graph:
    """Control-flow graph of one function scope."""

    def __init__(self) -> None:
        self.exit = Node(kind="exit")
        self.nodes_of_stmt: dict[int, list[Node]] = {}

    def node(self, stmt: ast.AST | None) -> Node:
        n = Node(stmt)
        if stmt is not None:
            self.nodes_of_stmt.setdefault(id(stmt), []).append(n)
        return n

    def seq(self, stmts: list[ast.stmt], k: Node, ctx: Ctx) -> Node:
        entry = k
        for stmt in reversed(stmts):
            entry = self.stmt(stmt, entry, ctx)
        return entry

    def stmt(self, stmt: ast.stmt, k: Node, ctx: Ctx) -> Node:
        t = type(stmt)
        if t is ast.Return:
            n = self.node(stmt)
            n.succ = dedupe([ctx.exit])
            return n
        if t is ast.Raise:
            n = self.node(stmt)
            n.succ = dedupe([ctx.exc])
            return n
        if t is ast.Break:
            n = self.node(stmt)
            n.succ = dedupe([ctx.brk])
            return n
        if t is ast.Continue:
            n = self.node(stmt)
            n.succ = dedupe([ctx.cont])
            return n
        if t is ast.If:
            n = self.node(stmt)
            then_e = self.seq(stmt.body, k, ctx)
            else_e = self.seq(stmt.orelse, k, ctx) if stmt.orelse else k
            n.succ = dedupe([then_e, else_e])
            return n
        if t in (ast.While, ast.For, ast.AsyncFor):
            n = self.node(stmt)
            orelse_e = self.seq(stmt.orelse, k, ctx) if stmt.orelse else k
            body_ctx = Ctx(ctx.exit, ctx.exc, brk=k, cont=n, guarded=ctx.guarded)
            body_e = self.seq(stmt.body, n, body_ctx)
            n.succ = dedupe([body_e, orelse_e])
            return n
        if t in (ast.With, ast.AsyncWith):
            n = self.node(stmt)
            body_e = self.seq(stmt.body, k, ctx)
            n.succ = dedupe([body_e, ctx.exc] if ctx.guarded else [body_e])
            return n
        if t is ast.Try or (_TRYSTAR is not None and t is _TRYSTAR):
            return self.try_stmt(stmt, k, ctx)
        if t is ast.Match:
            n = self.node(stmt)
            entries = [self.seq(case.body, k, ctx) for case in stmt.cases]
            n.succ = dedupe(entries + [k])
            return n
        if t in (ast.FunctionDef, ast.AsyncFunctionDef, ast.ClassDef):
            n = self.node(stmt)
            n.succ = [k]
            return n
        # Every other statement is straight-line.
        n = self.node(stmt)
        succ: list[Node | None] = [k]
        if ctx.guarded:
            succ.append(ctx.exc)
        n.succ = dedupe(succ)
        return n

    def try_stmt(self, stmt: Any, k: Node, ctx: Ctx) -> Node:
        finalbody = list(getattr(stmt, "finalbody", []) or [])
        handlers = list(getattr(stmt, "handlers", []) or [])
        orelse = list(getattr(stmt, "orelse", []) or [])
        cache: dict[int, Node | None] = {}

        def fin(cont: Node | None) -> Node | None:
            if cont is None:
                return None
            if not finalbody:
                return cont
            if id(cont) in cache:
                return cache[id(cont)]
            fctx = Ctx(ctx.exit, ctx.exc, ctx.brk, ctx.cont, guarded=False)
            entry: Node | None = self.seq(finalbody, cont, fctx)
            cache[id(cont)] = entry
            return entry

        normal_after = fin(k)
        assert normal_after is not None
        if orelse:
            octx = Ctx(fin(ctx.exit), fin(ctx.exc), fin(ctx.brk), fin(ctx.cont), guarded=False)
            body_k = self.seq(orelse, normal_after, octx)
        else:
            body_k = normal_after

        handler_entry: Node | None = self.node(None) if handlers else None
        body_exc = handler_entry if handler_entry is not None else fin(ctx.exc)
        bctx = Ctx(fin(ctx.exit), body_exc, fin(ctx.brk), fin(ctx.cont), guarded=True)
        body_entry = self.seq(stmt.body, body_k, bctx)

        if handler_entry is not None:
            succ: list[Node | None] = []
            catch_all = False
            for handler in handlers:
                htype = getattr(handler, "type", None)
                if htype is None or dotted(htype) in (
                    "Exception",
                    "BaseException",
                    "builtins.Exception",
                    "builtins.BaseException",
                ):
                    catch_all = True
                hctx = Ctx(fin(ctx.exit), fin(ctx.exc), fin(ctx.brk), fin(ctx.cont), guarded=False)
                succ.append(self.seq(handler.body, normal_after, hctx))
            if not catch_all:
                succ.append(fin(ctx.exc))
            handler_entry.succ = dedupe(succ)

        n = self.node(stmt)
        n.succ = [body_entry]
        return n


# ---------------------------------------------------------------------------
# Join recognition
# ---------------------------------------------------------------------------


def awaited_targets(node: ast.AST) -> set[str]:
    """Keys that `await node` waits for; `gather`/`wait` fan out over args."""
    if isinstance(node, ast.Call):
        out: set[str] = set()
        if call_short(node) in GATHER_METHODS:
            for arg in list(node.args) + [kw.value for kw in node.keywords]:
                out |= awaited_targets(arg)
            return out
        return {key_of(node)}
    if isinstance(node, (ast.List, ast.Tuple, ast.Set)):
        out = set()
        for elt in node.elts:
            out |= awaited_targets(elt)
        return out
    if isinstance(node, ast.Starred):
        return awaited_targets(node.value)
    if isinstance(node, (ast.Name, ast.Attribute, ast.Subscript)):
        return {key_of(node)}
    return set()


def make_join_predicate(kind: str, target: str, canon: Callable[[str], str]):
    """Build the predicate that recognizes a dominating join on `target`."""

    if kind == "thread":

        def predicate(node: ast.AST) -> bool:
            if not isinstance(node, ast.Call):
                return False
            func = node.func
            if not isinstance(func, ast.Attribute) or func.attr != "join":
                return False
            return canon(receiver_key(func.value)) == target

        return predicate

    def awaited(node: ast.AST) -> bool:
        if not isinstance(node, ast.Await):
            return False
        return target in {canon(k) for k in awaited_targets(node.value)}

    return awaited


def unconditional_join(stmt: ast.stmt, predicate: Callable[[ast.AST], bool]) -> bool:
    """True when `predicate` holds for a node stmt evaluates unconditionally."""

    def walk(node: ast.AST) -> bool:
        if type(node) in _CONDITIONAL:
            return False
        if predicate(node):
            return True
        for child in ast.iter_child_nodes(node):
            if walk(child):
                return True
        return False

    return walk(stmt)


def dominated(node: Node, joins: set[Node]) -> bool:
    """True when no path from node reaches an exit without passing a join."""
    if node in joins:
        return True
    seen = {node}
    stack = [node]
    while stack:
        cur = stack.pop()
        for nxt in cur.succ:
            if nxt in joins or nxt in seen:
                continue
            if nxt.kind == "exit":
                return False
            seen.add(nxt)
            stack.append(nxt)
    return True


# ---------------------------------------------------------------------------
# Scope walking
# ---------------------------------------------------------------------------


def iter_scope(stmts: list[ast.stmt]):
    """Yield every node of a scope; nested defs and class bodies are leaves."""
    stack: list[ast.AST] = list(stmts)
    while stack:
        node = stack.pop()
        yield node
        if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef, ast.ClassDef)):
            continue
        for child in ast.iter_child_nodes(node):
            stack.append(child)


def enclosing_stmt(node: ast.AST) -> ast.stmt | None:
    cur: ast.AST | None = node
    while cur is not None:
        if isinstance(cur, ast.stmt):
            return cur
        cur = getattr(cur, "parent", None)
    return None


def enclosing_has(node: ast.AST, pred: Callable[[ast.AST], bool]) -> bool:
    cur: ast.AST | None = getattr(node, "parent", None)
    while cur is not None:
        if pred(cur):
            return True
        cur = getattr(cur, "parent", None)
    return False


def direct_assign_target(stmt: ast.stmt | None, call: ast.AST) -> str:
    if isinstance(stmt, ast.Assign) and len(stmt.targets) == 1 and stmt.value is call:
        return key_of(stmt.targets[0])
    if isinstance(stmt, ast.AnnAssign) and stmt.value is call:
        return key_of(stmt.target)
    if isinstance(stmt, ast.NamedExpr) and stmt.value is call:
        return key_of(stmt.target)
    return ""


def collect_bindings(
    stmts: list[ast.stmt], thread_names: set[str], seed: dict[str, str] | None = None
) -> tuple[dict[str, str], Callable[[str], str]]:
    """Map key -> kind ('thread' | 'task' | 'offload') with `u = t` aliases.

    `seed` carries the file-wide attribute and container-element bindings from
    `collect_container_bindings`; every binding this scope makes itself, of any
    form, is layered on top.
    """
    pairs: list[tuple[ast.AST, ast.AST]] = []
    for node in iter_scope(stmts):
        if isinstance(node, ast.Assign):
            for tgt in node.targets:
                pairs.append((tgt, node.value))
        elif isinstance(node, ast.AnnAssign) and node.value is not None:
            pairs.append((node.target, node.value))
        elif isinstance(node, ast.NamedExpr):
            pairs.append((node.target, node.value))

    kinds: dict[str, str] = dict(seed) if seed else {}
    alias: dict[str, str] = {}

    def resolve(k: str) -> str:
        seen: set[str] = set()
        while k in alias and k not in seen:
            seen.add(k)
            k = alias[k]
        return k

    for tgt, value in pairs:
        kind = construct_kind(value, thread_names)
        if kind is not None:
            kinds[key_of(tgt)] = kind

    for _ in range(32):
        changed = False
        for tgt, value in pairs:
            if not isinstance(value, (ast.Name, ast.Attribute, ast.Subscript)):
                continue
            src = resolve(key_of(value))
            if src not in kinds:
                continue
            tk = key_of(tgt)
            if resolve(tk) == src:
                continue
            alias[tk] = src
            kinds[tk] = kinds[src]
            changed = True
        if not changed:
            break

    return kinds, resolve


def collect_container_bindings(tree: ast.Module, thread_names: set[str]) -> dict[str, str]:
    """Constructor bindings held on an attribute or a container element, file-wide.

    `self.worker = Thread(...)` in one method and `self.pool["a"] = Thread(...)`
    in another name targets an unrelated scope can start: the attribute belongs
    to the instance and the element to the container, not to the frame that
    assigned them. Plain names are not collected here, because a name bound in
    one function is not necessarily the same object in another.
    """
    kinds: dict[str, str] = {}
    for node in ast.walk(tree):
        if isinstance(node, ast.Assign):
            targets = node.targets
            value = node.value
        elif isinstance(node, ast.AnnAssign) and node.value is not None:
            targets = [node.target]
            value = node.value
        else:
            continue
        kind = construct_kind(value, thread_names)
        if kind is None:
            continue
        for tgt in targets:
            if isinstance(tgt, (ast.Attribute, ast.Subscript)):
                kinds[key_of(tgt)] = kind
    return kinds


def constructor_expression_scopes(tree: ast.Module) -> list[list[ast.stmt]]:
    """One-statement scopes for the expressions a `def`/`class` statement evaluates.

    A default argument, a decorator, and a class base are evaluated where the
    statement sits, not inside the body that follows it, so no function scope
    owns them and `iter_scope` never reaches them. Each position becomes its own
    scope with a single synthetic expression statement, so the launch rules run
    on it and nothing in it can be proven joined.
    """
    scopes: list[list[ast.stmt]] = []
    for node in ast.walk(tree):
        positions: list[ast.AST] = []
        if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
            positions.extend(node.decorator_list)
            positions.extend(node.args.defaults)
            positions.extend(d for d in node.args.kw_defaults if d is not None)
        elif isinstance(node, ast.ClassDef):
            positions.extend(node.decorator_list)
            positions.extend(node.bases)
            positions.extend(kw.value for kw in node.keywords)
        else:
            continue
        if not positions:
            continue
        stmts: list[ast.stmt] = []
        for position in positions:
            synthetic = ast.Expr(value=position)
            ast.copy_location(synthetic, position)
            position.parent = synthetic
            stmts.append(synthetic)
        scopes.append(stmts)
    return scopes


# ---------------------------------------------------------------------------
# Scope analysis
# ---------------------------------------------------------------------------


def source_line(lines: list[str], lineno: int) -> str:
    if 1 <= lineno <= len(lines):
        return lines[lineno - 1].strip()
    return ""


def analyze_scope(
    graph: Graph,
    body: list[ast.stmt],
    thread_names: set[str],
    lines: list[str],
    seed: dict[str, str] | None = None,
):
    kinds, canon = collect_bindings(body, thread_names, seed)
    scope_nodes = list(iter_scope(body))
    statements = [n for n in scope_nodes if isinstance(n, ast.stmt)]
    calls = [n for n in scope_nodes if isinstance(n, ast.Call)]
    findings: list[tuple[int, str]] = []

    def report(lineno: int, rule: str) -> None:
        findings.append((lineno, rule))

    def launch_joined(stmt: ast.stmt | None, kind: str, target: str) -> bool:
        if stmt is None or target == "":
            return False
        nodes = graph.nodes_of_stmt.get(id(stmt), [])
        if not nodes:
            return False
        predicate = make_join_predicate(kind, canon(target), canon)
        joins: set[Node] = set()
        for s in statements:
            if isinstance(s, _COMPOUND):
                continue
            if unconditional_join(s, predicate):
                joins.update(graph.nodes_of_stmt.get(id(s), []))
        return all(dominated(n, joins) for n in nodes)

    for call in calls:
        if is_thread_ctor(call, thread_names):
            if any(kw.arg == "daemon" and is_true_literal(kw.value) for kw in call.keywords):
                report(call.lineno, "daemon-thread")
            if enclosing_has(call, lambda n: isinstance(n, (ast.ListComp, ast.SetComp, ast.DictComp, ast.GeneratorExp))):
                report(call.lineno, "thread-comprehension")
            continue

        if is_task_ctor(call):
            if enclosing_has(call, lambda n: isinstance(n, ast.Await)):
                continue
            stmt = enclosing_stmt(call)
            if not launch_joined(stmt, "task", direct_assign_target(stmt, call)):
                report(call.lineno, "asyncio-task")
            continue

        if is_offload_ctor(call):
            rule = "asyncio-to-thread" if call_short(call) == "to_thread" else "run-in-executor"
            if enclosing_has(call, lambda n: isinstance(n, ast.Await)):
                continue
            stmt = enclosing_stmt(call)
            if not launch_joined(stmt, "offload", direct_assign_target(stmt, call)):
                report(call.lineno, rule)
            continue

        func = call.func
        if isinstance(func, ast.Attribute) and func.attr == "start":
            receiver = func.value
            if is_thread_ctor(receiver, thread_names):
                # An inline `Thread(...).start()` discards the handle, so it can
                # never be joined.
                report(call.lineno, "thread-start")
                continue
            target = canon(receiver_key(receiver))
            if kinds.get(target) != "thread":
                continue
            if not launch_joined(enclosing_stmt(call), "thread", target):
                report(call.lineno, "thread-start")
            continue

        if isinstance(func, ast.Attribute) and func.attr == "submit":
            if enclosing_has(
                call,
                lambda n: isinstance(n, (ast.With, ast.AsyncWith))
                and any(is_executor_ctor(item.context_expr) for item in n.items),
            ):
                continue
            report(call.lineno, "executor-submit")

    # `x.daemon = True` marks a thread as not-to-be-waited-for.
    for node in scope_nodes:
        if not isinstance(node, ast.Assign) or len(node.targets) != 1:
            continue
        target = node.targets[0]
        if not isinstance(target, ast.Attribute) or target.attr != "daemon":
            continue
        if not is_true_literal(node.value):
            continue
        if kinds.get(canon(key_of(target.value))) == "thread":
            report(node.lineno, "daemon-thread")

    out: list[dict[str, Any]] = []
    seen: set[tuple[int, str]] = set()
    for lineno, rule in sorted(set(findings)):
        if (lineno, rule) in seen:
            continue
        seen.add((lineno, rule))
        out.append({"line": lineno, "rule": rule, "text": source_line(lines, lineno)})
    return out


def scopes_of(tree: ast.Module) -> list[list[ast.stmt]]:
    scopes = [tree.body]
    for node in ast.walk(tree):
        if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
            scopes.append(node.body)
    return scopes


def attach_parents(tree: ast.AST) -> ast.AST:
    for node in ast.walk(tree):
        for child in ast.iter_child_nodes(node):
            child.parent = node
    return tree


def scan_file(path: str, source: str) -> dict[str, Any]:
    try:
        tree = ast.parse(source, filename=path)
    except SyntaxError as err:
        return {"path": path, "findings": [], "error": f"SyntaxError: {err}"}
    attach_parents(tree)
    lines = source.splitlines()
    thread_names = thread_names_from_imports(tree)
    seed = collect_container_bindings(tree, thread_names)
    findings: list[dict[str, Any]] = []
    for body in scopes_of(tree) + constructor_expression_scopes(tree):
        graph = Graph()
        graph.seq(body, graph.exit, Ctx(graph.exit, graph.exit))
        findings.extend(analyze_scope(graph, body, thread_names, lines, seed))
    out: list[dict[str, Any]] = []
    seen_findings: set[tuple[int, str]] = set()
    for finding in sorted(findings, key=lambda f: (f["line"], f["rule"])):
        key = (finding["line"], finding["rule"])
        if key in seen_findings:
            continue
        seen_findings.add(key)
        out.append(finding)
    return {"path": path, "findings": out, "error": None}


def main() -> int:
    request = json.load(sys.stdin)
    results = [scan_file(str(e.get("path", "<memory>")), str(e.get("source", ""))) for e in request.get("files", [])]
    json.dump({"results": results}, sys.stdout)
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
