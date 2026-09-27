package tests

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// pythonDetachedWorkAllowlist lists Python launches that may run detached. Keys
// are "<relative path>:<line>". Every entry is reported in the PR body; the
// guard fails when a key stops matching a finding. It is empty: no Python
// source we ship needs an exception.
var pythonDetachedWorkAllowlist = launchAllowlist{}

// pyFinding is one detached-work construct found in a Python source.
type pyFinding struct {
	Text string
	Rule string
	Line int
}

// pyTarget matches the receiver or assignment target of a launch: a name with
// optional attribute and subscript parts, so a thread held on `self.worker`,
// `obj.items`, or `pool["a"]` is tracked exactly like a plain local name.
const pyTarget = `(?:[A-Za-z_]\w*)(?:\s*\.\s*[A-Za-z_]\w*|\s*\[[^\]]*\])*`

var (
	// A thread or process started and never joined outlives the caller.
	pyThreadCtor   = regexp.MustCompile(`\b(?:threading\.(?:Thread|Timer)|multiprocessing\.Process)\s*\(`)
	pyThreadAssign = regexp.MustCompile(`^\s*(` + pyTarget + `)\s*(?::[^=]*)?=\s*(?:threading\.(?:Thread|Timer)|multiprocessing\.Process)\s*\(`)
	pyStart        = regexp.MustCompile(`^\s*(` + pyTarget + `)\s*\.\s*start\s*\(\s*\)`)
	pyJoin         = regexp.MustCompile(`\b(` + pyTarget + `)\s*\.\s*join\s*\(`)

	// A thread created inline in a comprehension has no name to join later.
	pyComprehension = regexp.MustCompile(`\bfor\b[^:]*\bin\b`)

	// A daemon thread is explicitly declared as one not to wait for; it is
	// killed when the process exits and no caller holds a handle to join.
	pyDaemonKwarg  = regexp.MustCompile(`\bdaemon\s*=\s*True\b`)
	pyDaemonAssign = regexp.MustCompile(`^\s*` + pyTarget + `\s*\.\s*daemon\s*=\s*True`)

	// A task created with asyncio and never awaited or gathered is detached.
	pyAsyncioTask = regexp.MustCompile(`\basyncio\.(?:create_task|ensure_future)\s*\(`)
	pyTaskAssign  = regexp.MustCompile(`^\s*(` + pyTarget + `)\s*(?::[^=]*)?=\s*(?:await\s+)?.*?\basyncio\.(?:create_task|ensure_future)\s*\(`)
	pyGather      = regexp.MustCompile(`\basyncio\.(?:gather|wait)\s*\(`)

	// The join forms an awaitable target can carry. Cancellation is not one: it
	// requests a stop but does not wait for the task to reach it, so a task that
	// is only canceled can still be running when the caller returns.
	pyAwaitTarget = regexp.MustCompile(`\bawait\s+(` + pyTarget + `)\b`)
	pyGatherCall  = regexp.MustCompile(`\b(?:gather|wait)\s*\(\s*\*?\s*(` + pyTarget + `)\b`)

	// Offloading to a worker thread is only joined by awaiting the result.
	pyToThread    = regexp.MustCompile(`\basyncio\.to_thread\s*\(`)
	pyRunInExec   = regexp.MustCompile(`\brun_in_executor\s*\(`)
	pyOffloadAsgn = regexp.MustCompile(`^\s*(` + pyTarget + `)\s*(?::[^=]*)?=\s*.*?\b(?:asyncio\.to_thread|run_in_executor)\s*\(`)

	// An executor submit outside its `with` block is not joined by the context
	// manager's exit.
	pyWithExecutor = regexp.MustCompile(`^(\s*)with\s+.*(?:ThreadPoolExecutor|ProcessPoolExecutor)\s*[(.]`)
	pySubmit       = regexp.MustCompile(`\.submit\s*\(`)

	// A statement that can leave the enclosing function or loop before a later
	// join runs: a return, raise, break, or continue, including one written as
	// the body of a single-line compound statement (`if cond: return`).
	pyExitStmt = regexp.MustCompile(`^(?:return|raise|break|continue)\b|^(?:if|elif|else|for|while|try|except|finally|with|match|case)\b.*:\s*(?:return|raise|break|continue)\b`)
)

