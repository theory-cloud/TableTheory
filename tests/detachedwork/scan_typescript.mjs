#!/usr/bin/env node
/*
 * AST detached-work scanner for TypeScript/JavaScript sources.
 *
 * Invoked by TestTypeScript_NoDetachedWork and the TypeScript detector
 * self-tests in tests/detached_work_typescript_test.go, so the check runs
 * inside `make test-unit` and `make rubric`. It uses the TypeScript compiler
 * API from ts/node_modules/typescript (already a devDependency of ts/).
 *
 * Protocol (one request/response per process):
 *
 *   request  := {"files": [{"path": "<repo-relative>", "source": "<text>"}]}
 *   response := {"results": [{"path": ..., "findings": [
 *                   {"line": <1-based>, "rule": "<rule>", "text": "<source line>"}
 *               ], "error": null | "<message>"}]}
 *
 * Proof, exactly: a launch is reported unless a join on the same held target
 * executes on every path from the launch to every exit of the function that
 * contains it. The source is parsed with the TypeScript compiler; each function
 * scope (the file's top level included) gets a control-flow graph whose nodes
 * are statements and whose edges are fallthrough, branches, loop back-edges,
 * break/continue targets, and exception transfers into catch/finally clauses.
 * `return` and an unhandled `throw` are exits; a nested function is an opaque
 * statement, so its `return` is not this function's exit and its `await` is not
 * this function's join. A source the parser rejects is a scan error, not a
 * recovery-parsed tree.
 *
 * A launch is accepted only when no path from its statement to an exit avoids
 * every join statement, where a join statement is one that executes the join
 * unconditionally whenever it executes: an `await` (or `return`) of the held
 * target, or `await Promise.all/allSettled/race/any(...)` over it. A join
 * nested in an `if` branch, a loop body, a `try` body without a joining
 * `finally`, a nested arrow/function, a conditional expression (`c ? a : b`),
 * or a short-circuit operator (`c && a`, `c || a`, `a ?? b`) is not such a
 * statement, so `if (c) await p;`, `c && await p;`, and `c ? await p : 0` leave
 * the held promise reported. A join in a `finally` clause is reached from every
 * exit and so dominates; a join split across both branches of an `if/else` is
 * accepted, because the graph merges the branches.
 *
 * Recognized launches: timers (`setTimeout`/`setInterval`/`setImmediate`,
 * property or computed-member form) that are neither a promise-resolver delay
 * nor a handle this file clears or unrefs; `queueMicrotask` and
 * `process.nextTick`; a `void` discard of a call; a dropped async IIFE; a
 * dropped `.then`/`.catch`/`.finally` chain; a dropped call to a function this
 * file declares `async`; a promise bound to a name, member, or element and not
 * joined on every path (including `this.x =`, a rebinding `p = ...`, and a
 * container element `jobs["a"] = ...`); an `Array.from(...)`/`.map(...)` given
 * an async callback; and a class property initializer or `static { ... }` block,
 * which run while the instance or the class is built.
 *
 * Deliberately conservative, and documented as such in
 * docs/development-guidelines.md:
 *
 *   * Only exceptions raised inside a `try` block are modeled; they transfer to
 *     that statement's `catch` (or its `finally` and then outward). An exception
 *     anywhere else is not an exit, mirroring the Go detector.
 *   * A `Promise.all(...)`/`Promise.resolve(...)` call is treated as a launch
 *     only when it is dropped or bound; a dropped promise of an unknown
 *     producer is not inferred without types.
 *   * `switch` cases are modeled without fallthrough, so each case body is
 *     treated as ending the switch.
 *   * A class property initializer is reported whenever it launches, because the
 *     initializer itself cannot await: an `await` in a later method is not
 *     modeled as that initializer's join. A timer held in a field and cleared
 *     elsewhere in the class is reported for the same reason.
 *   * A dropped promise whose producer is only known from types (not syntax) is
 *     left to @typescript-eslint/no-floating-promises inside ts/**.
 */

import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const HERE = path.dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = path.resolve(HERE, '..', '..');

let ts;
try {
  const requireFromTs = createRequire(path.join(REPO_ROOT, 'ts', 'package.json'));
  ts = requireFromTs('typescript');
} catch (err) {
  process.stderr.write(`scan_typescript: cannot load the TypeScript compiler (${err})\n`);
  process.exit(2);
}

