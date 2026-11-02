package marshal_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	tabletheory "github.com/theory-cloud/tabletheory/v6"
	"github.com/theory-cloud/tabletheory/v6/pkg/core"
	"github.com/theory-cloud/tabletheory/v6/pkg/session"
	"github.com/theory-cloud/tabletheory/v6/pkg/testing/fakedb"
)

// lifecycleClocksPath is the shared lifecycle-clock table the Go, TypeScript
// and Python suites all consume. It lives beside the nil/empty table so a single
// file states these semantics for every runtime.
const lifecycleClocksPath = "../../contract-tests/serialization/lifecycle-clocks.yml"

type lifecycleClocksFixture struct {
	Name         string                      `yaml:"name"`
	WriterPaths  []string                    `yaml:"writer_paths"`
	Runtimes     []string                    `yaml:"runtimes"`
	ClockModes   []string                    `yaml:"clock_modes"`
	Stamps       map[string]string           `yaml:"stamps"`
	Expectations []string                    `yaml:"expectations"`
	Rows         []lifecycleClocksFixtureRow `yaml:"rows"`
}

type lifecycleClocksFixtureRow struct {
	Expect               map[string]map[string]string `yaml:"expect"`
	PutEqualsTransact    map[string]bool              `yaml:"put_equals_transact_write"`
	ExactEqualitySupport map[string]bool              `yaml:"exact_equality_supported"`
	Mode                 string                       `yaml:"mode"`
}

// lifecycleClocksItem is the lifecycle model the fixture scenarios drive. The
// field order mirrors the production model shape the fixture stands for.
//
//nolint:govet // Field order mirrors the production model under test.
type lifecycleClocksItem struct {
	PK        string    `theorydb:"pk,attr:PK" json:"PK"`
	SK        string    `theorydb:"sk,attr:SK" json:"SK"`
	CreatedAt time.Time `theorydb:"created_at,attr:createdAt" json:"createdAt"`
	UpdatedAt time.Time `theorydb:"updated_at,attr:updatedAt" json:"updatedAt"`
}

func (lifecycleClocksItem) TableName() string { return "lifecycle_clocks" }

func newLifecycleClocksItem() *lifecycleClocksItem {
	return &lifecycleClocksItem{PK: "p1", SK: "s1"}
}

// lifecycleClock is the one scenario clock both writer calls read, so an
// advancing clock sees the same sequence the fixture describes.
type lifecycleClock struct {
	stamps []time.Time
	mu     sync.Mutex
	calls  int
}

func (c *lifecycleClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	index := min(c.calls, len(c.stamps)-1)
	c.calls++
	return c.stamps[index]
}

func parseLifecycleStamp(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339Nano, value)
	require.NoError(t, err, "fixture stamp %q must parse", value)
	return parsed
}

func newLifecycleClock(t *testing.T, mode string, stamps map[string]string) *lifecycleClock {
	t.Helper()
	fixed := parseLifecycleStamp(t, stamps["fixed"])
	first := parseLifecycleStamp(t, stamps["advancing_first"])
	second := parseLifecycleStamp(t, stamps["advancing_second"])

	switch mode {
	case "fixed":
		return &lifecycleClock{stamps: []time.Time{fixed}}
	case "advancing":
		return &lifecycleClock{stamps: []time.Time{first, second}}
	case "missing", "":
		return nil
	default:
		t.Fatalf("unknown clock mode %q", mode)
		return nil
	}
}

func loadLifecycleClocks(t *testing.T) lifecycleClocksFixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.FromSlash(lifecycleClocksPath))
	require.NoError(t, err, "the shared lifecycle-clock table must exist at %s", lifecycleClocksPath)

	var fixture lifecycleClocksFixture
	require.NoError(t, yaml.Unmarshal(raw, &fixture))
	require.NotEmpty(t, fixture.Rows)
	return fixture
}

func lifecycleSessionConfig(clock *lifecycleClock) session.Config {
	cfg := session.Config{Region: "us-east-1"}
	if clock != nil {
		cfg.Now = clock.Now
	}
	return cfg
}

