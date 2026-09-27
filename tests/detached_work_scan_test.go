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
// repository root) whose repository-relative slash path satisfies include.
// Missing roots are skipped so a language surface that does not exist cannot
// make the guard fail; the caller is responsible for proving it scanned
// something.
func walkDetachedWorkSources(t *testing.T, root string, dirs []string, include func(rel string) bool, visit func(abs, rel string) error) {
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
			return visit(path, rel)
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

// goLaunchSite is one `go` statement located by the AST scanner.
type goLaunchSite struct {
	Rel      string
	Line     int
	Text     string
	Func     string
	Joined   bool
	Evidence string
}

// funcBody pairs a function's body with the declaration that owns it, so a
// launch can be attributed and named.
type funcBody struct {
	body *ast.BlockStmt
	decl ast.Node
}

// scanGoSource parses Go source with go/ast — never a regex — and returns every
// goroutine launch it contains, classified as joined or not. Parsing means a
// launch is recognised wherever it appears, including the `; go f()` form on a
// line that also holds other statements, which a line-anchored pattern misses.
// Both launch mechanisms are found: `go` statements and errgroup-style
// `<g>.Go(func(){...})` calls, because errgroup hides a goroutine behind an
// ordinary method call and is named as a join primitive in the same policy.
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
	record := func(node ast.Node, launched *ast.BlockStmt, recv string, errgroup bool) {
		owner := innermostFuncBody(bodies, node.Pos(), node.End())
		pos := fset.Position(node.Pos())
		site := goLaunchSite{
			Rel:  rel,
			Line: pos.Line,
			Text: strings.TrimSpace(sourceLine(src, pos.Line)),
		}
		if owner != nil {
			site.Func = funcName(owner.decl)
			if errgroup {
				site.Joined, site.Evidence = errgroupLaunchJoined(fset, owner.body, node.Pos(), launched, recv)
			} else {
				site.Joined, site.Evidence = goLaunchJoined(fset, owner.body, node.Pos(), launched)
			}
		}
		sites = append(sites, site)
	}

	ast.Inspect(file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.GoStmt:
			record(x, launchedFuncBody(x), "", false)
		case *ast.CallExpr:
			if launched, recv, ok := errgroupFuncLit(x); ok {
				record(x, launched, recv, true)
			}
		}
		return true
	})
	return sites, nil
}

// errgroupFuncLit recognises an errgroup-style launch: a call to a method named
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

// goLaunchJoined reports whether the goroutine started by goStmt provably
// finishes before the enclosing function returns. It recognises the two idioms
// this codebase uses to join: a sync.WaitGroup (Add before the launch, Wait
// after it, and Done inside the launched function) and a channel the enclosing
// function drains after the launch whose writes happen inside the launched
// function. Anything else is unproven and therefore reported as detached.
//
// The analysis deliberately stops at the launch boundary: it never descends
// into the launched function when hunting for the owner's Add/Wait/receive
// signals, so a WaitGroup or channel that only exists inside the goroutine — or
// a Wait that runs before the launch — cannot masquerade as a join. That is why
// MemoryMonitor.Start in pkg/protection (which calls mm.wg.Wait() *before*
// mm.wg.Add(1) and launches the next monitor) is not recognised as joined.
func goLaunchJoined(fset *token.FileSet, owner *ast.BlockStmt, launchPos token.Pos, launched *ast.BlockStmt) (bool, string) {
	goOffset := fset.Position(launchPos).Offset

	addBefore := map[string]bool{}
	waitAfter := map[string]bool{}
	inspectOutsideLaunches(owner, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		name := exprText(sel.X)
		switch sel.Sel.Name {
		case "Add":
			if fset.Position(call.Pos()).Offset < goOffset {
				addBefore[name] = true
			}
		case "Wait":
			if fset.Position(call.Pos()).Offset > goOffset {
				waitAfter[name] = true
			}
		}
		return true
	})

	for name := range addBefore {
		if launched == nil || !waitAfter[name] {
			continue
		}
		if bodyCallsMethod(launched, name, "Done") {
			return true, fmt.Sprintf("WaitGroup %s: Add before the launch, Wait after it, Done inside the goroutine", name)
		}
	}

	declaredChans := map[string]bool{}
	ast.Inspect(owner, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.AssignStmt:
			for i, rhs := range x.Rhs {
				if i >= len(x.Lhs) || !isMakeChan(rhs) {
					continue
				}
				id, ok := x.Lhs[i].(*ast.Ident)
				if ok && fset.Position(x.Pos()).Offset < goOffset {
					declaredChans[id.Name] = true
				}
			}
		case *ast.DeclStmt:
			gd, ok := x.Decl.(*ast.GenDecl)
			if !ok {
				return true
			}
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok || !isChanType(vs.Type) {
					continue
				}
				for _, name := range vs.Names {
					if fset.Position(spec.Pos()).Offset < goOffset {
						declaredChans[name.Name] = true
					}
				}
			}
		}
		return true
	})

	recvAfter := map[string]bool{}
	inspectOutsideLaunches(owner, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.UnaryExpr:
			if x.Op != token.ARROW {
				return true
			}
			if id, ok := x.X.(*ast.Ident); ok && fset.Position(x.Pos()).Offset > goOffset {
				recvAfter[id.Name] = true
			}
		case *ast.RangeStmt:
			if id, ok := x.X.(*ast.Ident); ok && fset.Position(x.Pos()).Offset > goOffset {
				recvAfter[id.Name] = true
			}
		}
		return true
	})

	for name := range declaredChans {
		if launched == nil || !recvAfter[name] {
			continue
		}
		if bodySendsOrClosesOn(launched, name) {
			return true, fmt.Sprintf("channel %s: declared before the launch, drained after it, written by the goroutine", name)
		}
	}

	return false, ""
}