const TIMER_NAMES = new Set(['setTimeout', 'setInterval', 'setImmediate']);
const CLEAR_NAMES = new Set(['clearTimeout', 'clearInterval', 'clearImmediate']);
const CHAIN_NAMES = new Set(['then', 'catch', 'finally']);
const RESOLVER_NAMES = new Set(['resolve', 'r', 'res', 'done', 'reject', '_resolve', '_r']);
const PROMISE_STATIC_NAMES = new Set(['all', 'allSettled', 'race', 'any']);
const ASYNC_CALLBACK_METHODS = new Set([
  'map',
  'filter',
  'forEach',
  'flatMap',
  'reduce',
  'from',
  'some',
  'every',
  'find',
]);

const STOP_KINDS = new Set([
  ts.SyntaxKind.ConditionalExpression,
  ts.SyntaxKind.FunctionDeclaration,
  ts.SyntaxKind.FunctionExpression,
  ts.SyntaxKind.ArrowFunction,
  ts.SyntaxKind.MethodDeclaration,
  ts.SyntaxKind.GetAccessor,
  ts.SyntaxKind.SetAccessor,
  ts.SyntaxKind.Constructor,
  ts.SyntaxKind.ClassDeclaration,
  ts.SyntaxKind.ClassExpression,
  ts.SyntaxKind.IfStatement,
  ts.SyntaxKind.ForStatement,
  ts.SyntaxKind.ForInStatement,
  ts.SyntaxKind.ForOfStatement,
  ts.SyntaxKind.WhileStatement,
  ts.SyntaxKind.DoStatement,
  ts.SyntaxKind.SwitchStatement,
  ts.SyntaxKind.TryStatement,
  ts.SyntaxKind.Block,
]);

const FUNC_LIKE_KINDS = new Set([
  ts.SyntaxKind.FunctionDeclaration,
  ts.SyntaxKind.FunctionExpression,
  ts.SyntaxKind.ArrowFunction,
  ts.SyntaxKind.MethodDeclaration,
  ts.SyntaxKind.GetAccessor,
  ts.SyntaxKind.SetAccessor,
  ts.SyntaxKind.Constructor,
]);

const COMPOUND_KINDS = new Set([
  ts.SyntaxKind.IfStatement,
  ts.SyntaxKind.ForStatement,
  ts.SyntaxKind.ForInStatement,
  ts.SyntaxKind.ForOfStatement,
  ts.SyntaxKind.WhileStatement,
  ts.SyntaxKind.DoStatement,
  ts.SyntaxKind.SwitchStatement,
  ts.SyntaxKind.TryStatement,
  ts.SyntaxKind.Block,
  ts.SyntaxKind.FunctionDeclaration,
  ts.SyntaxKind.ClassDeclaration,
]);

function isFunctionLike(node) {
  return FUNC_LIKE_KINDS.has(node.kind);
}

function isClassLike(node) {
  return (
    node.kind === ts.SyntaxKind.ClassDeclaration ||
    node.kind === ts.SyntaxKind.ClassExpression ||
    node.kind === ts.SyntaxKind.ClassStaticBlockDeclaration
  );
}

function hasAsync(node) {
  const mods = node.modifiers;
  return !!mods && mods.some((m) => m.kind === ts.SyntaxKind.AsyncKeyword);
}

function skipParens(node) {
  let cur = node;
  while (cur && ts.isParenthesizedExpression(cur)) {
    cur = cur.expression;
  }
  return cur;
}

function memberInfo(node) {
  const expr = skipParens(node);
  if (!expr) {
    return null;
  }
  if (ts.isPropertyAccessExpression(expr)) {
    return { name: expr.name.text, receiver: expr.expression };
  }
  if (ts.isElementAccessExpression(expr)) {
    const arg = expr.argumentExpression;
    if (arg && (ts.isStringLiteral(arg) || ts.isNoSubstitutionTemplateLiteral(arg))) {
      return { name: arg.text, receiver: expr.expression };
    }
  }
  return null;
}

function calleeName(node) {
  const expr = skipParens(node);
  if (!expr) {
    return null;
  }
  if (ts.isIdentifier(expr)) {
    return expr.text;
  }
  const info = memberInfo(expr);
  return info ? info.name : null;
}

function isAsyncFunctionExpression(node) {
  const expr = skipParens(node);
  return (
    !!expr && (ts.isArrowFunction(expr) || ts.isFunctionExpression(expr)) && hasAsync(expr)
  );
}

function walkAll(node, cb) {
  cb(node);
  ts.forEachChild(node, (child) => walkAll(child, cb));
}