// captureLifecyclePut records what Model().Create really stores.
func captureLifecyclePut(t *testing.T, clock *lifecycleClock) map[string]types.AttributeValue {
	t.Helper()
	fake := fakedb.New()
	db, err := tabletheory.NewWithClient(lifecycleSessionConfig(clock), fake)
	require.NoError(t, err)
	require.NoError(t, db.Model(newLifecycleClocksItem()).Create())

	items := fake.Items("lifecycle_clocks")
	require.Len(t, items, 1, "the fake must have stored exactly one put item")
	return items[0]
}

// captureLifecycleTransact records what TransactWrite with a real transaction
// builder really stores.
func captureLifecycleTransact(t *testing.T, clock *lifecycleClock) map[string]types.AttributeValue {
	t.Helper()
	fake := fakedb.New()
	db, err := tabletheory.NewWithClient(lifecycleSessionConfig(clock), fake)
	require.NoError(t, err)
	require.NoError(t, db.TransactWrite(context.Background(), func(tx core.TransactionBuilder) error {
		tx.Create(newLifecycleClocksItem())
		return nil
	}))

	items := fake.Items("lifecycle_clocks")
	require.Len(t, items, 1, "the fake must have stored exactly one transacted item")
	return items[0]
}

// lifecycleStampToken names the stored lifecycle value in the fixture's
// vocabulary instead of comparing against a value the test invented.
func lifecycleStampToken(av types.AttributeValue, stamps map[string]string) string {
	text, ok := av.(*types.AttributeValueMemberS)
	if !ok {
		return "unstamped"
	}
	named := map[string]string{
		"stamp_fixed":  stamps["fixed"],
		"stamp_first":  stamps["advancing_first"],
		"stamp_second": stamps["advancing_second"],
	}
	for token, value := range named {
		if text.Value == value {
			return token
		}
	}
	if text.Value == "0001-01-01T00:00:00Z" {
		// The zero time.Time is what the transactional writer stores for a
		// lifecycle field: that path does not stamp at all.
		return "unstamped"
	}
	return "wall_clock"
}

// TestSharedLifecycleClockTableGoColumn asserts the Go column of the shared
// lifecycle-clock table against payloads captured from both real writer paths.
// Every row must be mapped, so a dropped or renamed row fails the build instead
// of silently shrinking the pin.
func TestSharedLifecycleClockTableGoColumn(t *testing.T) {
	fixture := loadLifecycleClocks(t)

	require.Contains(t, fixture.Runtimes, "go")
	require.Equal(t, []string{"put", "transact_write"}, fixture.WriterPaths)
	require.Equal(t, []string{"fixed", "advancing", "missing"}, fixture.ClockModes)
	require.Contains(t, fixture.Expectations, "unstamped")
	require.Len(t, fixture.Rows, len(fixture.ClockModes),
		"the shared table must declare one row per clock mode")

	for _, row := range fixture.Rows {
		t.Run(row.Mode, func(t *testing.T) {
			clock := newLifecycleClock(t, row.Mode, fixture.Stamps)
			put := captureLifecyclePut(t, clock)
			transact := captureLifecycleTransact(t, clock)

			wantPut, ok := row.Expect["put"]["go"]
			require.True(t, ok, "row %s declares no go put expectation", row.Mode)
			wantTransact, ok := row.Expect["transact_write"]["go"]
			require.True(t, ok, "row %s declares no go transact_write expectation", row.Mode)

			assert.Equal(t, wantPut, lifecycleStampToken(put["createdAt"], fixture.Stamps),
				"put createdAt for the %s clock", row.Mode)
			assert.Equal(t, wantTransact, lifecycleStampToken(transact["createdAt"], fixture.Stamps),
				"transact_write createdAt for the %s clock", row.Mode)
			assert.Equal(t, put["createdAt"], put["updatedAt"],
				"both lifecycle fields of one put must come from the same clock read")
			assert.Equal(t, transact["createdAt"], transact["updatedAt"],
				"both lifecycle fields of one transactional put must agree")

			equal := reflect.DeepEqual(put, transact)
			if wantEqual, declared := row.PutEqualsTransact["go"]; declared {
				assert.Equal(t, wantEqual, equal,
					"the go writer paths store %s bytes under the %s clock", row.Mode, row.Mode)
			}
			assert.Equal(t, row.ExactEqualitySupport["go"], equal,
				"go can only support an exact-equality claim when both paths store the same bytes")
		})
	}
}
