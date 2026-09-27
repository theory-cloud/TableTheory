package tests

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// This file holds the shared machinery for the detached-work guard family. The
// operator ruling behind it is absolute: no work may run in a goroutine,
// thread, async task, or promise that outlives the invocation that started it.
// That holds for the examples (which deploy as Lambda functions) and for the
// library sources themselves, in every runtime: Go, TypeScript, and Python.
//
// The guard is deliberately split by language so that each detector can be
// proven not vacuous on its own (see the *DetectorIsNotVacuous tests), and so
// that a reviewer can read one language's rules at a time.
//
// What the Go half checks is narrower than "this goroutine is safe" and wider
// than "a Wait appears somewhere after the launch". A launch is accepted only
// when the join dominates every exit from the function that launched it: for
// every return path that starts after the launch statement, the join is
// executed on the way out (no return, no branch out of the region, no
// conditional, and no never-called closure in between). The dominance walk and
// the two join shapes it recognizes are described at goLaunchJoined.

// detachedWorkRepoRoot resolves the repository root from this file's location,
// independent of the working directory `go test` happens to use.
func detachedWorkRepoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve this test file's path")
	}
	root := filepath.Dir(filepath.Dir(file))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("resolved repo root %q has no go.mod: %v", root, err)
	}
	return root
}

// detachedWorkSkipDirs are dependency and build-output directories that never
// hold hand-written source we are responsible for. Skipping them by name keeps
// the guard deterministic and stops it from policing vendored third-party code
// or generated output (for example the ThreadPoolExecutor inside the
// gitignored examples/cdk-multilang/cdk.out tree).
var detachedWorkSkipDirs = map[string]bool{
	".git":          true,
	"node_modules":  true,
	"vendor":        true,
	".venv":         true,
	"venv":          true,
	"__pycache__":   true,
	"dist":          true,
	"build":         true,
	"cdk.out":       true,
	"coverage":      true,
	".pytest_cache": true,
	".ruff_cache":   true,
	".mypy_cache":   true,
	"_site":         true,
	".jekyll-cache": true,
}

// walkDetachedWorkSources visits every file below each of dirs (relative to the
// repository root) whose repository-relative slash path satisfies include, and
// hands the file's contents to visit. Missing roots are skipped so a language
// surface that does not exist cannot make the guard fail; the caller is
// responsible for proving it scanned something.
func walkDetachedWorkSources(t *testing.T, root string, dirs []string, include func(rel string) bool, visit func(rel string, src []byte) error) {
	t.Helper()

	for _, dir := range dirs {
		base := filepath.Join(root, dir)
		if _, err := os.Stat(base); err != nil {
			continue
		}
		err := filepath.WalkDir(base, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if detachedWorkSkipDirs[entry.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if include != nil && !include(rel) {
				return nil
			}
			// The walker is the single place that reads source, always from a
			// path it derived itself by walking the repository root.
			src, err := os.ReadFile(filepath.Clean(path))
			if err != nil {
				return err
			}
			return visit(rel, src)
		})
		if err != nil {
			t.Fatalf("scan %s: %v", dir, err)
		}
	}
}

// launchAllowlist maps "<relative path>:<line>" to the justification for
// permitting work at that exact line to run detached. Keying on the line (not
// the file) makes every allowance a reviewed, verifiable claim about one
// specific launch, and a stale key is a hard failure: the guard asserts that
// every key matched a launch it actually found, so an entry cannot silently
// outlive the code it was written for. Every entry is reported in the PR body.
type launchAllowlist map[string]string

// allowlisted reports whether rel:line is allowlisted, marking the key as seen.
func (a launchAllowlist) allowlisted(rel string, line int) bool {
	if a == nil {
		return false
	}
	_, ok := a[fmt.Sprintf("%s:%d", rel, line)]
	return ok
}

// checkAllowlistCoverage fails when an allowlist key never matched a launch the
// scan found, which would mean the justification is stale.
func (a launchAllowlist) checkAllowlistCoverage(t *testing.T, scanned map[string]bool) {
	t.Helper()
	for key := range a {
		if !scanned[key] {
			t.Errorf("allowlist entry %q matched no scanned launch; remove it or fix the line", key)
		}
	}
}

