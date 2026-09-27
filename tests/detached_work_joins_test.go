package tests

import (
	"go/token"
	"testing"
)

// This file pins the Go half of the detached-work guard: which launch shapes
// count as joined, which do not, and which timer APIs are reported. The cases
// named "bypass" are the shapes that a purely textual "a Wait appears after the
// launch" rule accepts even though the goroutine can outlive the function; each
// one must be reported as unjoined.

// scanSample classifies every finding in a sample source by the function that
// owns it.
func scanSample(t *testing.T, src string) map[string]goLaunchSite {
	t.Helper()
	sites, err := scanGoSource(token.NewFileSet(), "sample.go", []byte(src))
	if err != nil {
		t.Fatalf("parse sample: %v", err)
	}
	byFunc := map[string]goLaunchSite{}
	for _, site := range sites {
		byFunc[site.Func] = site
	}
	return byFunc
}

// TestGoJoinAnalyzerRequiresDominance proves the join rules accept every join
// idiom this repository uses and report each bypass that a mere "Wait comes
// later" check would wrongly bless.
func TestGoJoinAnalyzerRequiresDominance(t *testing.T) {
	const src = `package sample

import (
	"sync"

	"golang.org/x/sync/errgroup"
)

func waitGroupJoin() {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
	}()
	wg.Wait()
}

func loopLaunchWaitAfter() {
	var wg sync.WaitGroup
	for _, x := range []int{1, 2} {
		wg.Add(1)
		go func(v int) {
			defer wg.Done()
			_ = v
		}(x)
	}
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

func closeSignalJoin() {
	done := make(chan struct{})
	go func() {
		defer close(done)
	}()
	select {
	case <-done:
		return
	default:
		<-done
	}
}

func earlyReturnBypass(abort bool) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
	}()
	if abort {
		return
	}
	wg.Wait()
}

func conditionalWaitBypass(cond bool) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
	}()
	if cond {
		wg.Wait()
	}
}

func uncalledClosureWaitBypass() {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
	}()
	cleanup := func() {
		wg.Wait()
	}
	_ = cleanup
}

func multiSendSingleReceiveBypass() {
	out := make(chan int)
	go func() {
		defer close(out)
		for i := 0; i < 100; i++ {
			out <- i
		}
	}()
	<-out
}

func bufferedReceiveBypass() {
	done := make(chan struct{}, 1)
	go func() {
		close(done)
	}()
	<-done
}

func closeThenWorkBypass() {
	done := make(chan struct{})
	go func() {
		close(done)
		doWork()
	}()
	<-done
}

func gotoBypass(skip bool) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
	}()
	if skip {
		goto done
	}
	wg.Wait()
done:
	return
}

func addOutsideLaunchListBypass() {
	var wg sync.WaitGroup
	wg.Add(1)
	for _, x := range []int{1} {
		go func(v int) {
			defer wg.Done()
			_ = v
		}(x)
	}
	wg.Wait()
}

func deferredJoinValid() {
	var wg sync.WaitGroup
	wg.Add(1)
	defer wg.Wait()
	go func() {
		defer wg.Done()
	}()
}

func deferredJoinAfterReturnBypass(cond bool) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
	}()
	if cond {
		return
	}
	defer wg.Wait()
}

func conditionalDeferBypass(cond bool) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
	}()
	if cond {
		defer wg.Wait()
	}
}

func namedFunctionLaunch() {
	go doWork()
}

func doWork() {}
`

	want := map[string]bool{
		// Recognized joins.
		"waitGroupJoin":       true,
		"loopLaunchWaitAfter": true,
		"errgroupJoin":        true,
		"channelDrainJoin":    true,
		"closeSignalJoin":     true,
		"deferredJoinValid":   true,
		// Bypasses that must be reported as unjoined.
		"earlyReturnBypass":             false,
		"conditionalWaitBypass":         false,
		"uncalledClosureWaitBypass":     false,
		"multiSendSingleReceiveBypass":  false,
		"bufferedReceiveBypass":         false,
		"closeThenWorkBypass":           false,
		"gotoBypass":                    false,
		"addOutsideLaunchListBypass":    false,
		"deferredJoinAfterReturnBypass": false,
		"conditionalDeferBypass":        false,
		"namedFunctionLaunch":           false,
	}

	found := scanSample(t, src)
	if len(found) != len(want) {
		t.Fatalf("analyzer found %d findings, want %d: %v", len(found), len(want), found)
	}
	for fn, wantJoined := range want {
		site, ok := found[fn]
		if !ok {
			t.Errorf("analyzer missed the launch in %s", fn)
			continue
		}
		if site.Joined != wantJoined {
			t.Errorf("%s: joined=%v, want %v (evidence: %s)", fn, site.Joined, wantJoined, site.Evidence)
		}
		if wantJoined && site.Evidence == "" {
			t.Errorf("%s: accepted as joined with no evidence", fn)
		}
	}
}

// TestGoDetachedTimerDetector proves the timer half of the Go detector: a timer
// that schedules work and is never stopped is reported, and a timer that is
// stopped in the same function is not.
func TestGoDetachedTimerDetector(t *testing.T) {
	const src = `package sample

import "time"

func afterFuncUnmanaged() {
	time.AfterFunc(time.Second, func() {})
}

func tickUnmanaged() {
	for range time.Tick(time.Second) {
		break
	}
}

func newTimerStopped() {
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	<-timer.C
}

func newTickerUnstopped() {
	ticker := time.NewTicker(time.Second)
	<-ticker.C
}

func newTimerDiscarded() {
	<-time.NewTimer(time.Second).C
}

func newTimerStoppedInSelect() {
	timer := time.NewTimer(time.Second)
	select {
	case <-timer.C:
	case <-time.After(0):
		timer.Stop()
	}
}
`

	want := map[string]bool{
		"afterFuncUnmanaged":      true,
		"tickUnmanaged":           true,
		"newTimerStopped":         false,
		"newTickerUnstopped":      true,
		"newTimerDiscarded":       true,
		"newTimerStoppedInSelect": false,
	}

	flagged := 0
	for _, wantFlagged := range want {
		if wantFlagged {
			flagged++
		}
	}
	found := scanSample(t, src)
	if len(found) != flagged {
		t.Fatalf("detector found %d timer findings, want %d: %v", len(found), flagged, found)
	}
	for fn, wantFlagged := range want {
		site, ok := found[fn]
		if !ok {
			if wantFlagged {
				t.Errorf("detector missed the unmanaged timer in %s", fn)
			}
			continue
		}
		if site.Kind != "detached-timer" {
			t.Errorf("%s: kind=%q, want detached-timer", fn, site.Kind)
		}
		if wantFlagged && site.Joined {
			t.Errorf("%s: timer reported as joined, but nothing stops it", fn)
		}
		if !wantFlagged {
			t.Errorf("%s: stopped timer reported as detached: %s", fn, site.Evidence)
		}
	}
}