// ---------------------------------------------------------------------------
// Control-flow graph
// ---------------------------------------------------------------------------

class GNode {
  constructor(stmt) {
    this.succ = [];
    this.stmt = stmt ?? null;
    this.kind = 'plain';
  }
}

class Ctx {
  constructor(exit, exc, brk, cont, guarded) {
    this.exit = exit;
    this.exc = exc;
    this.brk = brk;
    this.cont = cont;
    this.guarded = guarded;
  }
}

function dedupe(nodes) {
  const out = [];
  for (const n of nodes) {
    if (n && !out.includes(n)) {
      out.push(n);
    }
  }
  return out;
}

class Graph {
  constructor() {
    this.exit = new GNode();
    this.exit.kind = 'exit';
    this.nodesOfStmt = new Map();
  }

  node(stmt) {
    const n = new GNode(stmt);
    if (stmt) {
      const list = this.nodesOfStmt.get(stmt) ?? [];
      list.push(n);
      this.nodesOfStmt.set(stmt, list);
    }
    return n;
  }

  seq(stmts, k, ctx) {
    let entry = k;
    for (let i = stmts.length - 1; i >= 0; i -= 1) {
      entry = this.stmt(stmts[i], entry, ctx);
    }
    return entry;
  }

  stmt(s, k, ctx) {
    if (s.kind === ts.SyntaxKind.ReturnStatement) {
      const n = this.node(s);
      n.succ = dedupe([ctx.exit]);
      return n;
    }
    if (s.kind === ts.SyntaxKind.ThrowStatement) {
      const n = this.node(s);
      n.succ = dedupe([ctx.exc]);
      return n;
    }
    if (s.kind === ts.SyntaxKind.BreakStatement) {
      const n = this.node(s);
      n.succ = dedupe([s.label ? ctx.exit : ctx.brk]);
      return n;
    }
    if (s.kind === ts.SyntaxKind.ContinueStatement) {
      const n = this.node(s);
      n.succ = dedupe([s.label ? ctx.exit : ctx.cont]);
      return n;
    }
    if (s.kind === ts.SyntaxKind.Block) {
      return this.seq(s.statements, k, ctx);
    }
    if (s.kind === ts.SyntaxKind.IfStatement) {
      const n = this.node(s);
      const thenE = this.stmt(s.thenStatement, k, ctx);
      const elseE = s.elseStatement ? this.stmt(s.elseStatement, k, ctx) : k;
      n.succ = dedupe([thenE, elseE]);
      return n;
    }
    if (
      s.kind === ts.SyntaxKind.WhileStatement ||
      s.kind === ts.SyntaxKind.ForStatement ||
      s.kind === ts.SyntaxKind.ForInStatement ||
      s.kind === ts.SyntaxKind.ForOfStatement
    ) {
      const n = this.node(s);
      const bodyCtx = new Ctx(ctx.exit, ctx.exc, k, n, ctx.guarded);
      const bodyE = this.stmt(s.statement, n, bodyCtx);
      n.succ = dedupe([bodyE, k]);
      return n;
    }
    if (s.kind === ts.SyntaxKind.DoStatement) {
      const n = this.node(s);
      const bodyCtx = new Ctx(ctx.exit, ctx.exc, k, n, ctx.guarded);
      const bodyE = this.stmt(s.statement, n, bodyCtx);
      n.succ = dedupe([bodyE, k]);
      return bodyE;
    }
    if (s.kind === ts.SyntaxKind.SwitchStatement) {
      const n = this.node(s);
      const brkCtx = new Ctx(ctx.exit, ctx.exc, k, ctx.cont, ctx.guarded);
      const entries = [];
      for (const clause of s.caseBlock.clauses) {
        entries.push(this.seq(clause.statements, k, brkCtx));
      }
      n.succ = dedupe([...entries, k]);
      return n;
    }
    if (s.kind === ts.SyntaxKind.TryStatement) {
      return this.tryStmt(s, k, ctx);
    }
    if (s.kind === ts.SyntaxKind.LabeledStatement) {
      return this.stmt(s.statement, k, ctx);
    }
    if (s.kind === ts.SyntaxKind.FunctionDeclaration || s.kind === ts.SyntaxKind.ClassDeclaration) {
      const n = this.node(s);
      n.succ = [k];
      return n;
    }
    const n = this.node(s);
    const succ = [k];
    if (ctx.guarded) {
      succ.push(ctx.exc);
    }
    n.succ = dedupe(succ);
    return n;
  }