// pyFrame is one open Python block: its indentation, the line that opened it,
// and whether that line is a function definition.
type pyFrame struct {
	indent int
	line   int
	isDef  bool
}

// pyStructure is the indentation-derived block structure of a Python source. It
// exists so a join can be required to dominate every return path of the
// function that launched the work, mirroring the Go AST walk. No Python parser
// is available to this Go test, so the structure is read from indentation: a
// block opens with a `def`/`async def` or a keyword statement whose header ends
// in `:`, and closes at the next line indented at or below its opener. The
// reading is deliberately conservative — a shape it cannot place is treated as
// un-joined rather than assumed joined.
type pyStructure struct {
	indent    []int
	defStack  [][]int
	enclosers [][]int
	exit      []bool
}

// pyBlockKeywords open a block when their header ends in `:`.
var pyBlockKeywords = []string{"if", "elif", "else", "for", "while", "try", "except", "finally", "with", "class", "match", "case"}

// buildPyStructure reads the block structure of src line by line.
func buildPyStructure(lines []string) pyStructure {
	st := pyStructure{
		indent:    make([]int, len(lines)),
		defStack:  make([][]int, len(lines)),
		enclosers: make([][]int, len(lines)),
		exit:      make([]bool, len(lines)),
	}
	var stack []pyFrame
	depth := 0
	record := func(i int) {
		encl := make([]int, 0, len(stack))
		var defs []int
		for _, f := range stack {
			encl = append(encl, f.line)
			if f.isDef {
				defs = append(defs, f.line)
			}
		}
		st.enclosers[i] = encl
		st.defStack[i] = defs
	}
	for i, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		startDepth := depth
		depth += pyCodeDelta(raw)
		st.indent[i] = pythonIndent(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if startDepth > 0 {
			// A continuation line inside brackets belongs to the block the
			// statement it continues sits in; it opens and closes nothing.
			record(i)
			continue
		}
		for len(stack) > 0 && stack[len(stack)-1].indent >= st.indent[i] {
			stack = stack[:len(stack)-1]
		}
		record(i)
		st.exit[i] = pyExitStmt.MatchString(trimmed)
		if isDef, opens := pyBlockOpener(trimmed); opens {
			stack = append(stack, pyFrame{indent: st.indent[i], line: i + 1, isDef: isDef})
		}
	}
	return st
}

// pyBlockOpener reports whether a statement opens a block, and whether that
// block is a function body.
func pyBlockOpener(trimmed string) (isDef, opens bool) {
	if headerIsDef(trimmed) {
		return true, true
	}
	head := stripPyComment(trimmed)
	if !strings.HasSuffix(head, ":") {
		return false, false
	}
	word := head
	if idx := strings.IndexAny(word, " \t(:"); idx >= 0 {
		word = word[:idx]
	}
	for _, kw := range pyBlockKeywords {
		if word == kw {
			return false, true
		}
	}
	return false, false
}

// headerIsDef reports whether a header begins a def or async def, including one
// whose signature is spread over several lines.
func headerIsDef(header string) bool {
	rest := header
	if strings.HasPrefix(rest, "async") {
		rest = strings.TrimSpace(strings.TrimPrefix(rest, "async"))
	}
	return strings.HasPrefix(rest, "def ") || strings.HasPrefix(rest, "def(")
}

// stripPyComment removes a trailing comment, ignoring a `#` inside a simple
// string literal.
func stripPyComment(line string) string {
	var quote byte
	for i := 0; i < len(line); i++ {
		c := line[i]
		if quote != 0 {
			if c == '\\' {
				i++
				continue
			}
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
		case '#':
			return line[:i]
		}
	}
	return line
}

