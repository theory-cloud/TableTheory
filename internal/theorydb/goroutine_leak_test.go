package theorydb

import (
	"context"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v4/pkg/session"
)

// This file encodes a repository invariant: no TableTheory init or handler path
// may leave work running after it returns. Lambda freezes the execution
// environment as soon as the handler returns, so a goroutine started during
// init may be frozen mid-flight and resume against an invocation that has
// already completed.
//
// The module has no goleak dependency, so the check is built on runtime.Stack:
// it snapshots every live goroutine before the call under test and then requires
// that no goroutine created by the call is still running afterwards.

var goroutineHeaderPattern = regexp.MustCompile(`(?m)^goroutine (\d+) \[`)

// goroutineStacksByID returns the stack trace of every live goroutine, keyed by
// goroutine ID. Go goroutine IDs are never reused, so an ID present before the
// call under test always identifies a pre-existing goroutine.
func goroutineStacksByID() map[int64]string {
	buf := make([]byte, 1<<20)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		buf = make([]byte, len(buf)*2)
	}

	matches := goroutineHeaderPattern.FindAllSubmatchIndex(buf, -1)
	stacks := make(map[int64]string, len(matches))
	for i, match := range matches {
		id, err := strconv.ParseInt(string(buf[match[2]:match[3]]), 10, 64)
		if err != nil {
			continue
		}
		end := len(buf)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		stacks[id] = string(buf[match[0]:end])
	}

	return stacks
}

// leakedGoroutines returns the stack of every goroutine that did not exist in
// before and whose stack mentions marker.
func leakedGoroutines(before map[int64]string, marker string) []string {
	var leaked []string
	for id, stack := range goroutineStacksByID() {
		if _, existed := before[id]; existed {
			continue
		}
		if strings.Contains(stack, marker) {
			leaked = append(leaked, stack)
		}
	}

	return leaked
}

// requireNoLeakedGoroutines fails the test if a goroutine matching marker is
// still running after the call under test returned. A goroutine may need a
// moment to unwind, so the check is retried until the deadline.
func requireNoLeakedGoroutines(t *testing.T, before map[int64]string, marker string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for {
		leaked := leakedGoroutines(before, marker)
		if len(leaked) == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("work outlived its invocation (matching %q):\n%s", marker, strings.Join(leaked, "\n"))
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// resetGlobalLambdaDB clears the process-wide Lambda instance so a test can
// exercise a fresh cold-start init.
func resetGlobalLambdaDB(t *testing.T) {
	t.Helper()

	globalLambdaDB = nil
	globalLambdaDBErr = nil
	lambdaOnce = sync.Once{}
	t.Cleanup(func() {
		globalLambdaDB = nil
		globalLambdaDBErr = nil
		lambdaOnce = sync.Once{}
	})
}

// TestGoroutineLeakDetectorFindsLeakedGoroutine proves the detector in this file
// is not vacuous: a deliberately leaked goroutine must be reported.
func TestGoroutineLeakDetectorFindsLeakedGoroutine(t *testing.T) {
	before := goroutineStacksByID()

	release := make(chan struct{})
	started := make(chan struct{})
	go func() {
		close(started)
		<-release
	}()
	<-started
	t.Cleanup(func() { close(release) })

	leaked := leakedGoroutines(before, "TestGoroutineLeakDetectorFindsLeakedGoroutine")
	require.Len(t, leaked, 1, "detector must see the deliberately leaked goroutine")
	require.Contains(t, leaked[0], "goroutine ")
}

// TestOptimizeForColdStart_LeavesNoDetachedWork covers the cold-start optimizer
// directly: it must return synchronously with no goroutine left behind.
func TestOptimizeForColdStart_LeavesNoDetachedWork(t *testing.T) {
	t.Setenv("AWS_LAMBDA_FUNCTION_NAME", "cold-start-test")

	httpClient := newCapturingHTTPClient(nil)
	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})

	dbAny, err := New(session.Config{Region: "us-east-1"})
	require.NoError(t, err)
	db := mustDB(t, dbAny)

	ldb := &LambdaDB{
		ExtendedDB:     db,
		db:             db,
		modelCache:     &sync.Map{},
		lambdaMemoryMB: 512,
		isLambda:       true,
	}

	before := goroutineStacksByID()
	ldb.OptimizeForColdStart()
	requireNoLeakedGoroutines(t, before, "internal/theorydb/lambda.go")

	// A detached pre-warm would have issued ListTables (which also needs IAM
	// permissions beyond item access) from a goroutine that may be frozen
	// mid-flight instead of ever completing.
	time.Sleep(50 * time.Millisecond)
	require.Zero(t, countRequestsByTarget(httpClient.Requests(), "DynamoDB_20120810.ListTables"))
	require.Empty(t, httpClient.Requests(), "cold-start optimization must not call DynamoDB")
}