// reportAllowlist logs every allowlist entry so the PR body can quote the
// complete, verifiable set.
func reportAllowlist(t *testing.T, a launchAllowlist) {
	t.Helper()
	if len(a) == 0 {
		t.Log("detached-work allowlist: (empty)")
		return
	}
	keys := make([]string, 0, len(a))
	for key := range a {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		t.Logf("detached-work allowlist: %s — %s", key, a[key])
	}
}

// goLaunchSite is one detached-work finding located by the AST scanner: either
// a goroutine launch (`go` statement or errgroup method) or a timer that
// schedules work past the caller's return.
type goLaunchSite struct {
	Rel      string
	Text     string
	Func     string
	Evidence string
	Kind     string
	Line     int
	Joined   bool
}

// funcBody pairs a function's body with the declaration that owns it, so a
// launch can be attributed and named.
type funcBody struct {
	body *ast.BlockStmt
	decl ast.Node
}

// scanGoSource parses Go source with go/ast — never a regex — and returns every
// detached-work finding it contains. Parsing means a launch is recognized
// wherever it appears, including the `; go f()` form on a line that also holds
// other statements, which a line-anchored pattern misses.
//
// Three launch mechanisms are found: `go` statements, errgroup-style
// `<g>.Go(func(){...})` calls (errgroup hides a goroutine behind an ordinary
// method call), and the timer APIs that run a callback or deliver ticks after
// the caller returns (time.AfterFunc, time.NewTimer, time.NewTicker, time.Tick)
// when nothing stops them.
func scanGoSource(fset *token.FileSet, rel string, src []byte) ([]goLaunchSite, error) {
	file, err := parser.ParseFile(fset, rel, src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}

	var bodies []funcBody
	ast.Inspect(file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncDecl:
			if x.Body != nil {
				bodies = append(bodies, funcBody{body: x.Body, decl: x})
			}
		case *ast.FuncLit:
			if x.Body != nil {
				bodies = append(bodies, funcBody{body: x.Body, decl: x})
			}
		}
		return true
	})

	var sites []goLaunchSite
	record := func(node ast.Node, launched *ast.BlockStmt, recv string, errgroup bool, kind string) {
		owner := innermostFuncBody(bodies, node.Pos(), node.End())
		pos := fset.Position(node.Pos())
		site := goLaunchSite{
			Rel:  rel,
			Line: pos.Line,
			Text: strings.TrimSpace(sourceLine(src, pos.Line)),
			Kind: kind,
		}
		if owner != nil {
			site.Func = funcName(owner.decl)
			if errgroup {
				site.Joined, site.Evidence = errgroupLaunchJoined(owner.body, node.Pos(), recv)
			} else {
				site.Joined, site.Evidence = goLaunchJoined(owner.body, node.Pos(), launched)
			}
		}
		sites = append(sites, site)
	}

	ast.Inspect(file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.GoStmt:
			record(x, launchedFuncBody(x), "", false, "go-statement")
		case *ast.CallExpr:
			if launched, recv, ok := errgroupFuncLit(x); ok {
				record(x, launched, recv, true, "errgroup-launch")
			}
		}
		return true
	})

	scanDetachedTimers(fset, rel, src, file, bodies, &sites)
	return sites, nil
}