// errgroupLaunchJoined reports whether an errgroup-style launch is joined by a
// Wait on the same group later in the enclosing function. errgroup's Wait is
// the only join it offers and it joins every launch on that group, so a Wait
// positioned after this launch proves this goroutine is awaited too.
func errgroupLaunchJoined(fset *token.FileSet, owner *ast.BlockStmt, launchPos token.Pos, launched *ast.BlockStmt, recv string) (bool, string) {
	if launched == nil || recv == "" {
		return false, ""
	}
	launchOffset := fset.Position(launchPos).Offset
	waitAfter := false
	inspectOutsideLaunches(owner, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Wait" {
			return true
		}
		if exprText(sel.X) == recv && fset.Position(call.Pos()).Offset > launchOffset {
			waitAfter = true
		}
		return true
	})
	if waitAfter {
		return true, fmt.Sprintf("errgroup %s: Wait after the launch joins every goroutine started on the group", recv)
	}
	return false, ""
}

// inspectOutsideLaunches walks n but does not descend into the body of a
// launched goroutine, so the owner's join signals are never confused with the
// launched function's own internals.
func inspectOutsideLaunches(n ast.Node, fn func(ast.Node) bool) {
	ast.Inspect(n, func(node ast.Node) bool {
		if node == nil {
			return true
		}
		if _, ok := node.(*ast.GoStmt); ok {
			return false
		}
		if call, ok := node.(*ast.CallExpr); ok {
			if _, _, isLaunch := errgroupFuncLit(call); isLaunch {
				return false
			}
		}
		return fn(node)
	})
}

// launchedFuncBody returns the body of a `go func(){...}()` launch, or nil when
// the launched callable is a named function (whose body cannot be inspected
// here, so the launch is treated as unproven).
func launchedFuncBody(goStmt *ast.GoStmt) *ast.BlockStmt {
	call, ok := goStmt.Call.Fun.(*ast.FuncLit)
	if !ok {
		return nil
	}
	return call.Body
}

// bodyCallsMethod reports whether body calls <name>.<method>(...).
func bodyCallsMethod(body *ast.BlockStmt, name, method string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != method {
			return true
		}
		if exprText(sel.X) == name {
			found = true
		}
		return true
	})
	return found
}

// bodySendsOrClosesOn reports whether body writes to channel name (a send) or
// closes it, which is the goroutine-side half of a channel-drain join.
func bodySendsOrClosesOn(body *ast.BlockStmt, name string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.SendStmt:
			if exprText(x.Chan) == name {
				found = true
			}
		case *ast.CallExpr:
			id, ok := x.Fun.(*ast.Ident)
			if ok && id.Name == "close" && len(x.Args) == 1 && exprText(x.Args[0]) == name {
				found = true
			}
		}
		return true
	})
	return found
}

// isMakeChan reports whether e is make(chan ...).
func isMakeChan(e ast.Expr) bool {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return false
	}
	id, ok := call.Fun.(*ast.Ident)
	if !ok || id.Name != "make" || len(call.Args) == 0 {
		return false
	}
	_, ok = call.Args[0].(*ast.ChanType)
	return ok
}

// isChanType reports whether t is a channel type.
func isChanType(t ast.Expr) bool {
	_, ok := t.(*ast.ChanType)
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

// describeLaunch renders a launch as a one-line, reviewer-readable finding.
func describeLaunch(site goLaunchSite) string {
	where := site.Rel
	if site.Func != "" {
		where = fmt.Sprintf("%s (%s)", where, site.Func)
	}
	return fmt.Sprintf("%s:%d: %s", where, site.Line, site.Text)
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