// TestOptimizeForColdStart_NilReceiversAreNoOps keeps the optimizer safe on the
// zero and nil handles.
func TestOptimizeForColdStart_NilReceiversAreNoOps(t *testing.T) {
	require.NotPanics(t, func() { (*LambdaDB)(nil).OptimizeForColdStart() })
	require.NotPanics(t, func() { (&LambdaDB{}).OptimizeForColdStart() })
}

// TestLambdaInit_LeavesNoDetachedWork covers the blessed Lambda entry point end
// to end: LambdaInit must finish its work inside the init call.
func TestLambdaInit_LeavesNoDetachedWork(t *testing.T) {
	t.Setenv("AWS_LAMBDA_FUNCTION_NAME", "lambda-init-test")

	httpClient := newCapturingHTTPClient(nil)
	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})
	resetGlobalLambdaDB(t)

	before := goroutineStacksByID()

	db, err := LambdaInit(&cov4LambdaModel{})
	require.NoError(t, err)
	require.NotNil(t, db)

	requireNoLeakedGoroutines(t, before, "internal/theorydb/lambda.go")

	// LambdaInit must need no IAM permissions beyond the handler's own DynamoDB
	// access, so it issues no API call at all.
	time.Sleep(50 * time.Millisecond)
	require.Empty(t, httpClient.Requests(), "LambdaInit must not issue DynamoDB API calls")
}

// TestNewMultiAccount_LeavesNoDetachedWork covers the multi-account init path,
// which previously started a five-minute credential-refresh ticker goroutine.
func TestNewMultiAccount_LeavesNoDetachedWork(t *testing.T) {
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(nil), nil
	})
	resetGlobalLambdaDB(t)

	before := goroutineStacksByID()

	mdb, err := NewMultiAccount(map[string]AccountConfig{
		"partner": {
			RoleARN:         "arn:aws:iam::123456789012:role/PartnerRole",
			ExternalID:      "ext",
			Region:          "us-east-1",
			SessionDuration: time.Hour,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, mdb)

	requireNoLeakedGoroutines(t, before, "internal/theorydb/multiaccount.go")
	require.NoError(t, mdb.Close())
}

// TestMultiAccountPartner_RefreshesInsideTheInvocation covers the replacement
// for the removed background ticker: the renewal sweep runs synchronously on the
// Partner() path, so the refresh completes before Partner() returns.
func TestMultiAccountPartner_RefreshesInsideTheInvocation(t *testing.T) {
	partnerID := "partner"
	mdb := &MultiAccountDB{
		cache: &sync.Map{},
		accounts: map[string]AccountConfig{
			partnerID: {
				RoleARN:         "arn:aws:iam::123456789012:role/PartnerRole",
				ExternalID:      "ext",
				Region:          "us-east-1",
				SessionDuration: time.Hour,
			},
		},
		baseConfig: minimalAWSConfig(nil),
	}
	mdb.cache.Store(partnerID, &cacheEntry{
		db:         &LambdaDB{},
		expiry:     time.Now().Add(-time.Hour),
		partnerID:  partnerID,
		accountCfg: mdb.accounts[partnerID],
	})

	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(nil), nil
	})

	before := goroutineStacksByID()

	db, err := mdb.Partner(partnerID)
	require.NoError(t, err)
	require.NotNil(t, db)

	// The renewal sweep is synchronous, so the refreshed entry is already
	// visible when Partner() returns.
	value, ok := mdb.cache.Load(partnerID)
	require.True(t, ok)
	entry, ok := value.(*cacheEntry)
	require.True(t, ok)
	require.True(t, entry.expiry.After(time.Now()), "Partner() must refresh the expired entry before returning")

	requireNoLeakedGoroutines(t, before, "internal/theorydb/multiaccount.go")
}