// scanDetachedTimers reports timer constructors that schedule work past the
// caller's return and are never stopped. The handle is resolved from the
// statement that binds it (assignment, var declaration, or bare call) so a
// stopped timer is accepted and an unmanaged one is reported; a constructor the
// statement pass cannot attribute is reported as unmanaged.
func scanDetachedTimers(fset *token.FileSet, rel string, src []byte, file *ast.File, bodies []funcBody, sites *[]goLaunchSite) {
	handled := map[token.Pos]bool{}
	report := func(call *ast.CallExpr, timer, name string) {
		handled[call.Pos()] = true
		owner := innermostFuncBody(bodies, call.Pos(), call.End())
		if timer != "time.Tick" && name != "" && owner != nil && bodyCallsMethod(owner.body, name, "Stop") {
			// The handle is stopped in the same function, so nothing fires after
			// this function returns.
			return
		}
		pos := fset.Position(call.Pos())
		evidence := fmt.Sprintf("%s schedules work that runs after the enclosing function returns and is never stopped", timer)
		if name == "" && timer != "time.Tick" {
			evidence = fmt.Sprintf("%s returns a handle nothing binds, so it cannot be stopped", timer)
		}
		if timer == "time.Tick" {
			evidence = "time.Tick returns a channel whose ticker can never be stopped, so it outlives every caller"
		}
		fn := "unknown"
		if owner != nil {
			fn = funcName(owner.decl)
		}
		*sites = append(*sites, goLaunchSite{
			Rel:      rel,
			Line:     pos.Line,
			Text:     strings.TrimSpace(sourceLine(src, pos.Line)),
			Func:     fn,
			Kind:     "detached-timer",
			Joined:   false,
			Evidence: evidence,
		})
	}

	ast.Inspect(file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.AssignStmt:
			for i, rhs := range x.Rhs {
				call, ok := rhs.(*ast.CallExpr)
				if !ok {
					continue
				}
				timer, ok := timerConstructor(call)
				if !ok {
					continue
				}
				name := ""
				if i < len(x.Lhs) {
					name = exprText(x.Lhs[i])
				}
				report(call, timer, name)
			}
		case *ast.ValueSpec:
			for i, value := range x.Values {
				call, ok := value.(*ast.CallExpr)
				if !ok {
					continue
				}
				timer, ok := timerConstructor(call)
				if !ok {
					continue
				}
				name := ""
				if i < len(x.Names) {
					name = x.Names[i].Name
				}
				report(call, timer, name)
			}
		case *ast.ExprStmt:
			call, ok := x.X.(*ast.CallExpr)
			if !ok {
				return true
			}
			timer, ok := timerConstructor(call)
			if !ok {
				return true
			}
			report(call, timer, "")
		}
		return true
	})

	// Anything the statement pass did not attribute (for example a constructor
	// nested inside a larger expression) is still a timer nobody can stop.
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || handled[call.Pos()] {
			return true
		}
		if timer, ok := timerConstructor(call); ok {
			report(call, timer, "")
		}
		return true
	})
}

// errgroupFuncLit recognizes an errgroup-style launch: a call to a method named
// Go whose argument is a function literal. It returns the literal's body and
// the receiver expression used to name the group.
func errgroupFuncLit(call *ast.CallExpr) (*ast.BlockStmt, string, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Go" {
		return nil, "", false
	}
	for _, arg := range call.Args {
		if lit, ok := arg.(*ast.FuncLit); ok {
			return lit.Body, exprText(sel.X), true
		}
	}
	return nil, "", false
}

// timerConstructor recognizes the timer APIs that keep work running past the
// caller's return. time.Tick is included because its ticker can never be
// stopped; the others are reported only when their handle is not stopped.
func timerConstructor(call *ast.CallExpr) (string, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok || pkg.Name != "time" {
		return "", false
	}
	switch sel.Sel.Name {
	case "Tick", "AfterFunc", "NewTimer", "NewTicker":
		return "time." + sel.Sel.Name, true
	default:
		return "", false
	}
}

// isMethodCall reports whether node is a call of the form <recv>.<method>(...).
func isMethodCall(node ast.Node, recv, method string) bool {
	call, ok := node.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != method {
		return false
	}
	return exprText(sel.X) == recv
}

// innermostFuncBody returns the tightest function body containing [start,end).
func innermostFuncBody(bodies []funcBody, start, end token.Pos) *funcBody {
	var best *funcBody
	for i := range bodies {
		b := &bodies[i]
		if b.body.Pos() <= start && end <= b.body.End() {
			if best == nil || b.body.Pos() > best.body.Pos() {
				best = b
			}
		}
	}
	return best
}

// funcName renders a stable name for a FuncDecl or FuncLit owner.
func funcName(decl ast.Node) string {
	switch x := decl.(type) {
	case *ast.FuncDecl:
		if x.Recv != nil && len(x.Recv.List) > 0 {
			return exprText(x.Recv.List[0].Type) + "." + x.Name.Name
		}
		return x.Name.Name
	case *ast.FuncLit:
		return "func literal"
	default:
		return "unknown"
	}
}

