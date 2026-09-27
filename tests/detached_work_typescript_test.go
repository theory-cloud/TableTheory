package tests

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// typescriptDetachedWorkAllowlist lists TypeScript launches that may run
// detached. Keys are "<relative path>:<line>". Every entry is reported in the
// PR body; the guard fails when a key stops matching a finding. It is empty:
// no TypeScript source we ship needs an exception.
var typescriptDetachedWorkAllowlist = launchAllowlist{}

// tsFinding is one fire-and-forget construct found in a TypeScript source.
type tsFinding struct {
	Text string
	Rule string
	Line int
}

var (
	// A `void <call>(...)` expression is the explicit TypeScript
	// fire-and-forget idiom: the promise is deliberately discarded. The bracket
	// alternative covers `void obj["method"]()`.
	tsVoidCall = regexp.MustCompile(`\bvoid\s+(?:\(|[A-Za-z_$][\w$.]*\s*\(|[A-Za-z_$][\w$.]*\s*\[\s*["'][\w$]+["']\s*\]\s*\()`)

	// Timer calls detach work from the current task. They are legitimate only
	// when they delay a promise being awaited, or when the handle is stored and
	// later unref'd or cleared — both recognized below. Timers reached through a
	// computed member (globalThis["setTimeout"]) are the same mechanism.
	tsTimerCall        = regexp.MustCompile(`\b(setTimeout|setInterval|setImmediate)\s*\(`)
	tsTimerBracketCall = regexp.MustCompile(`\b(?:window|globalThis|global)\s*\[\s*["'](setTimeout|setInterval|setImmediate)["']\s*\]\s*\(`)

	// `const timer = setTimeout(...)` — the handle is stored so it can be
	// disposed of. The name is checked against the rest of the file.
	tsTimerAssign        = regexp.MustCompile(`(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*=\s*(?:await\s+)?(?:window\.|globalThis\.)?(?:setTimeout|setInterval|setImmediate)\s*\(`)
	tsTimerAssignBracket = regexp.MustCompile(`(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*=\s*(?:await\s+)?(?:window|globalThis|global)\s*\[\s*["'](?:setTimeout|setInterval|setImmediate)["']\s*\]\s*\(`)
	tsAssignOnly         = regexp.MustCompile(`^\s*([A-Za-z_$][\w$]*)\s*=\s*(?:window\.|globalThis\.)?(?:setTimeout|setInterval|setImmediate)\s*\(`)
	tsAssignOnlyBracket  = regexp.MustCompile(`^\s*([A-Za-z_$][\w$]*)\s*=\s*(?:window|globalThis|global)\s*\[\s*["'](?:setTimeout|setInterval|setImmediate)["']\s*\]\s*\(`)

	// A promise resolver passed to setTimeout is a sleep, not detached work. The
	// optional quote and bracket let the bracket form match too.
	tsResolverDelay = regexp.MustCompile(`set(?:Timeout|Interval|Immediate)\s*["']?\s*\]?\s*\(\s*(?:resolve|r|res|done|reject|_resolve|_r)\b`)

	tsNewPromise = regexp.MustCompile(`new\s+Promise\s*[<(]`)

	// Microtask scheduling always runs after the current task returns; there is
	// no await that can join it.
	tsQueueMicrotask         = regexp.MustCompile(`\bqueueMicrotask\s*\(`)
	tsQueueMicrotaskBracket  = regexp.MustCompile(`\b(?:window|globalThis|global)\s*\[\s*["']queueMicrotask["']\s*\]\s*\(`)
	tsProcessNextTick        = regexp.MustCompile(`\bprocess\s*\.\s*nextTick\s*\(`)
	tsProcessNextTickBracket = regexp.MustCompile(`\bprocess\s*\[\s*["']nextTick["']\s*\]\s*\(`)

	// A statement-position async IIFE hands its promise to nobody.
	tsAsyncIIFE = regexp.MustCompile(`^\s*\(\s*async\s*(?:\(|function\b)`)

	// Names declared as async in the same file, so a bare statement-position
	// call to one of them is a discarded promise. Typed detection for the
	// library package remains the job of @typescript-eslint/no-floating-promises,
	// which covers ts/**; this covers the examples and tooling surfaces too.
	tsAsyncFunctionDecl = regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:default\s+)?async\s+function\s+([A-Za-z_$][\w$]*)`)
	tsAsyncArrowDecl    = regexp.MustCompile(`(?m)(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*=\s*async\b`)

	// A promise bound to a name and a member of the promise chain: the promise
	// is a launch, and it is joined only by an await or a return that dominates.
	tsPromiseBind = regexp.MustCompile(`^\s*(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*(?::[^=]*?)?=\s*(.+)$`)

	// A `return` or `throw` at the line's statement level can leave the function
	// before a later join runs; a line carrying an arrow body does not.
	tsReturnStmt = regexp.MustCompile(`\b(?:return|throw)\b`)
	tsArrow      = regexp.MustCompile(`=>`)

	// Control statements open a block, not a function body.
	tsControlOpener = regexp.MustCompile(`^\s*(?:if|for|while|switch|catch|do|else|try|finally)\b`)

	// A method or function header. Only used to bound the region a join must
	// cover; a line that is misread as a function can only shorten that region,
	// never accept a detached launch.
	tsMethodOpener = regexp.MustCompile(`^\s*(?:(?:public|private|protected|static|async|get|set|readonly|declare|override)\s+)*[A-Za-z_$][\w$]*\s*\([^;]*\)\s*(?::\s*[^{;]+)?\{\s*$`)
)

// tsChainMembers are the promise-chain members that make a bound promise a
// launch, mapped to the rule each reports.
var tsChainMembers = []string{".then(", ".catch(", ".finally("}

var tsChainRules = map[string]string{
	".then(":    "floating-then",
	".catch(":   "floating-catch",
	".finally(": "floating-finally",
}

// tsStructure is the brace-derived block structure of a TypeScript source. It
// exists so a promise's awaited join can be required to dominate every return
// path of the function that bound it, mirroring the Go AST walk. No TypeScript
// parser is available to this Go test, so the structure is read from braces
// (with comments and string contents removed) and function headers.
type tsStructure struct {
	openers [][]int
	funcs   [][]int
}

// buildTSStructure reads the brace structure of src line by line.
func buildTSStructure(lines []string) tsStructure {
	st := tsStructure{openers: make([][]int, len(lines)), funcs: make([][]int, len(lines))}
	var braceLines []int
	var braceIsFunc []bool
	for i, raw := range lines {
		code := tsStripCode(raw)
		st.openers[i] = append([]int(nil), braceLines...)
		funcs := make([]int, 0, len(braceLines))
		for k, line := range braceLines {
			if braceIsFunc[k] {
				funcs = append(funcs, line)
			}
		}
		st.funcs[i] = funcs
		isFunc := tsLineOpensFunction(code)
		for j := 0; j < len(code); j++ {
			switch code[j] {
			case '{':
				braceLines = append(braceLines, i+1)
				braceIsFunc = append(braceIsFunc, isFunc)
			case '}':
				if len(braceLines) > 0 {
					braceLines = braceLines[:len(braceLines)-1]
					braceIsFunc = braceIsFunc[:len(braceIsFunc)-1]
				}
			}
		}
	}
	return st
}

// tsStripCode removes line comments and simple string contents so a brace or
// the word `return` inside prose or a literal cannot shift the block structure.
func tsStripCode(line string) string {
	var b strings.Builder
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
		case '\'', '"', '`':
			quote = c
		case '/':
			if i+1 < len(line) && line[i+1] == '/' {
				return b.String()
			}
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// tsLineOpensFunction reports whether a line opens a function body, so its
// brace can bound the region a join must cover.
func tsLineOpensFunction(code string) bool {
	if strings.Contains(code, "function") || strings.Contains(code, "=>") {
		return true
	}
	if tsControlOpener.MatchString(code) {
		return false
	}
	return tsMethodOpener.MatchString(code)
}

// tsDominates reports whether a join for the promise bound at launchLine (a
// 1-based line number) runs on every path that returns from the launch's own
// function. The join must appear after the launch, must not sit inside a block
// the launch is not inside, and no return or throw may be crossed first.
func tsDominates(st tsStructure, lines []string, launchLine int, name string) bool {
	start := launchLine - 1
	if start < 0 || start >= len(lines) {
		return false
	}
	baseFuncs := st.funcs[start]
	baseOpen := st.openers[start]
	await := regexp.MustCompile(`\bawait\s+` + regexp.QuoteMeta(name) + `\b`)
	ret := regexp.MustCompile(`\breturn\s+` + regexp.QuoteMeta(name) + `\b`)
	for j := start + 1; j < len(lines); j++ {
		if !intStackPrefix(baseFuncs, st.funcs[j]) {
			return false
		}
		code := tsStripCode(lines[j])
		if intSubset(st.openers[j], baseOpen) && (await.MatchString(code) || ret.MatchString(code)) {
			return true
		}
		if tsReturnStmt.MatchString(code) && !tsArrow.MatchString(code) && intStackEqual(baseFuncs, st.funcs[j]) {
			return false
		}
	}
	return false
}

// tsTimerName reports the timer method a line calls, in either property or
// bracket access form.
func tsTimerName(line string) (string, bool) {
	if m := tsTimerCall.FindStringSubmatch(line); m != nil {
		return m[1], true
	}
	if m := tsTimerBracketCall.FindStringSubmatch(line); m != nil {
		return m[1], true
	}
	return "", false
}

// scanTypeScriptSource finds TypeScript fire-and-forget launches. It is a
// deliberately conservative line-and-brace scan: TypeScript has no parser
// available to this Go test, so the detector targets the banned idioms named by
// policy — `void <call>(`, a bare `.then(`/`.catch(`/`.finally(` chain that
// neither awaits, returns, nor assigns, a promise bound to a name and never
// awaited on a dominating path, a statement-position async IIFE, microtask
// scheduling, a timer that is neither a resolver delay nor a managed, cleared
// handle, and a bare call to a function this file declares as async. A timer or
// microtask reached through a computed member (`globalThis["setTimeout"]`,
// `process["nextTick"]`) is the same launch as its property-access spelling.
func scanTypeScriptSource(src string) []tsFinding {
	lines := strings.Split(src, "\n")
	st := buildTSStructure(lines)
	var findings []tsFinding

	timerNames := map[string]bool{}
	managedNames := map[string]bool{}
	for _, re := range []*regexp.Regexp{tsTimerAssign, tsTimerAssignBracket} {
		for _, m := range re.FindAllStringSubmatch(src, -1) {
			timerNames[m[1]] = true
		}
	}
	for _, re := range []*regexp.Regexp{tsAssignOnly, tsAssignOnlyBracket} {
		for _, m := range re.FindAllStringSubmatch(src, -1) {
			timerNames[m[1]] = true
		}
	}
	for name := range timerNames {
		disposers := []string{
			"clearTimeout(" + name + ")",
			"clearInterval(" + name + ")",
			"clearImmediate(" + name + ")",
			name + ".unref",
		}
		for _, d := range disposers {
			if strings.Contains(src, d) {
				managedNames[name] = true
				break
			}
		}
	}

	asyncNames := map[string]bool{}
	for _, m := range tsAsyncFunctionDecl.FindAllStringSubmatch(src, -1) {
		asyncNames[m[1]] = true
	}
	for _, m := range tsAsyncArrowDecl.FindAllStringSubmatch(src, -1) {
		asyncNames[m[1]] = true
	}

	prevNonEmpty := ""
	for i, line := range lines {
		lineNo := i + 1
		trimmed := strings.TrimSpace(line)
		record := func(rule string) {
			findings = append(findings, tsFinding{Line: lineNo, Text: trimmed, Rule: rule})
		}

		// Whole-line comments are prose. The scan is line-based, so a trailing
		// comment on a code line is still inspected; whole-line comments are
		// the case that matters because documentation routinely shows the
		// banned idioms as examples.
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "/*") {
			if trimmed != "" {
				prevNonEmpty = line
			}
			continue
		}

		code := tsStripCode(line)

		if tsVoidCall.MatchString(line) {
			record("void-call")
		}

		if tsFloatingChain(line, ".then(") {
			record("floating-then")
		}
		if tsFloatingChain(line, ".catch(") {
			record("floating-catch")
		}
		if tsFloatingChain(line, ".finally(") {
			record("floating-finally")
		}

		// A promise bound to a name is a launch unless a dominating await or
		// return joins it.
		if m := tsPromiseBind.FindStringSubmatch(code); m != nil {
			rhs := m[2]
			if !strings.Contains(rhs, "await ") {
				for _, member := range tsChainMembers {
					if !strings.Contains(rhs, member) {
						continue
					}
					if !tsDominates(st, lines, lineNo, m[1]) {
						record(tsChainRules[member])
					}
					break
				}
			}
		}

		if tsAsyncIIFE.MatchString(line) {
			record("detached-async-iife")
		}

		if tsQueueMicrotask.MatchString(line) || tsQueueMicrotaskBracket.MatchString(line) {
			record("queue-microtask")
		}
		if tsProcessNextTick.MatchString(line) || tsProcessNextTickBracket.MatchString(line) {
			record("process-next-tick")
		}

		if tsDiscardedAsyncCall(trimmed, asyncNames) {
			record("discarded-async-call")
		}

		if name, ok := tsTimerName(line); ok {
			if !tsResolverDelay.MatchString(line) &&
				!tsNewPromise.MatchString(line) &&
				!tsNewPromise.MatchString(prevNonEmpty) &&
				!tsTimerHandleIsManaged(line, managedNames) {
				record("floating-" + name)
			}
		}

		if trimmed != "" {
			prevNonEmpty = line
		}
	}

	return findings
}

// tsDiscardedAsyncCall reports whether the line is a statement-position call to
// a function this file declares as async, which discards the returned promise.
func tsDiscardedAsyncCall(trimmed string, asyncNames map[string]bool) bool {
	for name := range asyncNames {
		if !strings.HasPrefix(trimmed, name) {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(trimmed, name))
		if !strings.HasPrefix(rest, "(") {
			continue
		}
		// Declarations and type positions are not calls.
		if strings.Contains(trimmed, "=>") || strings.Contains(rest, "=>") {
			return false
		}
		return true
	}
	return false
}

// tsTimerHandleIsManaged reports whether a timer call on this line stores its
// handle under a name that the file later unref's or clears.
func tsTimerHandleIsManaged(line string, managed map[string]bool) bool {
	for _, re := range []*regexp.Regexp{tsTimerAssign, tsTimerAssignBracket, tsAssignOnly, tsAssignOnlyBracket} {
		if m := re.FindStringSubmatch(line); m != nil && managed[m[1]] {
			return true
		}
	}
	return false
}

// tsFloatingChain reports whether the line has a chain member (`.then(`,
// `.catch(`, `.finally(`) that neither awaits it, returns it, nor assigns it —
// the canonical dropped-promise shape.
func tsFloatingChain(line, member string) bool {
	idx := strings.Index(line, member)
	if idx < 0 {
		return false
	}
	prefix := line[:idx]
	if strings.Contains(prefix, "await ") || strings.Contains(prefix, "return") {
		return false
	}
	if hasAssignmentOperator(prefix) {
		return false
	}
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	// Only a statement-position chain is a dropped promise. Declarations, type
	// positions, and object literal keys are not.
	switch {
	case strings.HasPrefix(trimmed, "const "),
		strings.HasPrefix(trimmed, "let "),
		strings.HasPrefix(trimmed, "var "),
		strings.HasPrefix(trimmed, "await "),
		strings.HasPrefix(trimmed, "return "),
		strings.HasPrefix(trimmed, "void "),
		strings.HasPrefix(trimmed, "export "),
		strings.HasPrefix(trimmed, "//"),
		strings.HasPrefix(trimmed, "*"):
		return false
	}
	return true
}

// hasAssignmentOperator reports whether s contains an assignment `=` that is not
// part of a comparison or an arrow function.
func hasAssignmentOperator(s string) bool {
	cleaned := s
	for _, op := range []string{"=>", "===", "==", "!==", "!=", ">=", "<=", "+=", "-=", "*=", "/="} {
		cleaned = strings.ReplaceAll(cleaned, op, " ")
	}
	return strings.Contains(cleaned, "=")
}

// TestTypeScript_NoDetachedWork fails when a TypeScript or JavaScript source we
// ship (ts/src, ts/examples, examples) or run (scripts, contract-test runners)
// launches work that is neither awaited nor returned. Every path in those
// surfaces is scanned; there is no exclusion.
func TestTypeScript_NoDetachedWork(t *testing.T) {
	root := detachedWorkRepoRoot(t)
	reportAllowlist(t, typescriptDetachedWorkAllowlist)

	scanned := 0
	scannedKeys := map[string]bool{}
	var problems []string

	include := func(rel string) bool {
		if !hasTypeScriptSuffix(rel) {
			return false
		}
		for _, suffix := range []string{".test.ts", ".spec.ts", ".d.ts", ".test.mts", ".test.js"} {
			if strings.HasSuffix(rel, suffix) {
				return false
			}
		}
		return true
	}
	visit := func(rel string, src []byte) error {
		scanned++
		for _, finding := range scanTypeScriptSource(string(src)) {
			key := launchKey(rel, finding.Line)
			scannedKeys[key] = true
			if typescriptDetachedWorkAllowlist.allowlisted(rel, finding.Line) {
				continue
			}
			problems = append(problems, describeTSFinding(rel, finding))
		}
		return nil
	}
	walkDetachedWorkSources(t, root, []string{"ts/src", "ts/examples", "examples", "scripts", "contract-tests/runners"}, include, visit)

	t.Logf("scanned %d TypeScript/JavaScript source files", scanned)
	if scanned == 0 {
		t.Fatal("guard is vacuous: no TypeScript source files were scanned")
	}
	typescriptDetachedWorkAllowlist.checkAllowlistCoverage(t, scannedKeys)
	if len(problems) > 0 {
		sort.Strings(problems)
		t.Fatalf(
			"TypeScript must not launch work that outlives the task that started it.\n"+
				"Await or return the call, clear the timer, or add the exact line to typescriptDetachedWorkAllowlist with a justification:\n%s",
			strings.Join(problems, "\n"),
		)
	}
}

// hasTypeScriptSuffix reports whether rel is a TypeScript or JavaScript source
// we scan (the detectors are syntax-based and apply to both).
func hasTypeScriptSuffix(rel string) bool {
	for _, suffix := range []string{".ts", ".mts", ".cts", ".js", ".mjs", ".cjs"} {
		if strings.HasSuffix(rel, suffix) {
			return true
		}
	}
	return false
}

// describeTSFinding renders a TypeScript finding for a reviewer.
func describeTSFinding(rel string, f tsFinding) string {
	return rel + ":" + strconv.Itoa(f.Line) + ": [" + f.Rule + "] " + f.Text
}

// TestTypeScriptDetachedWorkDetectorIsNotVacuous proves the TypeScript detector
// fires on each banned idiom and stays quiet on the shapes the repository
// legitimately uses.
func TestTypeScriptDetachedWorkDetectorIsNotVacuous(t *testing.T) {
	flagged := []struct {
		name string
		src  string
		rule string
	}{
		{"void call", "void sendNotification(user);", "void-call"},
		{"void iife", "void (async () => { await work(); })();", "void-call"},
		{"void bracket call", "void obj[\"send\"](user);", "void-call"},
		{"bare then", "fetchUser(id).then((user) => { render(user); });", "floating-then"},
		{"bare then on literal", "Promise.resolve(1).then((v) => console.log(v));", "floating-then"},
		{"bare catch", "sendWebhook(job).catch((err) => { log(err); });", "floating-catch"},
		{"bare finally", "flushQueue().finally(() => { markDone(); });", "floating-finally"},
		{"async iife", "(async () => {\n  await work();\n})();", "detached-async-iife"},
		{"async function iife", "(async function () {\n  await work();\n})();", "detached-async-iife"},
		{"queueMicrotask", "queueMicrotask(() => { flush(); });", "queue-microtask"},
		{"bracket queueMicrotask", "globalThis[\"queueMicrotask\"](() => { flush(); });", "queue-microtask"},
		{"process.nextTick", "process.nextTick(() => { flush(); });", "process-next-tick"},
		{"bracket process.nextTick", "process[\"nextTick\"](() => { flush(); });", "process-next-tick"},
		{"discarded async call", "async function syncUser(id: string): Promise<void> {\n  await write(id);\n}\nsyncUser(\"u1\");", "discarded-async-call"},
		{"discarded async arrow call", "const flush = async (): Promise<void> => {\n  await write();\n};\nflush();", "discarded-async-call"},
		{"unmanaged timer", "setTimeout(() => flush(), 1000);", "floating-setTimeout"},
		{"unmanaged interval", "setInterval(poll, 5000);", "floating-setInterval"},
		{"bracket timer", "globalThis[\"setTimeout\"](() => flush(), 1000);", "floating-setTimeout"},
		{"bracket interval", "window[\"setInterval\"](poll, 5000);", "floating-setInterval"},
		{"bracket immediate", "global[\"setImmediate\"](work);", "floating-setImmediate"},
		// A bound promise is a launch; a join that does not dominate is not one.
		{"bound chain returned on one path only", "async function run(cond) {\n  const p = load(id).then((u) => render(u));\n  if (cond) return;\n  await p;\n}", "floating-then"},
		{"bound chain awaited inside a conditional", "async function run(cond) {\n  const p = load(id).then((u) => render(u));\n  if (cond) {\n    await p;\n  }\n}", "floating-then"},
		{"assigned catch never awaited", "const settled = sendWebhook(job).catch((err) => { log(err); });", "floating-catch"},
	}
	for _, tc := range flagged {
		found := scanTypeScriptSource(tc.src + "\n")
		if !containsRule(found, tc.rule) {
			t.Errorf("%s: detector missed %q (want rule %s); got %+v", tc.name, tc.src, tc.rule, found)
		}
	}

	clean := []struct {
		name string
		src  string
	}{
		{"awaited promise delay", "await new Promise((r) => setTimeout(r, ms));"},
		{"block promise delay", "await new Promise<void>((resolve) => {\n  setTimeout(resolve, 200);\n});"},
		{"managed timer", "const timer = setTimeout(() => controller.abort(), ms);\ntimer.unref?.();\nreturn { cleanup: () => clearTimeout(timer) };"},
		{"managed bracket timer", "const timer = globalThis[\"setTimeout\"](() => controller.abort(), ms);\nclearTimeout(timer);"},
		{"awaited then", "const p = sem.acquire().then(() => { release(); });\nawait p;"},
		{"returned then", "return fetchThing().then((x) => x.value);"},
		{"awaited catch", "await sendWebhook(job).catch((err) => { log(err); });"},
		{"returned finally", "return flushQueue().finally(() => { markDone(); });"},
		{"bound promise awaited", "async function run() {\n  const p = load(id).then((u) => render(u));\n  await p;\n}"},
		{"bound promise returned", "async function run() {\n  const p = load(id).then((u) => render(u));\n  return p;\n}"},
		{"bound promise awaited before a later return", "async function run(cond) {\n  const p = load(id).then((u) => render(u));\n  await p;\n  if (cond) return;\n}"},
		{"awaited async call", "async function syncUser(id: string): Promise<void> {\n  await write(id);\n}\nawait syncUser(\"u1\");"},
		{"returned async call", "async function syncUser(id: string): Promise<void> {\n  await write(id);\n}\nreturn syncUser(\"u1\");"},
		{"async callback in map", "const users = await Promise.all(actions.map(async (_, index) => await load(index)));"},
		{"comment", "// void sendNotification(user) is deliberately not used here"},
		{"comment catch", "// sendWebhook(job).catch((err) => log(err)); is deliberately not used here"},
	}
	for _, tc := range clean {
		if found := scanTypeScriptSource(tc.src + "\n"); len(found) > 0 {
			t.Errorf("%s: detector false-positive on %q: %+v", tc.name, tc.src, found)
		}
	}
}

// containsRule reports whether any finding carries the given rule.
func containsRule(findings []tsFinding, rule string) bool {
	for _, f := range findings {
		if f.Rule == rule {
			return true
		}
	}
	return false
}
