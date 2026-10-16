package theorydb

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/stretchr/testify/require"

	"github.com/theory-cloud/tabletheory/v6/pkg/core"
)

// v6LambdaRecord is a minimal model for the core-interface tests below.
type v6LambdaRecord struct {
	ID   string `theorydb:"pk,attr:id"`
	Name string `theorydb:"attr:name"`
}

func (v6LambdaRecord) TableName() string { return "v6_lambda_records" }

// TestLambdaInitHandleSatisfiesCoreExtendedDB reproduces the reported Lambda
// cold-start failure and pins the fix.
//
// *LambdaDB embeds core.ExtendedDB but declares its own WithLambdaTimeout; while
// that declared method returned *LambdaDB, the concrete result shadowed the
// promoted interface method and a `handle.(core.ExtendedDB)` assertion against a
// LambdaInit handle failed. The handle must now satisfy both public interfaces
// and serve reads and transactions through them.
func TestLambdaInitHandleSatisfiesCoreExtendedDB(t *testing.T) {
	httpClient := newCapturingHTTPClient(map[string]string{
		"DynamoDB_20120810.TransactWriteItems": `{}`,
	})
	stubSessionConfigLoad(t, func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return minimalAWSConfig(httpClient), nil
	})
	resetGlobalLambdaDB(t)

	ldb, err := LambdaInit(&v6LambdaRecord{})
	require.NoError(t, err)
	require.NotNil(t, ldb)

	// core.DB is satisfied at compile time.
	var asDB core.DB = ldb
	require.NotNil(t, asDB)

	// core.ExtendedDB is satisfied at runtime: this is the assertion that failed
	// at cold start before WithLambdaTimeout returned the interface type.
	handle, ok := any(ldb).(core.ExtendedDB)
	require.True(t, ok, "LambdaInit handle %T must satisfy core.ExtendedDB", ldb)

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(30*time.Second))
	t.Cleanup(cancel)

	// The deadline-scoped handle reached through the interface is still the
	// configured *LambdaDB.
	invocationDB := handle.WithLambdaTimeout(ctx)
	require.IsType(t, &LambdaDB{}, invocationDB)

	// TransactWrite is promoted from the embedded core.ExtendedDB and must work
	// through the interface-typed handle.
	require.NoError(t, handle.TransactWrite(ctx, func(tb core.TransactionBuilder) error {
		tb.Put(&v6LambdaRecord{ID: "record-1", Name: "alpha"})
		return nil
	}))
	require.Contains(t, capturedTargets(httpClient), "DynamoDB_20120810.TransactWriteItems")

	// Reads work through the derived core.DB handle.
	var rows []v6LambdaRecord
	require.NoError(t, invocationDB.Model(&v6LambdaRecord{}).Where("ID", "=", "record-1").All(&rows))
}

// TestLambdaDBWithLambdaTimeoutThroughInterfaceKeepsSemantics pins scenario (2):
// changing the declared result type must not change LambdaDB's timeout behavior.
// The interface-typed result is the configured *LambdaDB, and the deadline plus
// the 1s default buffer are applied exactly as before.
func TestLambdaDBWithLambdaTimeoutThroughInterfaceKeepsSemantics(t *testing.T) {
	db := &DB{}
	ldb := &LambdaDB{ExtendedDB: db, db: db, modelCache: &sync.Map{}}

	// No deadline: the same instance comes back, through core.DB.
	noDeadline := ldb.WithLambdaTimeout(context.Background())
	require.IsType(t, &LambdaDB{}, noDeadline)
	require.Same(t, ldb, noDeadline)

	deadline := time.Now().Add(5 * time.Second)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	t.Cleanup(cancel)

	// Deadline present: a new *LambdaDB carries the context, the hard deadline,
	// the shared model cache, and the LambdaDB default one-second buffer.
	timed := mustLambdaDB(t, ldb.WithLambdaTimeout(ctx))
	require.NotSame(t, ldb, timed)
	require.Equal(t, ctx, timed.db.ctx)
	require.WithinDuration(t, deadline, timed.db.lambdaDeadline, 25*time.Millisecond)
	require.Equal(t, defaultLambdaDBTimeoutBuffer, timed.db.lambdaTimeoutBuffer)
	require.Same(t, ldb.modelCache, timed.modelCache)

	// The effective stop window is applied exactly once: a 5s deadline with a 1s
	// buffer is still 4s away, so the check passes.
	require.NoError(t, (&queryExecutor{db: timed.db}).checkLambdaTimeout())

	// WithLambdaTimeoutConfig is not part of an interface and keeps returning the
	// concrete *LambdaDB, so a configured buffer still survives the (now
	// interface-typed) timeout derivation.
	configured := ldb.WithLambdaTimeoutConfig(LambdaTimeoutConfig{Buffer: 500 * time.Millisecond})
	require.Equal(t, 500*time.Millisecond, configured.db.lambdaTimeoutBuffer)
	require.Equal(t, 500*time.Millisecond,
		mustLambdaDB(t, configured.WithLambdaTimeout(ctx)).db.lambdaTimeoutBuffer)
}

// capturedTargets returns the X-Amz-Target of each captured request in order.
func capturedTargets(c *capturingHTTPClient) []string {
	requests := c.Requests()
	targets := make([]string, 0, len(requests))
	for _, request := range requests {
		targets = append(targets, request.Target)
	}
	return targets
}
