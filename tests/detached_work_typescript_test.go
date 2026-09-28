package tests

import (
	"fmt"
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

// TestTypeScript_NoDetachedWork fails when a TypeScript or JavaScript source we
// ship (ts/src, ts/examples, examples) or run (scripts, contract-test runners)
// launches work that is neither awaited nor returned on every path.
//
// Every scanned file is parsed with the TypeScript compiler API by
// tests/detachedwork/scan_typescript.mjs, and the join proof is the AST
// dominance walk described in that helper: a launch is accepted only when a
// join on the same held target executes on every path from the launch to every
// exit of the function that contains it.
func TestTypeScript_NoDetachedWork(t *testing.T) {
	root := detachedWorkRepoRoot(t)
	reportAllowlist(t, typescriptDetachedWorkAllowlist)

	var sources []astSource
	var rels []string

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
		sources = append(sources, astSource{Path: rel, Source: string(src)})
		rels = append(rels, rel)
		return nil
	}
	walkDetachedWorkSources(t, root, []string{"ts/src", "ts/examples", "examples", "scripts", "contract-tests/runners"}, include, visit)

	t.Logf("scanned %d TypeScript/JavaScript source files", len(sources))
	if len(sources) == 0 {
		t.Fatal("guard is vacuous: no TypeScript source files were scanned")
	}

	scannedKeys := map[string]bool{}
	var problems []string
	results := scanTypeScriptBatch(t, sources)
	for i, rel := range rels {
		for _, finding := range results[i] {
			key := launchKey(rel, finding.Line)
			scannedKeys[key] = true
			if typescriptDetachedWorkAllowlist.allowlisted(rel, finding.Line) {
				continue
			}
			problems = append(problems, describeTSFinding(rel, finding))
		}
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

// typescriptDetectorFlagged are the shapes the TypeScript detector must report.
// Each is a launch whose join does not dominate every path out of its function;
// the one-line guarded, expression-position, and non-declaration-assignment
// entries are the ones a line/brace reading accepts by mistake.
var typescriptDetectorFlagged = []struct{ name, src, rule string }{
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
	{"held promise awaited in one branch only", "async function run(c) {\n  const p = load(id).then((u) => render(u));\n  if (c) {\n    await p;\n  }\n  work();\n}", "floating-then"},
	{"held promise awaited before a later throw", "async function run(c) {\n  const p = load(id).then((u) => render(u));\n  if (c) throw new Error('x');\n  await p;\n}", "floating-then"},
	{"held promise awaited inside a loop body", "async function run(xs) {\n  const p = load(id).then((u) => render(u));\n  for (const x of xs) {\n    await p;\n  }\n}", "floating-then"},
	// A join that only some paths execute is not a join: the one-line guarded,
	// expression-position, and non-declaration spellings a line-based reading
	// accepted.
	{"one-line if await", "async function run(cond) {\n  const p = load(id).then((u) => render(u));\n  if (cond) await p;\n}", "floating-then"},
	{"ternary await", "async function run(cond) {\n  const p = load(id).then((u) => render(u));\n  cond ? await p : null;\n}", "floating-then"},
	{"short-circuit await", "async function run(cond) {\n  const p = load(id).then((u) => render(u));\n  cond && await p;\n}", "floating-then"},
	{"join only inside a nested arrow", "async function run() {\n  const p = load(id).then((u) => render(u));\n  const w = async () => await p;\n}", "floating-then"},
	{"member-held promise", "class C {\n  pending: Promise<void> = Promise.resolve();\n  start() {\n    this.pending = load(id).then((u) => render(u));\n  }\n}", "floating-then"},
	{"rebound promise", "async function run() {\n  let p;\n  p = load(id).then((u) => render(u));\n}", "floating-then"},
	{"container-held promise", "async function run() {\n  const jobs: Record<string, Promise<void>> = {};\n  jobs[\"a\"] = load(id).then((u) => render(u));\n}", "floating-then"},
	// A try whose handler can skip the join, or whose body joins only at its end.
	{"join at the end of a try body with a handler", "async function run() {\n  const p = load(id).then((u) => render(u));\n  try {\n    work();\n    await p;\n  } catch (e) {\n    log(e);\n  }\n}", "floating-then"},
	{"rebound async call not awaited", "async function run() {\n  let p: Promise<void>;\n  p = flushAsync();\n}\nasync function flushAsync(): Promise<void> {\n  await work();\n}", "discarded-async-call"},
}

// typescriptDetectorClean are the shapes the TypeScript detector must accept.
var typescriptDetectorClean = []struct{ name, src string }{
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
	// A join that dominates on every path is accepted, in every spelling.
	{"held promise awaited in both try branches", "async function run() {\n  const p = load(id).then((u) => render(u));\n  try {\n    work();\n  } catch (e) {\n    await p;\n  }\n  await p;\n}"},
	{"join in finally dominates", "async function run(c) {\n  const p = load(id).then((u) => render(u));\n  try {\n    if (c) return 1;\n    return 2;\n  } finally {\n    await p;\n  }\n}"},
	{"if/else both branches join", "async function run(c) {\n  const p = load(id).then((u) => render(u));\n  if (c) {\n    await p;\n  } else {\n    await p;\n  }\n}"},
	{"await Promise.all over a held promise", "async function run() {\n  const p = load(id).then((u) => render(u));\n  await Promise.all([p]);\n}"},
	{"await Promise.allSettled over a held promise", "async function run() {\n  const p = load(id).then((u) => render(u));\n  await Promise.allSettled([p]);\n}"},
	{"held async-callback array joined", "async function run() {\n  const workers = Array.from({ length: 2 }, async () => { await work(); });\n  await Promise.allSettled(workers);\n}"},
	{"rebound promise awaited", "async function run() {\n  let p;\n  p = load(id).then((u) => render(u));\n  await p;\n}"},
	{"container-held promise awaited", "async function run() {\n  const jobs: Record<string, Promise<void>> = {};\n  jobs[\"a\"] = load(id).then((u) => render(u));\n  await jobs[\"a\"];\n}"},
	{"bind and await on one line", "async function run() {\n  const p = load(id).then((u) => render(u)); await p;\n}"},
}

// TestTypeScriptDetachedWorkDetectorIsNotVacuous proves the TypeScript detector
// fires on each banned idiom and stays quiet on the shapes the repository
// legitimately uses. One batch call parses every case with the TypeScript
// compiler API.
func TestTypeScriptDetachedWorkDetectorIsNotVacuous(t *testing.T) {
	sources := make([]astSource, 0, len(typescriptDetectorFlagged)+len(typescriptDetectorClean))
	for i, tc := range typescriptDetectorFlagged {
		sources = append(sources, astSource{Path: fmt.Sprintf("flagged-%02d.ts", i), Source: tc.src + "\n"})
	}
	for i, tc := range typescriptDetectorClean {
		sources = append(sources, astSource{Path: fmt.Sprintf("clean-%02d.ts", i), Source: tc.src + "\n"})
	}
	findings := scanTypeScriptBatch(t, sources)

	for i, tc := range typescriptDetectorFlagged {
		if !containsRule(findings[i], tc.rule) {
			t.Errorf("%s: detector missed %q (want rule %s); got %+v", tc.name, tc.src, tc.rule, findings[i])
		}
	}
	for i, tc := range typescriptDetectorClean {
		got := findings[len(typescriptDetectorFlagged)+i]
		if len(got) > 0 {
			t.Errorf("%s: detector false-positive on %q: %+v", tc.name, tc.src, got)
		}
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

// containsRule reports whether any finding carries the given rule.
func containsRule(findings []tsFinding, rule string) bool {
	for _, f := range findings {
		if f.Rule == rule {
			return true
		}
	}
	return false
}