  tryStmt(s, k, ctx) {
    const finalStmts = s.finallyBlock ? s.finallyBlock.statements : null;
    const cache = new Map();
    const fin = (cont) => {
      if (!cont) {
        return null;
      }
      if (!finalStmts) {
        return cont;
      }
      if (cache.has(cont)) {
        return cache.get(cont);
      }
      const fctx = new Ctx(ctx.exit, ctx.exc, ctx.brk, ctx.cont, false);
      const entry = this.seq(finalStmts, cont, fctx);
      cache.set(cont, entry);
      return entry;
    };
    const normalAfter = fin(k);
    const catchEntry = s.catchClause ? this.node(null) : null;
    const tryExc = catchEntry ?? fin(ctx.exc);
    const tctx = new Ctx(fin(ctx.exit), tryExc, fin(ctx.brk), fin(ctx.cont), true);
    const tryEntry = this.seq(s.tryBlock.statements, normalAfter, tctx);
    if (catchEntry) {
      const cctx = new Ctx(fin(ctx.exit), fin(ctx.exc), fin(ctx.brk), fin(ctx.cont), false);
      catchEntry.succ = dedupe([this.seq(s.catchClause.block.statements, normalAfter, cctx)]);
    }
    const n = this.node(s);
    n.succ = [tryEntry];
    return n;
  }
}

// ---------------------------------------------------------------------------
// Join recognition
// ---------------------------------------------------------------------------

function joinedKeys(file, expr, out) {
  if (!expr) {
    return out;
  }
  const node = skipParens(expr);
  if (node && ts.isCallExpression(node)) {
    const info = memberInfo(node.expression);
    const name = info ? info.name : null;
    const receiverText = info ? info.receiver.getText(file) : '';
    if (name && PROMISE_STATIC_NAMES.has(name) && receiverText === 'Promise') {
      for (const arg of node.arguments) {
        joinedKeys(file, arg, out);
      }
      return out;
    }
  }
  if (node && ts.isArrayLiteralExpression(node)) {
    for (const el of node.elements) {
      joinedKeys(file, el, out);
    }
    return out;
  }
  out.add(expr.getText(file));
  return out;
}

function makeJoinPredicate(file, target) {
  return (node) => {
    if (ts.isAwaitExpression(node)) {
      return joinedKeys(file, node.expression, new Set()).has(target);
    }
    if (ts.isReturnStatement(node) && node.expression) {
      return joinedKeys(file, node.expression, new Set()).has(target);
    }
    return false;
  };
}

function unconditionalJoin(stmt, predicate) {
  const walk = (node) => {
    if (STOP_KINDS.has(node.kind)) {
      return false;
    }
    if (ts.isBinaryExpression(node)) {
      const op = node.operatorToken.kind;
      if (
        op === ts.SyntaxKind.AmpersandAmpersandToken ||
        op === ts.SyntaxKind.BarBarToken ||
        op === ts.SyntaxKind.QuestionQuestionToken
      ) {
        return false;
      }
    }
    if (predicate(node)) {
      return true;
    }
    let found = false;
    ts.forEachChild(node, (child) => {
      if (!found && walk(child)) {
        found = true;
      }
    });
    return found;
  };
  return walk(stmt);
}

function dominates(node, joins) {
  if (joins.has(node)) {
    return true;
  }
  const seen = new Set([node]);
  const stack = [node];
  while (stack.length > 0) {
    const cur = stack.pop();
    for (const next of cur.succ) {
      if (joins.has(next) || seen.has(next)) {
        continue;
      }
      if (next.kind === 'exit') {
        return false;
      }
      seen.add(next);
      stack.push(next);
    }
  }
  return true;
}

// ---------------------------------------------------------------------------
// Scope gathering
// ---------------------------------------------------------------------------

function collectScopes(file) {
  const scopes = [{ root: file, stmts: file.statements, initializer: null }];
  walkAll(file, (node) => {
    if (node === file) {
      return;
    }
    if (isFunctionLike(node) && node.body && ts.isBlock(node.body)) {
      scopes.push({ root: node, stmts: node.body.statements, initializer: null });
      return;
    }
    if (node.kind === ts.SyntaxKind.ClassStaticBlockDeclaration && node.body) {
      scopes.push({ root: node, stmts: node.body.statements, initializer: null });
      return;
    }
    // A field initializer runs while the instance (or the class) is being
    // built, so it is a launch position in its own right; nothing inside it can
    // await, so no join in it can dominate a launch it makes.
    if (ts.isPropertyDeclaration(node) && node.initializer) {
      scopes.push({ root: node, stmts: [], initializer: node.initializer });
    }
  });
  return scopes;
}

