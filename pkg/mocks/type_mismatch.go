package mocks

import (
	"fmt"
	"strings"

	"github.com/stretchr/testify/mock"
)

const mockReturnTypeMismatchKey = "tabletheory_mock_return_type_mismatches"

func recordReturnTypeMismatch(m *mock.Mock, method string, index int, expected string, got any) {
	if m == nil {
		return
	}

	if method == "" {
		method = "<unknown>"
	}

	msg := fmt.Sprintf(
		"mock return type mismatch for %s return[%d]: expected %s, got %T",
		method,
		index,
		expected,
		got,
	)

	data := m.TestData()
	mismatches, ok := data[mockReturnTypeMismatchKey].([]string)
	if !ok {
		mismatches = nil
	}
	data[mockReturnTypeMismatchKey] = append(mismatches, msg)
}

func typedReturn[T any](m *mock.Mock, method string, index int, expected string, got any) (T, bool) {
	var zero T
	if got == nil {
		return zero, false
	}

	value, ok := got.(T)
	if !ok {
		recordReturnTypeMismatch(m, method, index, expected, got)
		return zero, false
	}
	return value, true
}

func assertNoReturnTypeMismatches(t mock.TestingT, m *mock.Mock) bool {
	if m == nil {
		return true
	}

	mismatches, ok := m.TestData()[mockReturnTypeMismatchKey].([]string)
	if !ok {
		mismatches = nil
	}
	if len(mismatches) == 0 {
		return true
	}

	t.Errorf("TableTheory mock return type mismatches:\n%s", strings.Join(mismatches, "\n"))
	return false
}

func assertMockExpectations(t mock.TestingT, m *mock.Mock) bool {
	expectationsOK := m.AssertExpectations(t)
	mismatchesOK := assertNoReturnTypeMismatches(t, m)
	return expectationsOK && mismatchesOK
}