// pyCodeDelta is the net opening-bracket count of a line once comments and
// simple string contents are removed, so a bracket in prose or a literal cannot
// shift where a block starts.
func pyCodeDelta(line string) int {
	code := stripPyComment(line)
	delta := 0
	var quote byte
	for i := 0; i < len(code); i++ {
		c := code[i]
		if quote != 0 {
			if c == '\\' {
				i++
				continue
			}
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
		case '(', '[', '{':
			delta++
		case ')', ']', '}':
			delta--
		}
	}
	return delta
}

// pyDominates reports whether a join for the work launched at launchLine (a
// 1-based line number) runs on every path that returns from the launch's own
// function. The join must appear after the launch, must not sit inside a block
// the launch is not inside (so a join behind `if`, in a loop body, or in a
// nested function does not count), and no return, raise, break, or continue may
// be crossed first. That is the Python reading of the Go dominance walk.
func pyDominates(st pyStructure, lines []string, launchLine int, isJoin func(line int) bool) bool {
	start := launchLine - 1
	if start < 0 || start >= len(lines) {
		return false
	}
	baseDefs := st.defStack[start]
	baseEncl := st.enclosers[start]
	for j := start + 1; j < len(lines); j++ {
		if !intStackPrefix(baseDefs, st.defStack[j]) {
			return false
		}
		if intSubset(st.enclosers[j], baseEncl) && isJoin(j+1) {
			return true
		}
		if st.exit[j] && intStackEqual(baseDefs, st.defStack[j]) {
			return false
		}
	}
	return false
}

// pyKey normalizes a target so a spaced spelling and a compact one match.
func pyKey(target string) string {
	return strings.Join(strings.Fields(target), "")
}

// isPyCommentLine reports whether a whole line is blank or a comment.
func isPyCommentLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return trimmed == "" || strings.HasPrefix(trimmed, "#")
}

// pyJoinForThread reports whether a line joins the given thread target.
func pyJoinForThread(line, target string) bool {
	if isPyCommentLine(line) {
		return false
	}
	for _, m := range pyJoin.FindAllStringSubmatch(line, -1) {
		if pyKey(m[1]) == target {
			return true
		}
	}
	return false
}

// pyJoinForTask reports whether a line joins the given asyncio task: awaiting
// it, or gathering it. Canceling it is not a join, because cancellation is
// cooperative and the task can still be running when the caller returns.
func pyJoinForTask(line, target string) bool {
	if isPyCommentLine(line) {
		return false
	}
	if m := pyAwaitTarget.FindStringSubmatch(line); m != nil && pyKey(m[1]) == target {
		return true
	}
	if m := pyGatherCall.FindStringSubmatch(line); m != nil && pyKey(m[1]) == target {
		return true
	}
	return false
}

// pyJoinForOffload reports whether a line awaits the given offload result.
func pyJoinForOffload(line, target string) bool {
	if isPyCommentLine(line) {
		return false
	}
	m := pyAwaitTarget.FindStringSubmatch(line)
	return m != nil && pyKey(m[1]) == target
}

// pyDominatingJoin reports whether the join for the launch at launchLine
// dominates every return path of its function.
func pyDominatingJoin(st pyStructure, lines []string, launchLine int, target string, lineJoins func(string, string) bool) bool {
	return pyDominates(st, lines, launchLine, func(line int) bool {
		if line < 1 || line > len(lines) {
			return false
		}
		return lineJoins(lines[line-1], target)
	})
}

// pyOffloadJoined reports whether an offload assigned on the given line has a
// dominating await of the bound result.
func pyOffloadJoined(st pyStructure, lines []string, launchLine int, line string) bool {
	m := pyOffloadAsgn.FindStringSubmatch(line)
	if m == nil {
		return false
	}
	return pyDominatingJoin(st, lines, launchLine, pyKey(m[1]), pyJoinForOffload)
}

