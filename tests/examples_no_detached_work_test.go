package tests

import (
	"go/token"
	"os"
	"sort"
	"strings"
	"testing"
)

// examplesDetachedWorkAllowlist lists example launches that may outlive their
// caller because the example really is a long-lived process rather than a
// Lambda handler. Keys are "<relative path>:<line>", so each entry justifies
// one exact launch; the guard fails if a key stops matching a real launch.
var examplesDetachedWorkAllowlist = launchAllowlist{
	"examples/multi-tenant/cmd/local/main.go:69": "local dev HTTP server, not a Lambda handler: main starts ListenAndServe in this goroutine and then blocks on SIGINT/SIGTERM before a graceful shutdown, so the server never outlives the process that owns it",
}

// TestExamples_NoDetachedWorkInLambdaEntrypoints fails when an example starts
// work that can outlive the invocation that started it. The blog example
// deploys only as Lambda functions and the payment example only as Lambda
// handlers, so neither may fork a goroutine; the local dev server is the sole
// reviewed exception. Launches are found with go/ast, so the `; go f()` form
// and any other placement is caught rather than only a line-initial `go`.
func TestExamples_NoDetachedWorkInLambdaEntrypoints(t *testing.T) {
	root := detachedWorkRepoRoot(t)
	reportAllowlist(t, examplesDetachedWorkAllowlist)

	scanned := 0
	scannedKeys := map[string]bool{}
	var problems []string

	include := func(rel string) bool {
		return strings.HasSuffix(rel, ".go") && !strings.HasSuffix(rel, "_test.go")
	}
	walkDetachedWorkSources(t, root, []string{"examples"}, include, func(abs, rel string) error {
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
			key := launchKey(rel, site.Line)
			scannedKeys[key] = true
			if examplesDetachedWorkAllowlist.allowlisted(rel, site.Line) {
				continue
			}
			problems = append(problems, describeLaunch(site))
		}
		return nil
	})

	if scanned == 0 {
		t.Fatal("guard is vacuous: no example Go files were scanned")
	}
	examplesDetachedWorkAllowlist.checkAllowlistCoverage(t, scannedKeys)
	if len(problems) > 0 {
		sort.Strings(problems)
		t.Fatalf(
			"examples must not start work that can outlive a Lambda invocation.\n"+
				"Deliver it on the invocation path instead, or add the exact line to examplesDetachedWorkAllowlist with a justification:\n%s",
			strings.Join(problems, "\n"),
		)
	}
}

// TestExamplesDetachedWorkDetectorIsNotVacuous proves the AST detector is not
// vacuous: it must find real launches — including the same-line `; go f()` form
// that a line-anchored regex misses — it must recognise a WaitGroup join and a
// channel-drain join, it must refuse to call a launch joined when the Wait runs
// before it, and it must not invent launches out of directives, comments,
// prose, or string literals.
func TestExamplesDetachedWorkDetectorIsNotVacuous(t *testing.T) {
	const src = `//go:build linux && amd64
//go:embed demo.yml

package sample

import "sync"

const note = "go worker(i) is deliberately absent"

func joinedDirect() {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
	}()
	wg.Wait()
}

func joinedChannel() {
	done := make(chan struct{})
	go func() {
		defer close(done)
	}()
	<-done
}

func waitBeforeLaunch() {
	var wg sync.WaitGroup
	wg.Wait()
	wg.Add(1)
	go func() {
		defer wg.Done()
	}()
}

func sameLine() {
	ready := true
	if ready {
		_ = ready; go worker()
	}
}

func methodValue() {
	go s.worker(2)
}
`
	sites, err := scanGoSource(token.NewFileSet(), "sample.go", []byte(src))
	if err != nil {
		t.Fatalf("parse sample: %v", err)
	}

	byText := map[string]bool{}
	for _, site := range sites {
		byText[site.Text] = site.Joined
	}

	// Nothing that is not a goroutine launch may be reported: the build and
	// embed directives, the comment-like prose, and the string literal above
	// must all be invisible to the detector.
	wantLaunches := []string{
		"go func() {",
		"_ = ready; go worker()",
		"go s.worker(2)",
	}
	for _, text := range wantLaunches {
		if _, ok := byText[text]; !ok {
			t.Errorf("detector missed the launch %q; found %v", text, launchTexts(sites))
		}
	}

	if len(sites) != 5 {
		t.Errorf("detector found %d launches, want 5: %v", len(sites), launchTexts(sites))
	}
	if strings.Contains(strings.Join(launchTexts(sites), "|"), "go worker(i)") {
		t.Error("detector matched a string literal as a goroutine launch")
	}

	// Same-line form must be recognised as a launch and as unjoined.
	if joined, ok := byText["_ = ready; go worker()"]; !ok || joined {
		t.Errorf("same-line `; go worker()`: found=%v joined=%v, want found=true joined=false", ok, joined)
	}
	if joined, ok := byText["go s.worker(2)"]; !ok || joined {
		t.Errorf("`go s.worker(2)`: found=%v joined=%v, want found=true joined=false", ok, joined)
	}

	// Two of the three `go func() {` launches are joined (WaitGroup, channel
	// drain) and one is not (Wait ran before the launch). Count them by join
	// evidence so the ordering rule is pinned.
	joined := 0
	for _, site := range sites {
		if site.Text == "go func() {" && site.Joined {
			joined++
		}
	}
	if joined != 2 {
		t.Errorf("recognised %d joined `go func() {` launches, want 2 (WaitGroup + channel drain)", joined)
	}
}

// launchTexts lists the source text of every detected launch, for diagnostics.
func launchTexts(sites []goLaunchSite) []string {
	out := make([]string, 0, len(sites))
	for _, site := range sites {
		out = append(out, site.Text)
	}
	return out
}
