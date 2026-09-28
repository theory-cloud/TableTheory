package tests

import (
	"go/token"
	"sort"
	"strings"
	"testing"
)

// libraryDetachedWorkAllowlist lists library launches that are permitted to run
// detached. It is intentionally empty: no goroutine a library function starts
// may outlive the invocation that started it, and no library launch has a
// justified exception. Keys are "<relative path>:<line>"; the guard fails when
// a key stops matching a real launch, and every entry would be reported in the
// PR body.
var libraryDetachedWorkAllowlist = launchAllowlist{}

// TestLibrary_NoDetachedWorkInGoSources fails when Go code we ship or run
// starts work that can outlive the invocation that started it: the root
// package, pkg/, internal/, cmd/, the helper programs under scripts/, and the
// contract-test runners. Library code also runs inside a Lambda invocation, so
// a goroutine frozen mid-flight at handler return is the same defect whether it
// was launched by a consumer's handler or by a helper we run ourselves.
//
// A launch is accepted only when goLaunchJoined shows the join dominating every
// return path (WaitGroup Add-before/Done-inside/Wait-after, errgroup Wait-after,
// or an unbuffered close-signal channel drained after the launch). A timer that
// nobody stops is reported too. Everything else must be justified line by line
// in libraryDetachedWorkAllowlist, which is empty and must stay empty.
func TestLibrary_NoDetachedWorkInGoSources(t *testing.T) {
	root := detachedWorkRepoRoot(t)
	reportAllowlist(t, libraryDetachedWorkAllowlist)

	scanned := 0
	findings := 0
	recognized := 0
	scannedKeys := map[string]bool{}
	var problems []string

	visit := func(rel string, src []byte) error {
		scanned++
		sites, err := scanGoSource(token.NewFileSet(), rel, src)
		if err != nil {
			t.Errorf("parse %s: %v", rel, err)
			return nil
		}
		for _, site := range sites {
			findings++
			key := launchKey(rel, site.Line)
			scannedKeys[key] = true
			if libraryDetachedWorkAllowlist.allowlisted(rel, site.Line) {
				continue
			}
			if site.Joined {
				recognized++
				t.Logf("joined launch: %s:%d (%s) — %s", site.Rel, site.Line, site.Func, site.Evidence)
				continue
			}
			problems = append(problems, describeLaunch(site))
		}
		return nil
	}

	// One walk of the repository root, keeping the root package sources (depth
	// 1, which is why the walk starts at ".") and the Go surfaces we ship or
	// run: pkg/, internal/, cmd/, scripts/, and the contract-test runners.
	include := func(rel string) bool {
		if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") {
			return false
		}
		switch {
		case !strings.Contains(rel, "/"):
			return true
		case strings.HasPrefix(rel, "pkg/"),
			strings.HasPrefix(rel, "internal/"),
			strings.HasPrefix(rel, "cmd/"),
			strings.HasPrefix(rel, "scripts/"),
			strings.HasPrefix(rel, "contract-tests/runners/"):
			return true
		default:
			return false
		}
	}
	walkDetachedWorkSources(t, root, []string{"."}, include, visit)

	if scanned == 0 {
		t.Fatal("guard is vacuous: no library Go files were scanned")
	}
	if findings == 0 {
		t.Fatal("guard is vacuous: no goroutine launch or timer was detected at all")
	}
	if recognized == 0 {
		t.Fatal("guard is vacuous: no launch was recognized as joined; the join analyzer may be broken")
	}
	libraryDetachedWorkAllowlist.checkAllowlistCoverage(t, scannedKeys)
	if len(problems) > 0 {
		sort.Strings(problems)
		t.Fatalf(
			"Go code we ship or run must not start work that can outlive the invocation that started it.\n"+
				"Join the goroutine before returning, stop the timer, or add the exact line to libraryDetachedWorkAllowlist with a justification:\n%s",
			strings.Join(problems, "\n"),
		)
	}
}