function collectInScope(stmts) {
  const out = [];
  const visit = (node) => {
    out.push(node);
    if (isFunctionLike(node) || isClassLike(node)) {
      return;
    }
    ts.forEachChild(node, visit);
  };
  for (const s of stmts) {
    visit(s);
  }
  return out;
}

// ---------------------------------------------------------------------------
// Launch classification
// ---------------------------------------------------------------------------

function promiseRule(file, expr, managed, asyncNames) {
  if (ts.isVoidExpression(expr)) {
    return ts.isCallExpression(expr.expression) ? 'void-call' : null;
  }
  if (!ts.isCallExpression(expr)) {
    return null;
  }
  if (isAsyncFunctionExpression(expr.expression)) {
    return 'detached-async-iife';
  }
  const info = memberInfo(expr.expression);
  const name = info ? info.name : calleeName(expr.expression);
  if (info && CHAIN_NAMES.has(info.name)) {
    return `floating-${info.name}`;
  }
  if (name === 'queueMicrotask') {
    return 'queue-microtask';
  }
  if (info && info.name === 'nextTick' && info.receiver.getText(file) === 'process') {
    return 'process-next-tick';
  }
  if (name && TIMER_NAMES.has(name)) {
    const first = expr.arguments[0];
    if (first && ts.isIdentifier(first) && RESOLVER_NAMES.has(first.text)) {
      return null;
    }
    const handle = timerHandleName(file, expr);
    if (handle && managed.has(handle)) {
      return null;
    }
    return `floating-${name}`;
  }
  if (!info && name && asyncNames.has(name)) {
    return 'discarded-async-call';
  }
  if (info && info.name && PROMISE_STATIC_NAMES.has(info.name) && info.receiver.getText(file) === 'Promise') {
    return 'floating-promise';
  }
  if (name && ASYNC_CALLBACK_METHODS.has(name) && expr.arguments.some(isAsyncFunctionExpression)) {
    return 'floating-promise';
  }
  return null;
}

function timerHandleName(file, call) {
  const parent = call.parent;
  if (parent && ts.isVariableDeclaration(parent) && parent.initializer === call) {
    return parent.name.getText(file);
  }
  if (parent && ts.isBinaryExpression(parent) && parent.right === call) {
    return parent.left.getText(file);
  }
  return null;
}

function managedHandleNames(file) {
  const managed = new Set();
  walkAll(file, (node) => {
    if (!ts.isCallExpression(node)) {
      return;
    }
    const info = memberInfo(node.expression);
    const name = info ? info.name : ts.isIdentifier(node.expression) ? node.expression.text : null;
    if (name && CLEAR_NAMES.has(name) && node.arguments.length >= 1) {
      managed.add(node.arguments[0].getText(file));
    }
    if (info && info.name === 'unref') {
      managed.add(info.receiver.getText(file));
    }
  });
  return managed;
}

function asyncFunctionNames(file) {
  const names = new Set();
  walkAll(file, (node) => {
    if (ts.isFunctionDeclaration(node) && hasAsync(node) && node.name) {
      names.add(node.name.text);
    }
    if (ts.isVariableDeclaration(node) && node.initializer && isAsyncFunctionExpression(node.initializer)) {
      if (ts.isIdentifier(node.name)) {
        names.add(node.name.text);
      }
    }
  });
  return names;
}

// ---------------------------------------------------------------------------
// Scope analysis
// ---------------------------------------------------------------------------

