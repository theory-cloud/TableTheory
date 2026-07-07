package runner

import (
	"errors"
	"fmt"
	"testing"

	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/theory-cloud/tabletheory-contract-tests/runners/go/internal/driver"
	"github.com/theory-cloud/tabletheory-contract-tests/runners/go/internal/scenario"
	"github.com/theory-cloud/tabletheory-contract-tests/runners/go/internal/spec"
	theorydbErrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
)

type requireCaptureT struct {
	failed bool
}

func (t *requireCaptureT) Errorf(string, ...any) {
	t.failed = true
}

func (t *requireCaptureT) FailNow() {
	panic("require failed")
}

func (t *requireCaptureT) Helper() {}

func requireFails(t *testing.T, fn func(requireT *requireCaptureT)) {
	t.Helper()

	capture := &requireCaptureT{}
	defer func() {
		if recovered := recover(); recovered != nil && fmt.Sprint(recovered) != "require failed" {
			panic(recovered)
		}
		if !capture.failed {
			t.Fatalf("expected assertion failure")
		}
	}()

	fn(capture)
}

func TestAssertStepResult_FailsClosedWhenItemAssertionHasErrorWithoutOk(t *testing.T) {
	r := &Runner{vars: map[string]any{}}
	model := spec.Model{
		Name: "User",
		Attributes: []spec.Attribute{
			{Attribute: "PK", Type: "S"},
		},
	}

	requireFails(t, func(requireT *requireCaptureT) {
		r.assertStepResult(
			requireT,
			scenario.Expectation{ItemEquals: map[string]any{"PK": "USER#1"}},
			nil,
			errors.New("missing item"),
			nil,
			model,
		)
	})
}

func TestAssertStepResult_FailsClosedWhenItemAssertionHasNoItem(t *testing.T) {
	r := &Runner{vars: map[string]any{}}
	model := spec.Model{
		Name: "User",
		Attributes: []spec.Attribute{
			{Attribute: "PK", Type: "S"},
		},
	}

	requireFails(t, func(requireT *requireCaptureT) {
		r.assertStepResult(
			requireT,
			scenario.Expectation{ItemMissingFields: []string{"nickname"}},
			nil,
			nil,
			nil,
			model,
		)
	})
}

func TestAssertStepResult_RejectsRawAssertionsWithErrorExpectations(t *testing.T) {
	r := &Runner{vars: map[string]any{}}
	model := spec.Model{
		Name: "User",
		Attributes: []spec.Attribute{
			{Attribute: "PK", Type: "S"},
		},
	}

	requireFails(t, func(requireT *requireCaptureT) {
		r.assertStepResult(
			requireT,
			scenario.Expectation{
				Error:           string(driver.ErrItemNotFound),
				RawItemContains: map[string]any{"PK": "USER#1"},
			},
			nil,
			theorydbErrors.ErrItemNotFound,
			nil,
			model,
		)
	})
}

func TestAssertReadResult_FailsClosedWhenReadAssertionHasErrorWithoutOk(t *testing.T) {
	r := &Runner{vars: map[string]any{}}
	model := spec.Model{
		Name: "User",
		Attributes: []spec.Attribute{
			{Attribute: "PK", Type: "S"},
		},
	}

	requireFails(t, func(requireT *requireCaptureT) {
		r.assertReadResult(
			requireT,
			scenario.Expectation{ItemsContains: []map[string]any{{"PK": "USER#1"}}},
			driver.ReadResult{},
			errors.New("query failed"),
			model,
		)
	})
}

func TestAssertStepResult_UsesCanonicalDecimalStringsForRawNumberAssertions(t *testing.T) {
	r := &Runner{vars: map[string]any{}}
	model := numberPrecisionModel()
	expect := scenario.Expectation{ItemContains: map[string]any{
		"largeInteger":   "9007199254740993",
		"preciseDecimal": "0.12345678901234567",
	}}
	item := map[string]any{
		"largeInteger":   "9007199254740993",
		"preciseDecimal": "0.12345678901234567",
	}
	raw := map[string]ddbtypes.AttributeValue{
		"largeInteger":   &ddbtypes.AttributeValueMemberN{Value: "9007199254740993"},
		"preciseDecimal": &ddbtypes.AttributeValueMemberN{Value: "0.12345678901234567"},
	}

	r.assertStepResult(t, expect, item, nil, raw, model)

	lossyRaw := map[string]ddbtypes.AttributeValue{
		"largeInteger":   &ddbtypes.AttributeValueMemberN{Value: "9007199254740992"},
		"preciseDecimal": &ddbtypes.AttributeValueMemberN{Value: "0.12345678901234566"},
	}
	requireFails(t, func(requireT *requireCaptureT) {
		r.assertStepResult(requireT, expect, item, nil, lossyRaw, model)
	})
}

func TestAssertReadResult_UsesCanonicalDecimalStringsForQueryNumberAssertions(t *testing.T) {
	r := &Runner{vars: map[string]any{}}
	model := numberPrecisionModel()
	expect := scenario.Expectation{
		ItemCount: intPtr(1),
		ItemsContains: []map[string]any{{
			"largeInteger":   "9007199254740993",
			"preciseDecimal": "0.12345678901234567",
		}},
	}
	exactResult := driver.ReadResult{Items: []map[string]any{{
		"largeInteger":   "9007199254740993",
		"preciseDecimal": "0.12345678901234567",
	}}}

	r.assertReadResult(t, expect, exactResult, nil, model)

	lossyResult := driver.ReadResult{Items: []map[string]any{{
		"largeInteger":   "9007199254740992",
		"preciseDecimal": "0.12345678901234566",
	}}}
	requireFails(t, func(requireT *requireCaptureT) {
		r.assertReadResult(requireT, expect, lossyResult, nil, model)
	})
}

func numberPrecisionModel() spec.Model {
	return spec.Model{
		Name: "NumberPrecision",
		Attributes: []spec.Attribute{
			{Attribute: "largeInteger", Type: "N"},
			{Attribute: "preciseDecimal", Type: "N"},
		},
	}
}

func intPtr(value int) *int {
	return &value
}