// scanPythonSource finds Python detached-work launches: threads or processes
// started without a dominating join (including threads created inline in a
// comprehension, daemon threads, and threads held on an attribute or a
// container element), asyncio tasks created without a dominating await or
// gather, thread offloads (asyncio.to_thread, loop.run_in_executor) that are
// never awaited, and executor submits that escape their `with` block. It is a
// deliberately conservative line-and-indentation scan, since no Python parser
// is available to this Go test; a launch whose join cannot be shown to dominate
// every return path is reported.
func scanPythonSource(src string) []pyFinding {
	lines := strings.Split(src, "\n")
	st := buildPyStructure(lines)
	var findings []pyFinding

	// Threads and processes are tracked by the target they are bound to, which
	// may be a local name, an attribute (self.worker), or a container element
	// (pool["a"]).
	threadNames := map[string]bool{}
	for _, line := range lines {
		if isPyCommentLine(line) {
			continue
		}
		if m := pyThreadAssign.FindStringSubmatch(line); m != nil {
			threadNames[pyKey(m[1])] = true
		}
	}
	joinedStarts := map[string]bool{}
	for i, line := range lines {
		if isPyCommentLine(line) {
			continue
		}
		m := pyStart.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		target := pyKey(m[1])
		if !threadNames[target] {
			continue
		}
		if pyDominatingJoin(st, lines, i+1, target, pyJoinForThread) {
			joinedStarts[nameLineKey(target, i+1)] = true
		}
	}

	// Indentation levels of the `with ...Executor(...)` blocks currently open.
	var executorBlocks []int

	// A thread or process constructed inside a bracketed expression that spans
	// several lines and contains a comprehension is a comprehension thread even
	// when the `for` clause sits on a different line from the constructor.
	regionComprehension := comprehensionRegions(lines)

	for i, line := range lines {
		lineNo := i + 1
		trimmed := strings.TrimSpace(line)
		record := func(rule string) {
			findings = append(findings, pyFinding{Line: lineNo, Text: trimmed, Rule: rule})
		}

		if isPyCommentLine(line) {
			continue
		}

		indent := pythonIndent(line)
		for len(executorBlocks) > 0 && indent <= executorBlocks[len(executorBlocks)-1] {
			executorBlocks = executorBlocks[:len(executorBlocks)-1]
		}
		if m := pyWithExecutor.FindStringSubmatch(line); m != nil {
			executorBlocks = append(executorBlocks, len(m[1]))
		}

		if pyThreadCtor.MatchString(line) && strings.Contains(line, ".start(") {
			// Inline `Thread(...).start()` discards the handle, so it can never
			// be joined.
			record("thread-start")
		} else if m := pyStart.FindStringSubmatch(line); m != nil && threadNames[pyKey(m[1])] {
			if !joinedStarts[nameLineKey(pyKey(m[1]), lineNo)] {
				record("thread-start")
			}
		}

		if pyThreadInComprehension(line) || (pyThreadCtor.MatchString(line) && regionComprehension[i]) {
			record("thread-comprehension")
		}

		if pyDaemonKwarg.MatchString(line) && pyThreadCtor.MatchString(line) {
			record("daemon-thread")
		} else if pyDaemonAssign.MatchString(line) {
			record("daemon-thread")
		}

		if pyAsyncioTask.MatchString(line) {
			recognized := strings.Contains(line, "await ") || pyGather.MatchString(line)
			if !recognized {
				if m := pyTaskAssign.FindStringSubmatch(line); m != nil {
					recognized = pyDominatingJoin(st, lines, lineNo, pyKey(m[1]), pyJoinForTask)
				}
			}
			if !recognized {
				record("asyncio-task")
			}
		}

		if pyToThread.MatchString(line) && !strings.Contains(line, "await ") {
			if !pyOffloadJoined(st, lines, lineNo, line) {
				record("asyncio-to-thread")
			}
		}
		if pyRunInExec.MatchString(line) && !strings.Contains(line, "await ") {
			if !pyOffloadJoined(st, lines, lineNo, line) {
				record("run-in-executor")
			}
		}

		if pySubmit.MatchString(line) && len(executorBlocks) == 0 {
			record("executor-submit")
		}
	}

	return findings
}