// ---------------------------------------------------------------------------
// Join dominance
// ---------------------------------------------------------------------------
//
// A launch counts as joined only when the join dominates every exit from the
// function that launched it. The analysis walks the statement lists of the
// owner function (blocks, if/else bodies, loops, switch and select clauses) and
// asks: starting at the statement after the launch, does every path that
// reaches a return of the owner execute the join first?
//
// It is deliberately structural and conservative in the direction that matters:
// a shape it does not model is reported as unjoined rather than assumed joined.

// joinFlow describes how a statement-list region treats the join.
type joinFlow int

const (
	// flowContinues: no path executed the join, and no path left the owner.
	flowContinues joinFlow = iota
	// flowJoined: every path through the region executed the join.
	flowJoined
	// flowEscaped: some path returned from the owner without the join.
	flowEscaped
)

// stmtList is one statement list inside a function body, linked to the list
// that contains it so a launch can be followed forward through every enclosing
// block up to the function's own body.
type stmtList struct {
	parent    *stmtList
	stmts     []ast.Stmt
	parentIdx int
	depth     int
	ownerBody bool
}

// buildStmtLists builds the statement-list tree of a function body.
func buildStmtLists(body *ast.BlockStmt) []*stmtList {
	root := &stmtList{stmts: body.List, ownerBody: true}
	all := []*stmtList{root}
	var walk func(list *stmtList)
	walk = func(list *stmtList) {
		for i, stmt := range list.stmts {
			for _, childStmts := range childStatementLists(stmt) {
				child := &stmtList{stmts: childStmts, parent: list, parentIdx: i, depth: list.depth + 1}
				all = append(all, child)
				walk(child)
			}
		}
	}
	walk(root)
	return all
}

// childStatementLists returns the statement lists directly inside stmt. Bodies
// of closures are not returned: a function literal's body belongs to its own
// function and is analyzed as its own owner.
func childStatementLists(stmt ast.Stmt) [][]ast.Stmt {
	switch s := stmt.(type) {
	case *ast.BlockStmt:
		return [][]ast.Stmt{s.List}
	case *ast.IfStmt:
		lists := [][]ast.Stmt{s.Body.List}
		switch e := s.Else.(type) {
		case *ast.BlockStmt:
			lists = append(lists, e.List)
		case *ast.IfStmt:
			lists = append(lists, childStatementLists(e)...)
		}
		return lists
	case *ast.ForStmt:
		return [][]ast.Stmt{s.Body.List}
	case *ast.RangeStmt:
		return [][]ast.Stmt{s.Body.List}
	case *ast.SwitchStmt:
		if s.Body != nil {
			return [][]ast.Stmt{s.Body.List}
		}
	case *ast.TypeSwitchStmt:
		if s.Body != nil {
			return [][]ast.Stmt{s.Body.List}
		}
	case *ast.SelectStmt:
		if s.Body != nil {
			return [][]ast.Stmt{s.Body.List}
		}
	case *ast.CaseClause:
		return [][]ast.Stmt{s.Body}
	case *ast.CommClause:
		return [][]ast.Stmt{s.Body}
	case *ast.LabeledStmt:
		return childStatementLists(s.Stmt)
	}
	return nil
}

// launchList returns the innermost statement list holding the launch statement,
// together with that statement's index.
func launchList(all []*stmtList, launchPos token.Pos) (*stmtList, int) {
	var best *stmtList
	bestIdx := -1
	for _, list := range all {
		for i, stmt := range list.stmts {
			if stmt.Pos() <= launchPos && launchPos < stmt.End() {
				if best == nil || list.depth > best.depth {
					best, bestIdx = list, i
				}
			}
		}
	}
	return best, bestIdx
}

// joinDominates reports whether every path from the statement after the launch
// to a return of the owning function executes the join described by isJoin.
func joinDominates(all []*stmtList, launchPos token.Pos, isJoin func(ast.Node) bool) bool {
	list, idx := launchList(all, launchPos)
	if list == nil {
		return false
	}
	flow := listFlow(list.stmts, idx+1, list.ownerBody, isJoin)
	for flow == flowContinues && list.parent != nil {
		parent := list.parent
		flow = listFlow(parent.stmts, list.parentIdx+1, parent.ownerBody, isJoin)
		list = parent
	}
	return flow == flowJoined
}

