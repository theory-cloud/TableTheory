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

var (
	// A thread or process started and never joined outlives the caller.
	pyThreadCtor   = regexp.MustCompile(`\b(?:threading\.(?:Thread|Timer)|multiprocessing\.Process)\s*\(`)
	pyThreadAssign = regexp.MustCompile(`^\s*([A-Za-z_]\w*)\s*(?::[^=]*)?=\s*(?:threading\.(?:Thread|Timer)|multiprocessing\.Process)\s*\(`)
	pyStart        = regexp.MustCompile(`^\s*([A-Za-z_]\w*)\s*\.\s*start\s*\(\s*\)`)
	pyJoin         = regexp.MustCompile(`(?:^|\s)([A-Za-z_]\w*)\s*\.\s*join\s*\(`)

	// A thread created inline in a comprehension has no name to join later.
	pyComprehension = regexp.MustCompile(`\bfor\b[^:]*\bin\b`)

	// A daemon thread is explicitly declared as one not to wait for; it is
	// killed when the process exits and no caller holds a handle to join.
	pyDaemonKwarg  = regexp.MustCompile(`\bdaemon\s*=\s*True\b`)
	pyDaemonAssign = regexp.MustCompile(`^\s*([A-Za-z_]\w*)\s*\.\s*daemon\s*=\s*True`)

	// A task created with asyncio and never awaited or gathered is detached.
	pyAsyncioTask = regexp.MustCompile(`\basyncio\.(?:create_task|ensure_future)\s*\(`)
	pyTaskAssign  = regexp.MustCompile(`^\s*([A-Za-z_]\w*)\s*=\s*.*?\basyncio\.(?:create_task|ensure_future)\s*\(`)
	pyGather      = regexp.MustCompile(`\basyncio\.(?:gather|wait)\s*\(`)

	// Offloading to a worker thread is only joined by awaiting the result.
	pyToThread      = regexp.MustCompile(`\basyncio\.to_thread\s*\(`)
	pyRunInExecutor = regexp.MustCompile(`\brun_in_executor\s*\(`)

	// An executor submit outside its `with` block is not joined by the context
	// manager's exit.
	pyWithExecutor = regexp.MustCompile(`^(\s*)with\s+.*(?:ThreadPoolExecutor|ProcessPoolExecutor)\s*[(.]`)
	pySubmit       = regexp.MustCompile(`\.submit\s*\(`)
)

// scanPythonSource finds Python detached-work launches: threads or processes
// started without a later join (including threads created inline in a
// comprehension and daemon threads), asyncio tasks created without await or
// gather, thread offloads (asyncio.to_thread, loop.run_in_executor) that are
// never awaited, and executor submits that escape their `with` block. It is a
// deliberately conservative line scan, since no Python parser is available to
// this Go test.
func scanPythonSource(src string) []pyFinding {
	lines := strings.Split(src, "\n")
	var findings []pyFinding

	// Thread names bound to a Thread/Timer/Process constructor, and the lines
	// where each such name is started and joined. A start with a later join is
	// a joined thread and is not detached work.
	threadNames := map[string]bool{}
	for _, line := range lines {
		if m := pyThreadAssign.FindStringSubmatch(line); m != nil {
			threadNames[m[1]] = true
		}
	}
	joinedStarts := map[string]bool{}
	startLines := map[string][]int{}
	joinLines := map[string][]int{}
	for i, line := range lines {
		if m := pyStart.FindStringSubmatch(line); m != nil && threadNames[m[1]] {
			startLines[m[1]] = append(startLines[m[1]], i+1)
		}
		if m := pyJoin.FindStringSubmatch(line); m != nil && threadNames[m[1]] {
			joinLines[m[1]] = append(joinLines[m[1]], i+1)
		}
	}
	for name, starts := range startLines {
		for _, s := range starts {
			for _, j := range joinLines[name] {
				if j > s {
					joinedStarts[nameLineKey(name, s)] = true
				}
			}
		}
	}

	taskNames := map[string]bool{}
	for _, line := range lines {
		if m := pyTaskAssign.FindStringSubmatch(line); m != nil {
			taskNames[m[1]] = true
		}
	}
	joinedTasks := map[string]bool{}
	for name := range taskNames {
		if strings.Contains(src, "await "+name) || strings.Contains(src, "gather("+name) ||
			strings.Contains(src, "gather(*"+name) || strings.Contains(src, name+".cancel()") {
			joinedTasks[name] = true
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

		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
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
		} else if m := pyStart.FindStringSubmatch(line); m != nil && threadNames[m[1]] {
			if !joinedStarts[nameLineKey(m[1], lineNo)] {
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
				if m := pyTaskAssign.FindStringSubmatch(line); m != nil && joinedTasks[m[1]] {
					recognized = true
				}
			}
			if !recognized {
				record("asyncio-task")
			}
		}

		if pyToThread.MatchString(line) && !strings.Contains(line, "await ") {
			record("asyncio-to-thread")
		}
		if pyRunInExecutor.MatchString(line) && !strings.Contains(line, "await ") {
			record("run-in-executor")
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

// keys builds the "<name>:<line>" key used for joined-start bookkeeping.
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
		{"to_thread not awaited", "result = asyncio.to_thread(compute, arg)\n", "asyncio-to-thread"},
		{"to_thread bare", "asyncio.to_thread(compute, arg)\n", "asyncio-to-thread"},
		{"run_in_executor not awaited", "future = loop.run_in_executor(pool, compute)\n", "run-in-executor"},
		{"submit outside with", "ex = ThreadPoolExecutor(max_workers=4)\nex.submit(work)\n", "executor-submit"},
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