// pyThreadInComprehension reports whether the line creates a thread or process
// inside a comprehension: the constructor sits after an opening bracket that is
// still open on the same line, and a `for ... in` follows it.
func pyThreadInComprehension(line string) bool {
	loc := pyThreadCtor.FindStringIndex(line)
	if loc == nil {
		return false
	}
	bracket := strings.LastIndexAny(line[:loc[0]], "[{(")
	if bracket < 0 {
		return false
	}
	return pyComprehension.MatchString(line[loc[1]:])
}

// comprehensionRegions marks every line that sits inside a bracketed expression
// containing a comprehension, so a comprehension written across several lines
// is treated like a single-line one. Bracket counting is textual and can be
// thrown off by brackets inside strings and comments; that can only make the
// guard flag a region, never accept detached work, and no scanned surface
// constructs a thread inside a bracketed literal today.
func comprehensionRegions(lines []string) []bool {
	regions := make([]bool, len(lines))
	depth := 0
	start := -1
	for i, line := range lines {
		delta := pyBracketDelta(line)
		if start < 0 {
			if delta > 0 {
				depth = delta
				start = i
			}
			continue
		}
		depth += delta
		if depth > 0 {
			continue
		}
		if pyComprehension.MatchString(strings.Join(lines[start:i+1], "\n")) {
			for j := start; j <= i; j++ {
				regions[j] = true
			}
		}
		start = -1
		depth = 0
	}
	return regions
}

// pyBracketDelta is the net opening-bracket count of a line.
func pyBracketDelta(line string) int {
	delta := 0
	for _, r := range line {
		switch r {
		case '[', '{', '(':
			delta++
		case ']', '}', ')':
			delta--
		}
	}
	return delta
}

// nameLineKey builds the "<name>:<line>" key used for joined-start bookkeeping.
func nameLineKey(name string, line int) string {
	return name + ":" + strconv.Itoa(line)
}

// pythonIndent measures leading whitespace, counting a tab as eight columns so
// mixed indentation still nests correctly.
func pythonIndent(line string) int {
	width := 0
	for _, r := range line {
		switch r {
		case ' ':
			width++
		case '\t':
			width += 8
		default:
			return width
		}
	}
	return width
}

// TestPython_NoDetachedWork fails when a Python source we ship (py/src,
// py/examples, examples) or run (scripts, contract-test runners) launches work
// that is not joined before the caller returns.
func TestPython_NoDetachedWork(t *testing.T) {
	root := detachedWorkRepoRoot(t)
	reportAllowlist(t, pythonDetachedWorkAllowlist)

	scanned := 0
	scannedKeys := map[string]bool{}
	var problems []string

	include := func(rel string) bool {
		if !strings.HasSuffix(rel, ".py") {
			return false
		}
		base := rel[strings.LastIndex(rel, "/")+1:]
		return !strings.HasPrefix(base, "test_") && !strings.HasSuffix(base, "_test.py") && base != "conftest.py"
	}
	visit := func(rel string, src []byte) error {
		scanned++
		for _, finding := range scanPythonSource(string(src)) {
			key := launchKey(rel, finding.Line)
			scannedKeys[key] = true
			if pythonDetachedWorkAllowlist.allowlisted(rel, finding.Line) {
				continue
			}
			problems = append(problems, rel+":"+strconv.Itoa(finding.Line)+": ["+finding.Rule+"] "+finding.Text)
		}
		return nil
	}
	walkDetachedWorkSources(t, root, []string{"py/src", "py/examples", "examples", "scripts", "contract-tests/runners"}, include, visit)

	t.Logf("scanned %d Python source files", scanned)
	if scanned == 0 {
		t.Fatal("guard is vacuous: no Python source files were scanned")
	}
	pythonDetachedWorkAllowlist.checkAllowlistCoverage(t, scannedKeys)
	if len(problems) > 0 {
		sort.Strings(problems)
		t.Fatalf(
			"Python must not launch work that outlives the call that started it.\n"+
				"Join the thread/task, keep the executor submit inside its `with` block, or add the exact line to pythonDetachedWorkAllowlist with a justification:\n%s",
			strings.Join(problems, "\n"),
		)
	}
}