// listFlow classifies statements[i:] of one statement list.
func listFlow(stmts []ast.Stmt, i int, ownerBody bool, isJoin func(ast.Node) bool) joinFlow {
	for ; i < len(stmts); i++ {
		joined, escaped := statementFlow(stmts[i], isJoin)
		if escaped {
			return flowEscaped
		}
		if joined {
			return flowJoined
		}
	}
	if ownerBody {
		// Falling off the end of the owner body is a return without the join.
		return flowEscaped
	}
	return flowContinues
}

// statementFlow reports whether every path through stmt executes the join, and
// whether some path returns from the owning function without it.
func statementFlow(stmt ast.Stmt, isJoin func(ast.Node) bool) (joined, escaped bool) {
	if executesJoinUnconditionally(stmt, isJoin) {
		return true, false
	}
	switch s := stmt.(type) {
	case *ast.ReturnStmt:
		return false, true
	case *ast.BranchStmt:
		// break/continue without a label stay inside the enclosing loop or
		// clause and reach the join afterwards. A labeled branch, or a goto,
		// can leave the region the join is checked over.
		if s.Label != nil || s.Tok == token.GOTO {
			return false, true
		}
		return false, false
	case *ast.ExprStmt:
		if call, ok := s.X.(*ast.CallExpr); ok {
			if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "panic" {
				return false, true
			}
		}
		return false, false
	case *ast.BlockStmt:
		return flowToPair(listFlow(s.List, 0, false, isJoin))
	case *ast.IfStmt:
		thenFlow := listFlow(s.Body.List, 0, false, isJoin)
		elseFlow := flowContinues
		switch e := s.Else.(type) {
		case *ast.BlockStmt:
			elseFlow = listFlow(e.List, 0, false, isJoin)
		case *ast.IfStmt:
			elseFlow = statementFlowAsFlow(e, isJoin)
		}
		return combineFlows(thenFlow, elseFlow)
	case *ast.ForStmt:
		return loopFlow(s.Body.List, isJoin)
	case *ast.RangeStmt:
		return loopFlow(s.Body.List, isJoin)
	case *ast.SwitchStmt:
		return switchFlow(s.Body, isJoin)
	case *ast.TypeSwitchStmt:
		return switchFlow(s.Body, isJoin)
	case *ast.SelectStmt:
		return selectFlow(s, isJoin)
	case *ast.LabeledStmt:
		return statementFlow(s.Stmt, isJoin)
	}
	return false, false
}

// statementFlowAsFlow adapts statementFlow to the flow return type used when
// combining branches.
func statementFlowAsFlow(stmt ast.Stmt, isJoin func(ast.Node) bool) joinFlow {
	joined, escaped := statementFlow(stmt, isJoin)
	switch {
	case escaped:
		return flowEscaped
	case joined:
		return flowJoined
	default:
		return flowContinues
	}
}

// flowToPair converts a flow into the (joined, escaped) pair for a statement
// that is exactly one nested block.
func flowToPair(flow joinFlow) (bool, bool) {
	switch flow {
	case flowJoined:
		return true, false
	case flowEscaped:
		return false, true
	default:
		return false, false
	}
}

// combineFlows merges the flows of an if statement's branches. An escape on
// either branch escapes; a join is claimed only when every branch joins, since
// a branch that merely continues will reach whatever follows the statement.
func combineFlows(flows ...joinFlow) (bool, bool) {
	allJoined := len(flows) > 0
	for _, flow := range flows {
		if flow == flowEscaped {
			return false, true
		}
		if flow != flowJoined {
			allJoined = false
		}
	}
	return allJoined, false
}

// loopFlow handles for and range bodies. A loop may run zero times and may
// break, so it never proves the join on its own; a body that returns without
// the join is an escape.
func loopFlow(body []ast.Stmt, isJoin func(ast.Node) bool) (bool, bool) {
	return false, listFlow(body, 0, false, isJoin) == flowEscaped
}

// switchFlow handles switch and type-switch bodies. A switch with a default
// clause and every clause joining is a join; without a default the subject can
// match nothing and fall through, so it only ever continues.
func switchFlow(body *ast.BlockStmt, isJoin func(ast.Node) bool) (bool, bool) {
	if body == nil {
		return false, false
	}
	hasDefault := false
	allJoined := true
	for _, clause := range body.List {
		cc, ok := clause.(*ast.CaseClause)
		if !ok {
			allJoined = false
			continue
		}
		if cc.List == nil {
			hasDefault = true
		}
		switch listFlow(cc.Body, 0, false, isJoin) {
		case flowEscaped:
			return false, true
		case flowJoined:
		default:
			allJoined = false
		}
	}
	return hasDefault && allJoined, false
}

