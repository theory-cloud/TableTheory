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
	// fire-and-forget idiom: the promise is deliberately discarded.
	tsVoidCall = regexp.MustCompile(`\bvoid\s+(?:\(|[A-Za-z_$][\w$.]*\s*\()`)

	// Timer calls detach work from the current task. They are legitimate only
	// when they delay a promise being awaited, or when the handle is stored and
	// later unref'd or cleared — both recognized below.
	tsTimerCall = regexp.MustCompile(`\b(setTimeout|setInterval|setImmediate)\s*\(`)

	// `const timer = setTimeout(...)` — the handle is stored so it can be
	// disposed of. The name is checked against the rest of the file.
	tsTimerAssign = regexp.MustCompile(`(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*=\s*(?:await\s+)?(?:window\.|globalThis\.)?(?:setTimeout|setInterval|setImmediate)\s*\(`)
	tsAssignOnly  = regexp.MustCompile(`^\s*([A-Za-z_$][\w$]*)\s*=\s*(?:window\.|globalThis\.)?(?:setTimeout|setInterval|setImmediate)\s*\(`)

	// A promise resolver passed to setTimeout is a sleep, not detached work.
	tsResolverDelay = regexp.MustCompile(`set(?:Timeout|Interval|Immediate)\s*\(\s*(?:resolve|r|res|done|reject|_resolve|_r)\b`)

	tsNewPromise = regexp.MustCompile(`new\s+Promise\s*[<(]`)

	// Microtask scheduling always runs after the current task returns; there is
	// no await that can join it.
	tsQueueMicrotask  = regexp.MustCompile(`\bqueueMicrotask\s*\(`)
	tsProcessNextTick = regexp.MustCompile(`\bprocess\s*\.\s*nextTick\s*\(`)

	// A statement-position async IIFE hands its promise to nobody.
	tsAsyncIIFE = regexp.MustCompile(`^\s*\(\s*async\s*(?:\(|function\b)`)

	// Names declared as async in the same file, so a bare statement-position
	// call to one of them is a discarded promise. Typed detection for the
	// library package remains the job of @typescript-eslint/no-floating-promises,
	// which covers ts/**; this covers the examples and tooling surfaces too.
	tsAsyncFunctionDecl = regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:default\s+)?async\s+function\s+([A-Za-z_$][\w$]*)`)
	tsAsyncArrowDecl    = regexp.MustCompile(`(?m)(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*=\s*async\b`)
)

// scanTypeScriptSource finds TypeScript fire-and-forget launches. It is a
// deliberately conservative line scan: TypeScript has no parser available to
// this Go test, so the detector targets the banned idioms named by policy —
// `void <call>(`, a bare `.then(`/`.catch(`/`.finally(` chain that neither
// awaits, returns, nor assigns, a statement-position async IIFE, microtask
// scheduling, a timer that is neither a resolver delay nor a managed, cleared
// handle, and a bare call to a function this file declares as async.
func scanTypeScriptSource(src string) []tsFinding {
	lines := strings.Split(src, "\n")
	var findings []tsFinding

	timerNames := map[string]bool{}
	managedNames := map[string]bool{}
	for _, m := range tsTimerAssign.FindAllStringSubmatch(src, -1) {
		timerNames[m[1]] = true
	}
	for _, m := range tsAssignOnly.FindAllStringSubmatch(src, -1) {
		timerNames[m[1]] = true
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

		if tsAsyncIIFE.MatchString(line) {
			record("detached-async-iife")
		}

		if tsQueueMicrotask.MatchString(line) {
			record("queue-microtask")
		}
		if tsProcessNextTick.MatchString(line) {
			record("process-next-tick")
		}

		if tsDiscardedAsyncCall(trimmed, asyncNames) {
			record("discarded-async-call")
		}

		if m := tsTimerCall.FindStringSubmatch(line); m != nil {
			if !tsResolverDelay.MatchString(line) &&
				!tsNewPromise.MatchString(line) &&
				!tsNewPromise.MatchString(prevNonEmpty) &&
				!tsTimerHandleIsManaged(line, managedNames) {
				record("floating-" + m[1])
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
	if m := tsTimerAssign.FindStringSubmatch(line); m != nil && managed[m[1]] {
		return true
	}
	if m := tsAssignOnly.FindStringSubmatch(line); m != nil && managed[m[1]] {
		return true
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
		{"bare then", "fetchUser(id).then((user) => { render(user); });", "floating-then"},
		{"bare then on literal", "Promise.resolve(1).then((v) => console.log(v));", "floating-then"},
		{"bare catch", "sendWebhook(job).catch((err) => { log(err); });", "floating-catch"},
		{"bare finally", "flushQueue().finally(() => { markDone(); });", "floating-finally"},
		{"async iife", "(async () => {\n  await work();\n})();", "detached-async-iife"},
		{"async function iife", "(async function () {\n  await work();\n})();", "detached-async-iife"},
		{"queueMicrotask", "queueMicrotask(() => { flush(); });", "queue-microtask"},
		{"process.nextTick", "process.nextTick(() => { flush(); });", "process-next-tick"},
		{"discarded async call", "async function syncUser(id: string): Promise<void> {\n  await write(id);\n}\nsyncUser(\"u1\");", "discarded-async-call"},
		{"discarded async arrow call", "const flush = async (): Promise<void> => {\n  await write();\n};\nflush();", "discarded-async-call"},
		{"unmanaged timer", "setTimeout(() => flush(), 1000);", "floating-setTimeout"},
		{"unmanaged interval", "setInterval(poll, 5000);", "floating-setInterval"},
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
		{"awaited then", "const p = sem.acquire().then(() => { release(); });\nawait p;"},
		{"returned then", "return fetchThing().then((x) => x.value);"},
		{"awaited catch", "await sendWebhook(job).catch((err) => { log(err); });"},
		{"returned finally", "return flushQueue().finally(() => { markDone(); });"},
		{"assigned catch", "const settled = sendWebhook(job).catch((err) => { log(err); });"},
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
