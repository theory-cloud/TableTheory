package contracttests

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	theorydbErrors "github.com/theory-cloud/tabletheory/v5/pkg/errors"

	"github.com/theory-cloud/tabletheory-contract-tests/runners/go/internal/driver"
	"github.com/theory-cloud/tabletheory-contract-tests/runners/go/internal/runner"
)

// TestContract_TransactionCancellation drives the shared cancellation fixture
// through the in-memory DynamoDB seam so Go, TypeScript and Python assert the
// exact same outcomes from one fixture. It needs no DynamoDB Local.
func TestContract_TransactionCancellation(t *testing.T) {
	t.Helper()

	root, err := runner.RepoRootFromModuleDir()
	require.NoError(t, err)

	fixturePath := filepath.Join(root, "contract-tests", "scenarios", "cancellation", "01-transaction-cancellation.yml")
	raw, err := os.ReadFile(fixturePath)
	require.NoError(t, err)

	var fixture cancellationFixture
	require.NoError(t, yaml.Unmarshal(raw, &fixture))
	require.NotEmpty(t, fixture.Cases, "fixture must declare cancellation cases")

	ctx := context.Background()
	for _, testCase := range fixture.Cases {
		testCase := testCase
		t.Run(testCase.Name, func(t *testing.T) {
			drv, fake, err := driver.NewFakeTheorydbDriver()
			require.NoError(t, err)

			switch testCase.Kind {
			case "cancellation":
				fake.QueueTransactionCancellation(cancellationReasons(testCase.Reasons))
			case "transport":
				fake.QueueTransactionError(errors.New(testCase.ErrorMessage))
			default:
				t.Fatalf("unknown fixture kind %q", testCase.Kind)
			}

			transactErr := drv.TransactWrite(ctx, "User", []driver.TransactWriteAction{{
				Kind:  "put",
				Model: "User",
				Item:  map[string]any{"PK": "USER#1", "SK": "PROFILE", "emailHash": "hash_abc"},
			}})
			require.Error(t, transactErr)

			code, reasonCodes, causeRecoverable := cancellationOutcome(transactErr)
			require.Equal(t, testCase.Expect.Code, code, "mapped error code")
			require.Equal(t, normalizedStrings(testCase.Expect.ReasonCodes), normalizedStrings(reasonCodes), "ordered reason codes")
			require.Equal(t, testCase.Expect.CauseRecoverable, causeRecoverable, "original cancellation recoverable")
		})
	}
}

type cancellationFixture struct {
	Name  string                    `yaml:"name"`
	Cases []cancellationFixtureCase `yaml:"cases"`
}

type cancellationFixtureCase struct {
	Name         string                      `yaml:"name"`
	Kind         string                      `yaml:"kind"`
	Reasons      []cancellationFixtureReason `yaml:"reasons"`
	ErrorMessage string                      `yaml:"error_message"`
	Expect       cancellationExpectation     `yaml:"expect"`
}

type cancellationFixtureReason struct {
	Code    *string `yaml:"code"`
	Message *string `yaml:"message"`
}

type cancellationExpectation struct {
	Code             string   `yaml:"code"`
	ReasonCodes      []string `yaml:"reason_codes"`
	CauseRecoverable bool     `yaml:"cause_recoverable"`
}

func cancellationReasons(spec []cancellationFixtureReason) []types.CancellationReason {
	reasons := make([]types.CancellationReason, 0, len(spec))
	for _, entry := range spec {
		reason := types.CancellationReason{}
		if entry.Code != nil {
			reason.Code = entry.Code
		}
		if entry.Message != nil {
			reason.Message = entry.Message
		}
		reasons = append(reasons, reason)
	}
	return reasons
}

// cancellationOutcome reports the code, the ordered reason codes and whether the
// original AWS cancellation stays recoverable. It reads reasons from the
// TransactionError when present and otherwise from the preserved AWS exception.
func cancellationOutcome(err error) (string, []string, bool) {
	var canceled *types.TransactionCanceledException
	causeRecoverable := errors.As(err, &canceled)

	var transactionErr *theorydbErrors.TransactionError
	var codes []string
	switch {
	case errors.As(err, &transactionErr):
		for _, reason := range transactionErr.Reasons {
			codes = append(codes, reason.Code)
		}
	case causeRecoverable:
		for _, reason := range canceled.CancellationReasons {
			code := ""
			if reason.Code != nil {
				code = *reason.Code
			}
			codes = append(codes, code)
		}
	}

	code := ""
	switch {
	case errors.Is(err, theorydbErrors.ErrConditionFailed):
		code = "ErrConditionFailed"
	case errors.Is(err, theorydbErrors.ErrTransactionConflict):
		code = "ErrTransactionConflict"
	case errors.Is(err, theorydbErrors.ErrThrottled):
		code = "ErrThrottled"
	case errors.Is(err, theorydbErrors.ErrTransactionFailed):
		code = "ErrTransactionFailed"
	}
	return code, codes, causeRecoverable
}

func normalizedStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	return values
}
