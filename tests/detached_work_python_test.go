package tests

import (
	"fmt"
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

// TestPython_NoDetachedWork fails when a Python source we ship (py/src,
// py/examples, examples) or run (scripts, contract-test runners) launches work
// that is not joined before the caller returns.
//
// Every scanned file is parsed with Python's own `ast` module by
// tests/detachedwork/scan_python.py, and the join proof is the AST dominance
// walk described in that helper: a launch is accepted only when a join on the
// same target executes on every path from the launch to every exit of the
// function that contains it.
func TestPython_NoDetachedWork(t *testing.T) {
	root := detachedWorkRepoRoot(t)
	reportAllowlist(t, pythonDetachedWorkAllowlist)

	var sources []astSource
	var rels []string

	include := func(rel string) bool {
		if !strings.HasSuffix(rel, ".py") {
			return false
		}
		base := rel[strings.LastIndex(rel, "/")+1:]
		return !strings.HasPrefix(base, "test_") && !strings.HasSuffix(base, "_test.py") && base != "conftest.py"
	}
	visit := func(rel string, src []byte) error {
		sources = append(sources, astSource{Path: rel, Source: string(src)})
		rels = append(rels, rel)
		return nil
	}
	walkDetachedWorkSources(t, root, []string{"py/src", "py/examples", "examples", "scripts", "contract-tests/runners"}, include, visit)

	t.Logf("scanned %d Python source files", len(sources))
	if len(sources) == 0 {
		t.Fatal("guard is vacuous: no Python source files were scanned")
	}

	scannedKeys := map[string]bool{}
	var problems []string
	results := scanPythonBatch(t, sources)
	for i, rel := range rels {
		for _, finding := range results[i] {
			key := launchKey(rel, finding.Line)
			scannedKeys[key] = true
			if pythonDetachedWorkAllowlist.allowlisted(rel, finding.Line) {
				continue
			}
			problems = append(problems, rel+":"+strconv.Itoa(finding.Line)+": ["+finding.Rule+"] "+finding.Text)
		}
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

// pythonDetectorFlagged are the shapes the Python detector must report. Each is
// a launch whose join does not dominate every path out of its function; the
// single-line and expression-position entries are the ones a line/indentation
// reading accepts by mistake.
var pythonDetectorFlagged = []struct{ name, src, rule string }{
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
	{"raise before join", "def run():\n    t = threading.Thread(target=work)\n    t.start()\n    raise RuntimeError('stop')\n", "thread-start"},
	{"return inside a loop before the join", "def run(xs):\n    t = threading.Thread(target=work)\n    t.start()\n    for x in xs:\n        return\n    t.join()\n", "thread-start"},
	{"join inside a loop body", "def run(xs):\n    t = threading.Thread(target=work)\n    t.start()\n    for x in xs:\n        t.join()\n", "thread-start"},
	{"only one branch joins", "def run(cond):\n    t = threading.Thread(target=work)\n    t.start()\n    if cond:\n        t.join()\n    work()\n", "thread-start"},
	{"thread joined in a nested function", "def run():\n    t = threading.Thread(target=work)\n    t.start()\n    def inner():\n        t.join()\n    inner()\n", "thread-start"},
	{"conditional task await", "async def run(cond):\n    task = asyncio.create_task(work())\n    if cond:\n        await task\n", "asyncio-task"},
	{"early return before await", "async def run(abort):\n    task = asyncio.create_task(work())\n    if abort:\n        return\n    await task\n", "asyncio-task"},
	// A thread held on an attribute or a container element is still a thread.
	{"self-held thread", "class W:\n    def go(self):\n        self.worker = threading.Thread(target=work)\n        self.worker.start()\n", "thread-start"},
	{"self-held thread conditional join", "class W:\n    def go(self, cond):\n        self.worker = threading.Thread(target=work)\n        self.worker.start()\n        if cond:\n            self.worker.join()\n", "thread-start"},
	{"container-held thread", "pool = {}\npool[\"a\"] = threading.Thread(target=work)\npool[\"a\"].start()\n", "thread-start"},
	{"offload conditional await", "async def run(cond):\n    result = asyncio.to_thread(compute, arg)\n    if cond:\n        await result\n", "asyncio-to-thread"},
	// A join that only some paths execute is not a join: the one-line compound
	// and expression-position spellings a line-based reading accepted.
	{"one-line if join", "def run(cond):\n    t = threading.Thread(target=work)\n    t.start()\n    if cond: t.join()\n", "thread-start"},
	{"one-line for join", "def run(items):\n    t = threading.Thread(target=work)\n    t.start()\n    for x in items: t.join()\n", "thread-start"},
	{"one-line while join", "def run(cond):\n    t = threading.Thread(target=work)\n    t.start()\n    while cond: t.join()\n", "thread-start"},
	{"boolean-guard expression join", "def run(cond):\n    t = threading.Thread(target=work)\n    t.start()\n    ok = cond and t.join()\n", "thread-start"},
	{"conditional-expression join", "def run(cond):\n    t = threading.Thread(target=work)\n    t.start()\n    t.join() if cond else None\n", "thread-start"},
	{"one-line conditional task await", "async def run(cond):\n    task = asyncio.create_task(work())\n    if cond: await task\n", "asyncio-task"},
	{"one-line conditional offload await", "async def run(cond):\n    result = asyncio.to_thread(compute, arg)\n    if cond: await result\n", "asyncio-to-thread"},
	// A try whose handler can skip the join, or whose body joins only at its end.
	{"join at the end of a try body with a handler", "def run():\n    t = threading.Thread(target=work)\n    t.start()\n    try:\n        work()\n        t.join()\n    except Exception:\n        pass\n", "thread-start"},
	{"non-exhaustive handler before the join", "def run():\n    t = threading.Thread(target=work)\n    t.start()\n    try:\n        work()\n    except ValueError:\n        pass\n    t.join()\n", "thread-start"},
}

// pythonDetectorClean are the shapes the Python detector must accept. They are
// the joins the repository is allowed to use, including the ones a naive
// reading rejects.
var pythonDetectorClean = []struct{ name, src string }{
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
	// A join that dominates on every path is accepted, in every spelling.
	{"join in finally dominates", "def run(c):\n    t = threading.Thread(target=work)\n    t.start()\n    try:\n        if c:\n            return 1\n        return 2\n    finally:\n        t.join()\n"},
	{"if/else both branches join", "def run(c):\n    t = threading.Thread(target=work)\n    t.start()\n    if c:\n        t.join()\n    else:\n        t.join()\n"},
	{"if/elif/else all branches join", "def run(c):\n    t = threading.Thread(target=work)\n    t.start()\n    if c == 1:\n        t.join()\n    elif c == 2:\n        t.join()\n    else:\n        t.join()\n"},
	{"one line start then join", "def run():\n    t = threading.Thread(target=work)\n    t.start(); t.join()\n"},
	{"loop join with the join after the loop", "def run(xs):\n    t = threading.Thread(target=work)\n    t.start()\n    for x in xs:\n        work(x)\n    t.join()\n"},
	{"task gathered", "async def run():\n    task = asyncio.create_task(work())\n    await asyncio.gather(task)\n"},
	{"task awaited in both try and except", "async def run():\n    task = asyncio.create_task(work())\n    try:\n        work()\n    except Exception:\n        await task\n    else:\n        await task\n"},
	{"alias join", "def run():\n    t = threading.Thread(target=work)\n    u = t\n    u.start()\n    u.join()\n"},
	{"offload alias awaited", "async def run():\n    r = asyncio.to_thread(f)\n    alias = r\n    await alias\n"},
}

// TestPythonDetachedWorkDetectorIsNotVacuous proves the Python detector fires on
// each banned construct and stays quiet on the joined forms the repository is
// allowed to use. One batch call parses every case with Python's `ast`.
func TestPythonDetachedWorkDetectorIsNotVacuous(t *testing.T) {
	sources := make([]astSource, 0, len(pythonDetectorFlagged)+len(pythonDetectorClean))
	for i, tc := range pythonDetectorFlagged {
		sources = append(sources, astSource{Path: fmt.Sprintf("flagged-%02d.py", i), Source: tc.src})
	}
	for i, tc := range pythonDetectorClean {
		sources = append(sources, astSource{Path: fmt.Sprintf("clean-%02d.py", i), Source: tc.src})
	}
	findings := scanPythonBatch(t, sources)

	for i, tc := range pythonDetectorFlagged {
		if !containsPyRule(findings[i], tc.rule) {
			t.Errorf("%s: detector missed %q (want rule %s); got %+v", tc.name, tc.src, tc.rule, findings[i])
		}
	}
	for i, tc := range pythonDetectorClean {
		got := findings[len(pythonDetectorFlagged)+i]
		if len(got) > 0 {
			t.Errorf("%s: detector false-positive on %q: %+v", tc.name, tc.src, got)
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