// selectFlow handles select statements. A select always runs exactly one of its
// clauses, so every clause joining is a join; a clause that merely continues,
// or that escapes, is not.
func selectFlow(stmt *ast.SelectStmt, isJoin func(ast.Node) bool) (bool, bool) {
	if stmt.Body == nil || len(stmt.Body.List) == 0 {
		return false, false
	}
	allJoined := true
	for _, clause := range stmt.Body.List {
		cc, ok := clause.(*ast.CommClause)
		if !ok {
			allJoined = false
			continue
		}
		if clauseCommIsJoin(cc.Comm, isJoin) {
			// `case <-done:` is the join itself: the clause runs only when the
			// receive completes.
			continue
		}
		switch listFlow(cc.Body, 0, false, isJoin) {
		case flowEscaped:
			return false, true
		case flowJoined:
		default:
			allJoined = false
		}
	}
	return allJoined, false
}

// clauseCommIsJoin reports whether a select clause's own communication is the
// join: `case <-done:` or `case res := <-done:`.
func clauseCommIsJoin(comm ast.Stmt, isJoin func(ast.Node) bool) bool {
	switch c := comm.(type) {
	case nil:
		return false
	case *ast.ExprStmt:
		return isJoin(c.X)
	case *ast.AssignStmt:
		for _, rhs := range c.Rhs {
			if isJoin(rhs) {
				return true
			}
		}
		return false
	default:
		return isJoin(c)
	}
}

// executesJoinUnconditionally reports whether node executes the join whenever
// node itself executes: the join is not inside a closure, a conditional, a
// loop, a deferred call, or a launched goroutine.
func executesJoinUnconditionally(node ast.Node, isJoin func(ast.Node) bool) bool {
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		if n == nil {
			return true
		}
		if isJoin(n) {
			found = true
			return false
		}
		switch n.(type) {
		case *ast.FuncLit, *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt,
			*ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt,
			*ast.DeferStmt, *ast.GoStmt:
			return false
		}
		return true
	})
	return found
}

// ---------------------------------------------------------------------------
// The two recognized join shapes
// ---------------------------------------------------------------------------

// goLaunchJoined reports whether the goroutine started at launchPos is joined
// before the enclosing function can return: for every path from the launch to a
// return, the join runs first.
//
// Two shapes are recognized, and nothing else:
//
//  1. sync.WaitGroup — an Add on the group runs unconditionally in the same
//     statement list before the launch, the launched function calls Done on it,
//     and a Wait on the same group dominates every return path.
//  2. A channel the launched function closes after its last send: the channel
//     is unbuffered and declared before the launch, the goroutine's only
//     operations on it are sends followed by a deferred or final close (and
//     never a receive), and the owner drains it after the launch on every
//     return path. When the goroutine sends on the channel, only a `for range`
//     drain counts, because a single receive leaves the remaining sends
//     blocked; a close-only goroutine is proven finished by any receive.
//
// Everything else — a named function launch (whose body is not visible here), a
// Wait inside a conditional or a never-called closure, a launch with a return
// before the Wait, a channel whose sends outnumber the receives, a sent-on but
// unbuffered channel never drained — is reported as unjoined rather than
// assumed joined.
func goLaunchJoined(owner *ast.BlockStmt, launchPos token.Pos, launched *ast.BlockStmt) (bool, string) {
	if owner == nil {
		return false, ""
	}
	lists := buildStmtLists(owner)

	if joined, evidence := waitGroupJoined(lists, launchPos, launched); joined {
		return true, evidence
	}
	if joined, evidence := channelJoined(lists, owner, launchPos, launched); joined {
		return true, evidence
	}
	return false, ""
}

