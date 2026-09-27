package tests

import (
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// libraryDetachedWorkAllowlist lists library launches that are genuinely
// long-lived and therefore bypass the join recogniser. Keys are
// "<relative path>:<line>"; the guard fails when a key stops matching a real
// launch. Every entry is reported in the PR body.
var libraryDetachedWorkAllowlist = launchAllowlist{
	"pkg/protection/resource_limiter.go:312": "intentional long-lived monitor: MemoryMonitor.Start launches monitorLoop, which runs until Stop() closes stopChan; it is not joined before Start returns, so a caller that never calls StopMemoryMonitoring before its Lambda handler returns would leave it frozen mid-flight. It is allowlisted rather than removed because it is an opt-in, explicitly lifecycle-managed background service (Start/Stop) that no library code path starts on its own — NewResourceProtector does not call Start.",
}

// TestLibrary_NoDetachedWorkInGoSources fails when a library goroutine is not
// provably joined before the function that launched it returns. This is the
// same invariant the examples guard enforces, applied to the code every
// consumer compiles in: library code also runs inside a Lambda invocation, so a
// goroutine that outlives the handler would be frozen against an invocation
// that is already over.
//
// A launch is accepted only when goLaunchJoined can prove the caller waits for
// it (WaitGroup Add/Wait + Done, errgroup Wait + Done, or a channel the caller
// drains after the launch that the goroutine writes or closes). Everything else
// must be justified line by line in libraryDetachedWorkAllowlist.
func TestLibrary_NoDetachedWorkInGoSources(t *testing.T) {
	root := detachedWorkRepoRoot(t)
	reportAllowlist(t, libraryDetachedWorkAllowlist)

	scanned := 0
	launches := 0
	recognised := 0
	scannedKeys := map[string]bool{}
	var problems []string

	visit := func(abs, rel string) error {
		src, err := os.ReadFile(abs)
		if err != nil {
			return err
		}
		scanned++
		sites, err := scanGoSource(token.NewFileSet(), rel, src)
		if err != nil {
			t.Errorf("parse %s: %v", rel, err)
			return nil
		}
		for _, site := range sites {
			launches++
			key := launchKey(rel, site.Line)
			scannedKeys[key] = true
			if libraryDetachedWorkAllowlist.allowlisted(rel, site.Line) {
				continue
			}
			if site.Joined {
				recognised++
				t.Logf("joined launch: %s:%d (%s) — %s", site.Rel, site.Line, site.Func, site.Evidence)
				continue
			}
			problems = append(problems, describeLaunch(site))
		}
		return nil
	}

	// Root-level package sources.
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read repo root: %v", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if err := visit(filepath.Join(root, name), name); err != nil {
			t.Fatalf("scan %s: %v", name, err)
		}
	}

	// Library subpackages.
	include := func(rel string) bool {
		return strings.HasSuffix(rel, ".go") && !strings.HasSuffix(rel, "_test.go")
	}
	walkDetachedWorkSources(t, root, []string{"pkg", "internal", "cmd"}, include, visit)

	if scanned == 0 {
		t.Fatal("guard is vacuous: no library Go files were scanned")
	}
	if launches == 0 {
		t.Fatal("guard is vacuous: no library goroutine launches were detected at all")
	}
	if recognised == 0 {
		t.Fatal("guard is vacuous: no library launch was recognised as joined; the join analyser may be broken")
	}
	libraryDetachedWorkAllowlist.checkAllowlistCoverage(t, scannedKeys)
	if len(problems) > 0 {
		sort.Strings(problems)
		t.Fatalf(
			"library code must not start work that can outlive the invocation that started it.\n"+
				"Join the goroutine before returning, or add the exact line to libraryDetachedWorkAllowlist with a justification:\n%s",
			strings.Join(problems, "\n"),
		)
	}
}

// TestLibraryDetachedWorkDetectorIsNotVacuous proves the join recogniser is not
// vacuous and is conservative: it must accept WaitGroup, errgroup, and drained
// channel joins, and it must reject a launch whose Wait ran before the launch,
// a launch whose goroutine never signals Done, and a channel that is only ever
// written inside the goroutine.
func TestLibraryDetachedWorkDetectorIsNotVacuous(t *testing.T) {
	const src = `package sample

import (
	"sync"

	"golang.org/x/sync/errgroup"
)

func waitgroupJoin() {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
	}()
	wg.Wait()
}

func errgroupJoin() {
	var g errgroup.Group
	g.Go(func() error {
		return nil
	})
	_ = g.Wait()
}

func channelDrainJoin() {
	out := make(chan int)
	go func() {
		defer close(out)
		out <- 1
	}()
	for range out {
	}
}

func orphanLaunch() {
	go func() {
		doWork()
	}()
}

func waitBeforeLaunch() {
	var wg sync.WaitGroup
	wg.Add(1)
	wg.Wait()
	go func() {
		defer wg.Done()
	}()
}

func channelOnlyWrittenInside() {
	out := make(chan int)
	go func() {
		out <- 1
	}()
}

func doWork() {}
`
	sites, err := scanGoSource(token.NewFileSet(), "sample.go", []byte(src))
	if err != nil {
		t.Fatalf("parse sample: %v", err)
	}

	type expectation struct {
		funcName string
		joined   bool
	}
	want := map[string]bool{
		"waitgroupJoin":            true,
		"errgroupJoin":             true,
		"channelDrainJoin":         true,
		"orphanLaunch":             false,
		"waitBeforeLaunch":         false,
		"channelOnlyWrittenInside": false,
	}

	got := map[string]bool{}
	for _, site := range sites {
		got[site.Func] = site.Joined
	}
	if len(sites) != len(want) {
		t.Fatalf("detector found %d launches, want %d: %+v", len(sites), len(want), sites)
	}
	for fn, wantJoined := range want {
		gotJoined, ok := got[fn]
		if !ok {
			t.Errorf("detector missed the launch inside %s", fn)
			continue
		}
		if gotJoined != wantJoined {
			t.Errorf("%s: joined=%v, want %v (evidence must be provable, not assumed)", fn, gotJoined, wantJoined)
		}
	}
}