function analyzeScope(file, scope, managed, asyncNames, lines) {
  const stmts = scope.stmts;
  const graph = new Graph();
  graph.seq(stmts, graph.exit, new Ctx(graph.exit, graph.exit, null, null, false));
  const nodes = collectInScope(stmts);
  const statements = nodes.filter((n) => ts.isStatement(n));
  const findings = [];
  const record = (node, rule) => findings.push({ line: lineOf(node), rule });

  const launchJoined = (stmt, target) => {
    if (!stmt || !target) {
      return false;
    }
    const owners = graph.nodesOfStmt.get(stmt) ?? [];
    if (owners.length === 0) {
      return false;
    }
    const predicate = makeJoinPredicate(file, target);
    const joins = new Set();
    for (const s of statements) {
      if (COMPOUND_KINDS.has(s.kind)) {
        continue;
      }
      if (unconditionalJoin(s, predicate)) {
        for (const n of graph.nodesOfStmt.get(s) ?? []) {
          joins.add(n);
        }
      }
    }
    return owners.every((n) => dominates(n, joins));
  };

  for (const node of nodes) {
    if (!ts.isExpressionStatement(node)) {
      continue;
    }
    const expr = node.expression;
    if (ts.isAwaitExpression(expr)) {
      continue;
    }
    if (ts.isBinaryExpression(expr) && isAssignmentOperator(expr.operatorToken.kind)) {
      const rule = promiseRule(file, expr.right, managed, asyncNames);
      if (rule && !launchJoined(node, expr.left.getText(file))) {
        record(node, rule);
      }
      continue;
    }
    const rule = promiseRule(file, expr, managed, asyncNames);
    if (rule) {
      record(node, rule);
    }
  }

  for (const node of nodes) {
    if (!ts.isVariableStatement(node)) {
      continue;
    }
    for (const decl of node.declarationList.declarations) {
      if (!decl.initializer || ts.isAwaitExpression(decl.initializer)) {
        continue;
      }
      const rule = promiseRule(file, decl.initializer, managed, asyncNames);
      if (rule && !launchJoined(node, decl.name.getText(file))) {
        record(node, rule);
      }
    }
  }

  if (scope.initializer) {
    const rule = promiseRule(file, scope.initializer, managed, asyncNames);
    if (rule) {
      record(scope.initializer, rule);
    }
  }

  return findings;
}

function isAssignmentOperator(kind) {
  return kind === ts.SyntaxKind.EqualsToken;
}

function lineOf(node) {
  const pos = node.getStart();
  return node.getSourceFile().getLineAndCharacterOfPosition(pos).line + 1;
}

function sourceLine(lines, lineno) {
  return lineno >= 1 && lineno <= lines.length ? lines[lineno - 1].trim() : '';
}

function scriptKindFor(fileName) {
  const lower = fileName.toLowerCase();
  if (lower.endsWith('.js') || lower.endsWith('.mjs') || lower.endsWith('.cjs')) {
    return ts.ScriptKind.JS;
  }
  return ts.ScriptKind.TS;
}

function scanFile(fileName, source) {
  const file = ts.createSourceFile(fileName, source, ts.ScriptTarget.Latest, true, scriptKindFor(fileName));
  // A source file created on its own carries the diagnostics the parser
  // recorded; the compiler API exposes no program-free getSyntacticDiagnostics,
  // and building a Program would pull in a default lib and report diagnostics
  // that have nothing to do with this source. A rejected source is a scan
  // failure rather than a silently recovery-parsed tree.
  const diagnostics = file.parseDiagnostics ?? [];
  if (diagnostics.length > 0) {
    const first = diagnostics[0];
    const message = ts.flattenDiagnosticMessageText(first.messageText, ' ');
    const at = file.getLineAndCharacterOfPosition(first.start ?? 0);
    return { path: fileName, findings: [], error: `SyntaxError: ${message} (line ${at.line + 1})` };
  }
  const managed = managedHandleNames(file);
  const asyncNames = asyncFunctionNames(file);
  const lines = source.split('\n');
  const findings = [];
  for (const scope of collectScopes(file)) {
    for (const f of analyzeScope(file, scope, managed, asyncNames, lines)) {
      findings.push({ line: f.line, rule: f.rule, text: sourceLine(lines, f.line) });
    }
  }
  const seen = new Set();
  const unique = [];
  findings.sort((a, b) => a.line - b.line || a.rule.localeCompare(b.rule));
  for (const f of findings) {
    const key = `${f.line}:${f.rule}`;
    if (seen.has(key)) {
      continue;
    }
    seen.add(key);
    unique.push(f);
  }
  return { path: fileName, findings: unique, error: null };
}

function readStdin() {
  return new Promise((resolve, reject) => {
    let data = '';
    process.stdin.setEncoding('utf8');
    process.stdin.on('data', (chunk) => {
      data += chunk;
    });
    process.stdin.on('end', () => resolve(data));
    process.stdin.on('error', reject);
  });
}

const request = JSON.parse(await readStdin());
const results = [];
for (const entry of request.files ?? []) {
  try {
    results.push(scanFile(String(entry.path ?? '<memory>'), String(entry.source ?? '')));
  } catch (err) {
    results.push({ path: String(entry.path ?? '<memory>'), findings: [], error: String(err && err.stack ? err.stack : err) });
  }
}
process.stdout.write(`${JSON.stringify({ results })}\n`);