// waitGroupJoined implements join shape 1.
func waitGroupJoined(lists []*stmtList, launchPos token.Pos, launched *ast.BlockStmt) (bool, string) {
	if launched == nil {
		return false, ""
	}
	list, idx := launchList(lists, launchPos)
	if list == nil {
		return false, ""
	}
	name := addBeforeLaunch(list, idx)
	if name == "" {
		return false, ""
	}
	if !bodyCallsMethod(launched, name, "Done") {
		return false, ""
	}
	if !joinDominates(lists, launchPos, func(n ast.Node) bool {
		return isMethodCall(n, name, "Wait")
	}) {
		return false, ""
	}
	return true, fmt.Sprintf("WaitGroup %s: Add runs before the launch, the goroutine calls Done, and a Wait on %s runs on every return path after it", name, name)
}

// addBeforeLaunch returns the name of a WaitGroup whose Add runs
// unconditionally before the launch inside the launch's own statement list.
// The Add must be in that list: an Add in an enclosing block cannot be shown to
// cover this launch, so those shapes are reported as unjoined.
func addBeforeLaunch(list *stmtList, idx int) string {
	for i := 0; i < idx; i++ {
		if name := unconditionalAddTarget(list.stmts[i]); name != "" {
			return name
		}
	}
	return ""
}

// unconditionalAddTarget returns the receiver of an `.Add(` call that executes
// whenever the statement executes.
func unconditionalAddTarget(stmt ast.Stmt) string {
	found := ""
	ast.Inspect(stmt, func(n ast.Node) bool {
		if found != "" {
			return false
		}
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Add" {
				found = exprText(sel.X)
				return false
			}
		}
		switch n.(type) {
		case *ast.FuncLit, *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt,
			*ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt,
			*ast.DeferStmt, *ast.GoStmt:
			return false
		}
		return true
	})
	return found
}

// channelJoined implements join shape 2.
func channelJoined(lists []*stmtList, owner *ast.BlockStmt, launchPos token.Pos, launched *ast.BlockStmt) (bool, string) {
	if launched == nil {
		return false, ""
	}
	for _, name := range unbufferedChannelsBefore(owner, launchPos) {
		if !bodyClosesChannel(launched, name) {
			continue
		}
		if bodyReceivesOn(launched, name) {
			continue
		}
		sends := bodySendsOn(launched, name)
		isJoin := func(n ast.Node) bool {
			if receive, ok := n.(*ast.UnaryExpr); ok && receive.Op == token.ARROW {
				return exprText(receive.X) == name
			}
			if rng, ok := n.(*ast.RangeStmt); ok {
				if _, ok := rng.X.(*ast.Ident); ok && exprText(rng.X) == name {
					return true
				}
			}
			return false
		}
		if sends {
			// A range drain consumes every send up to the close; a single
			// receive would leave the remaining sends blocked, so only the drain
			// proves the goroutine finished.
			isJoin = func(n ast.Node) bool {
				rng, ok := n.(*ast.RangeStmt)
				return ok && exprText(rng.X) == name
			}
		}
		if !joinDominates(lists, launchPos, isJoin) {
			continue
		}
		data := "a `for range` drain on every return path after the launch consumes every send it makes"
		if !sends {
			data = "a receive on every return path after the launch proves the close ran"
		}
		return true, fmt.Sprintf("channel %s (unbuffered): declared before the launch, closed by the goroutine as its last action, and %s", name, data)
	}
	return false, ""
}

// unbufferedChannelsBefore returns the names of unbuffered channels declared
// with make(chan T) before launchPos in the owner body.
func unbufferedChannelsBefore(owner *ast.BlockStmt, launchPos token.Pos) []string {
	var names []string
	ast.Inspect(owner, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for i, rhs := range assign.Rhs {
			if i >= len(assign.Lhs) || !isUnbufferedMakeChan(rhs) {
				continue
			}
			id, ok := assign.Lhs[i].(*ast.Ident)
			if ok && assign.Pos() < launchPos {
				names = append(names, id.Name)
			}
		}
		return true
	})
	return names
}

