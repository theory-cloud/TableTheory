package runner

import (
	"errors"
	"fmt"
	"testing"

	"github.com/theory-cloud/tabletheory-contract-tests/runners/go/internal/driver"
	"github.com/theory-cloud/tabletheory-contract-tests/runners/go/internal/scenario"
	"github.com/theory-cloud/tabletheory-contract-tests/runners/go/internal/spec"
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