// TestPythonDetachedWorkDetectorIsNotVacuous proves the Python detector fires on
// each banned construct and stays quiet on the joined forms the repository is
// allowed to use.
func TestPythonDetachedWorkDetectorIsNotVacuous(t *testing.T) {
	flagged := []struct{ name, src, rule string }{
		{"thread started", "t = threading.Thread(target=work)\nt.start()\n", "thread-start"},
		{"thread inline", "threading.Thread(target=work).start()\n", "thread-start"},
		{"timer started", "t = threading.Timer(5.0, work)\nt.start()\n", "thread-start"},
		{"process started", "p = multiprocessing.Process(target=work)\np.start()\n", "thread-start"},
		{"annotated thread start", "t: threading.Thread = threading.Thread(target=work)\nt.start()\n", "thread-start"},
		{"thread comprehension", "threads = [threading.Thread(target=work) for _ in range(4)]\n", "thread-comprehension"},
		{"multiline thread comprehension", "threads = [\n    threading.Thread(target=work)\n    for _ in range(4)\n]\nfor t in threads:\n    t.start()\n", "thread-comprehension"},
		{"daemon thread inline", "threading.Thread(target=work, daemon=True).start()\n", "daemon-thread"},
		{"daemon thread named", "t = threading.Thread(target=work, daemon=True)\nt.start()\n", "daemon-thread"},
		{"daemon attribute", "t = threading.Thread(target=work)\nt.daemon = True\nt.start()\n", "daemon-thread"},
		{"create_task", "asyncio.create_task(work())\n", "asyncio-task"},
		{"ensure_future", "task = asyncio.ensure_future(work())\n", "asyncio-task"},
		{"canceled but not awaited", "task = asyncio.create_task(work())\ntask.cancel()\n", "asyncio-task"},
		{"to_thread not awaited", "result = asyncio.to_thread(compute, arg)\n", "asyncio-to-thread"},
		{"to_thread bare", "asyncio.to_thread(compute, arg)\n", "asyncio-to-thread"},
		{"run_in_executor not awaited", "future = loop.run_in_executor(pool, compute)\n", "run-in-executor"},
		{"submit outside with", "ex = ThreadPoolExecutor(max_workers=4)\nex.submit(work)\n", "executor-submit"},
		// A join that is present but does not dominate is not a join.
		{"conditional thread join", "def run(cond):\n    t = threading.Thread(target=work)\n    t.start()\n    if cond:\n        t.join()\n", "thread-start"},
		{"early return before join", "def run(abort):\n    t = threading.Thread(target=work)\n    t.start()\n    if abort:\n        return\n    t.join()\n", "thread-start"},
		{"single-line return before join", "def run(cond):\n    t = threading.Thread(target=work)\n    t.start()\n    if cond: return\n    t.join()\n", "thread-start"},
		{"thread joined in a nested function", "def run():\n    t = threading.Thread(target=work)\n    t.start()\n    def inner():\n        t.join()\n    inner()\n", "thread-start"},
		{"conditional task await", "async def run(cond):\n    task = asyncio.create_task(work())\n    if cond:\n        await task\n", "asyncio-task"},
		{"early return before await", "async def run(abort):\n    task = asyncio.create_task(work())\n    if abort:\n        return\n    await task\n", "asyncio-task"},
		// A thread held on an attribute or a container element is still a thread.
		{"self-held thread", "class W:\n    def go(self):\n        self.worker = threading.Thread(target=work)\n        self.worker.start()\n", "thread-start"},
		{"self-held thread conditional join", "class W:\n    def go(self, cond):\n        self.worker = threading.Thread(target=work)\n        self.worker.start()\n        if cond:\n            self.worker.join()\n", "thread-start"},
		{"container-held thread", "pool = {}\npool[\"a\"] = threading.Thread(target=work)\npool[\"a\"].start()\n", "thread-start"},
		{"offload conditional await", "async def run(cond):\n    result = asyncio.to_thread(compute, arg)\n    if cond:\n        await result\n", "asyncio-to-thread"},
	}
	for _, tc := range flagged {
		if !containsPyRule(scanPythonSource(tc.src), tc.rule) {
			t.Errorf("%s: detector missed %q (want rule %s); got %+v", tc.name, tc.src, tc.rule, scanPythonSource(tc.src))
		}
	}

	clean := []struct{ name, src string }{
		{"submit inside with", "with ThreadPoolExecutor(max_workers=4) as ex:\n    futures = {ex.submit(scan, s): s for s in segs}\n    for f in futures:\n        f.result()\n"},
		{"task awaited", "task = asyncio.create_task(work())\nawait task\n"},
		{"tasks gathered inline", "results = await asyncio.gather(work_a(), work_b())\n"},
		{"thread joined", "t = threading.Thread(target=work)\nt.start()\nt.join()\n"},
		{"annotated thread joined", "t: threading.Thread = threading.Thread(target=work)\nt.start()\nt.join()\n"},
		{"daemon false joined", "t = threading.Thread(target=work, daemon=False)\nt.start()\nt.join()\n"},
		{"to_thread awaited", "result = await asyncio.to_thread(compute, arg)\n"},
		{"run_in_executor awaited", "future = await loop.run_in_executor(pool, compute)\n"},
		{"unstarted thread list", "threads = [\n    threading.Thread(target=work),\n]\n"},
		{"comment", "# asyncio.create_task(work()) is deliberately not used here\n"},
		{"comment comprehension", "# threads = [threading.Thread(target=work) for _ in range(4)] is not used\n"},
		{"import only", "import threading\nlock = threading.Lock()\n"},
		// A dominating join is accepted.
		{"thread joined in function", "def run():\n    t = threading.Thread(target=work)\n    t.start()\n    t.join()\n"},
		{"thread joined before a later return", "def run(cond):\n    t = threading.Thread(target=work)\n    t.start()\n    t.join()\n    if cond:\n        return\n"},
		{"self-held thread joined", "class W:\n    def go(self):\n        self.worker = threading.Thread(target=work)\n        self.worker.start()\n        self.worker.join()\n"},
		{"container-held thread joined", "pool = {}\npool[\"a\"] = threading.Thread(target=work)\npool[\"a\"].start()\npool[\"a\"].join()\n"},
		{"task awaited in return", "async def run():\n    task = asyncio.create_task(work())\n    other = await load()\n    return await task\n"},
		{"to_thread assigned then awaited", "result = asyncio.to_thread(compute, arg)\nvalue = await result\n"},
	}
	for _, tc := range clean {
		if found := scanPythonSource(tc.src); len(found) > 0 {
			t.Errorf("%s: detector false-positive on %q: %+v", tc.name, tc.src, found)
		}
	}
}

// containsPyRule reports whether any finding carries the given rule.
func containsPyRule(findings []pyFinding, rule string) bool {
	for _, f := range findings {
		if f.Rule == rule {
			return true
		}
	}
	return false
}