// bodyClosesChannel reports whether body closes name as its last action: a
// deferred close, or a close that is the last top-level statement. A close that
// happens before other work proves nothing about the goroutine finishing.
func bodyClosesChannel(body *ast.BlockStmt, name string) bool {
	if body == nil {
		return false
	}
	for _, stmt := range body.List {
		if deferStmt, ok := stmt.(*ast.DeferStmt); ok {
			if call, ok := deferStmt.Call.Fun.(*ast.Ident); ok && call.Name == "close" &&
				len(deferStmt.Call.Args) == 1 && exprText(deferStmt.Call.Args[0]) == name {
				return true
			}
		}
	}
	if len(body.List) == 0 {
		return false
	}
	last, ok := body.List[len(body.List)-1].(*ast.ExprStmt)
	if !ok {
		return false
	}
	call, ok := last.X.(*ast.CallExpr)
	if !ok {
		return false
	}
	id, ok := call.Fun.(*ast.Ident)
	return ok && id.Name == "close" && len(call.Args) == 1 && exprText(call.Args[0]) == name
}

// bodyReceivesOn reports whether body receives from name.
func bodyReceivesOn(body *ast.BlockStmt, name string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		receive, ok := n.(*ast.UnaryExpr)
		if ok && receive.Op == token.ARROW && exprText(receive.X) == name {
			found = true
		}
		return true
	})
	return found
}

// bodySendsOn reports whether body sends on name.
func bodySendsOn(body *ast.BlockStmt, name string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		send, ok := n.(*ast.SendStmt)
		if ok && exprText(send.Chan) == name {
			found = true
		}
		return true
	})
	return found
}

// errgroupLaunchJoined reports whether an errgroup-style launch is joined: a
// Wait on the same group runs on every return path after the launch. errgroup's
// Wait is the only join it offers, and it joins every launch on that group.
func errgroupLaunchJoined(owner *ast.BlockStmt, launchPos token.Pos, recv string) (bool, string) {
	if owner == nil || recv == "" {
		return false, ""
	}
	lists := buildStmtLists(owner)
	if !joinDominates(lists, launchPos, func(n ast.Node) bool {
		return isMethodCall(n, recv, "Wait")
	}) {
		return false, ""
	}
	return true, fmt.Sprintf("errgroup %s: a Wait on the group runs on every return path after the launch", recv)
}

// bodyCallsMethod reports whether body calls <name>.<method>(...).
func bodyCallsMethod(body *ast.BlockStmt, name, method string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if isMethodCall(n, name, method) {
			found = true
		}
		return !found
	})
	return found
}

// launchedFuncBody returns the body of a `go func(){...}()` launch, or nil when
// the launched callable is a named function (whose body cannot be inspected
// here, so the launch is treated as unjoined).
func launchedFuncBody(goStmt *ast.GoStmt) *ast.BlockStmt {
	call, ok := goStmt.Call.Fun.(*ast.FuncLit)
	if !ok {
		return nil
	}
	return call.Body
}

// isUnbufferedMakeChan reports whether e is make(chan ...) with no buffer. A
// buffered channel's receive can complete before the goroutine finished, so only
// unbuffered channels can carry the close-signal join.
func isUnbufferedMakeChan(e ast.Expr) bool {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return false
	}
	id, ok := call.Fun.(*ast.Ident)
	if !ok || id.Name != "make" || len(call.Args) != 1 {
		return false
	}
	_, ok = call.Args[0].(*ast.ChanType)
	return ok
}

// exprText renders a short, stable source form for an expression, used only for
// naming (WaitGroup and channel identities) in join evidence.
func exprText(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		return exprText(x.X) + "." + x.Sel.Name
	case *ast.StarExpr:
		return "*" + exprText(x.X)
	case *ast.IndexExpr:
		return exprText(x.X) + "[...]"
	default:
		return "?"
	}
}

// launchKey is the allowlist key for one launch.
func launchKey(rel string, line int) string {
	return fmt.Sprintf("%s:%d", rel, line)
}

// describeLaunch renders a finding as a one-line, reviewer-readable form.
func describeLaunch(site goLaunchSite) string {
	where := site.Rel
	if site.Func != "" {
		where = fmt.Sprintf("%s (%s)", where, site.Func)
	}
	note := ""
	if site.Evidence != "" {
		note = " — " + site.Evidence
	}
	return fmt.Sprintf("%s:%d: [%s] %s%s", where, site.Line, site.Kind, site.Text, note)
}

// sourceLine returns the 1-based line from src, or "" when it is out of range.
func sourceLine(src []byte, line int) string {
	if line < 1 {
		return ""
	}
	lines := strings.Split(string(src), "\n")
	if line > len(lines) {
		return ""
	}
	return lines[line-1]
}
