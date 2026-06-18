package keycontract

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEvaluateByNameAndMissingKey(t *testing.T) {
	t.Parallel()

	contract := &Contract{
		Version: ContractVersion,
		DerivedKeys: []DerivedKey{{
			Name: "Named",
			Join: "",
			Segments: []Segment{{
				Prefix: "id=",
				Value:  ValueSource{Input: "id"},
			}},
		}},
	}

	got, err := Evaluate(contract, "Named", map[string]any{"id": 42})
	require.NoError(t, err)
	require.Equal(t, "id=42", got)

	_, err = Evaluate(contract, "Missing", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "derived key not found")
}

func TestLiteralOptionalAndOmitRules(t *testing.T) {
	t.Parallel()

	literal := "literal"
	valueLiteral := "value-literal"
	key := DerivedKey{
		Name: "Literals",
		Join: ":",
		Segments: []Segment{
			{Literal: &literal},
			{Value: ValueSource{Literal: &valueLiteral}},
			{Value: ValueSource{Input: "missing"}, Optional: true},
			{Value: ValueSource{Input: "empty"}, OmitWhen: OmitWhen{Empty: true}},
			{Value: ValueSource{Input: "state"}, OmitWhen: OmitWhen{Values: []string{"skip"}}},
			{Value: ValueSource{Input: "kept"}},
		},
	}

	got, err := EvaluateDerivedKey(key, map[string]any{
		"empty": "",
		"state": "skip",
		"kept":  true,
	})
	require.NoError(t, err)
	require.Equal(t, "literal:value-literal:true", got)
}

func TestScalarToStringCoversSupportedScalars(t *testing.T) {
	t.Parallel()

	cases := []struct {
		value any
		want  string
	}{
		{"x", "x"},
		{false, "false"},
		{float32(1.5), "1.5"},
		{float64(2.25), "2.25"},
		{int(-1), "-1"},
		{int8(-2), "-2"},
		{int16(-3), "-3"},
		{int32(-4), "-4"},
		{int64(-5), "-5"},
		{uint(1), "1"},
		{uint8(2), "2"},
		{uint16(3), "3"},
		{uint32(4), "4"},
		{uint64(5), "5"},
	}

	for _, tc := range cases {
		got, err := scalarToString(tc.value)
		require.NoError(t, err)
		require.Equal(t, tc.want, got)
	}

	_, err := scalarToString(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "must not be null")

	_, err = scalarToString([]string{"not", "scalar"})
	require.Error(t, err)
}

func TestContractValidationErrors(t *testing.T) {
	t.Parallel()

	_, err := MarshalJSONStable(nil)
	require.Error(t, err)

	cases := []struct {
		name     string
		contract *Contract
		want     string
	}{
		{name: "nil", contract: nil, want: "nil"},
		{name: "empty_keys", contract: &Contract{Version: ContractVersion}, want: "derived_keys"},
		{
			name: "model_missing_name",
			contract: &Contract{
				Version:     ContractVersion,
				DerivedKeys: []DerivedKey{validKey("Known")},
				Models:      []ModelContract{{DerivedKeys: []string{"Known"}}},
			},
			want: "model contract missing name",
		},
		{
			name: "model_unknown_key",
			contract: &Contract{
				Version:     ContractVersion,
				DerivedKeys: []DerivedKey{validKey("Known")},
				Models:      []ModelContract{{Name: "Model", DerivedKeys: []string{"Missing"}}},
			},
			want: "unknown derived key",
		},
		{
			name: "input_missing_name",
			contract: &Contract{
				Version: ContractVersion,
				DerivedKeys: []DerivedKey{{
					Name:     "BadInput",
					Inputs:   []Input{{}},
					Segments: []Segment{{Value: ValueSource{Input: "id"}}},
				}},
			},
			want: "input missing name",
		},
		{
			name: "duplicate_input",
			contract: &Contract{
				Version: ContractVersion,
				DerivedKeys: []DerivedKey{{
					Name:     "DupInput",
					Inputs:   []Input{{Name: "id"}, {Name: "id"}},
					Segments: []Segment{{Value: ValueSource{Input: "id"}}},
				}},
			},
			want: "duplicate input",
		},
		{
			name: "unknown_input",
			contract: &Contract{
				Version: ContractVersion,
				DerivedKeys: []DerivedKey{{
					Name:     "UnknownInput",
					Inputs:   []Input{{Name: "id"}},
					Segments: []Segment{{Value: ValueSource{Input: "missing"}}},
				}},
			},
			want: "unknown input",
		},
		{
			name: "source_count",
			contract: &Contract{
				Version: ContractVersion,
				DerivedKeys: []DerivedKey{{
					Name:     "NoSource",
					Segments: []Segment{{}},
				}},
			},
			want: "exactly one value source",
		},
		{
			name: "duplicate_fixture",
			contract: &Contract{
				Version: ContractVersion,
				DerivedKeys: []DerivedKey{{
					Name: "DupFixture",
					Segments: []Segment{{
						Value: ValueSource{Input: "id"},
					}},
					Fixtures: []Fixture{
						{Name: "case", Input: map[string]any{"id": "a"}, Expect: "a"},
						{Name: "case", Input: map[string]any{"id": "b"}, Expect: "b"},
					},
				}},
			},
			want: "duplicate fixture",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.contract.Validate()
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestVerifyFixturesReportsMismatchAndEvaluationErrors(t *testing.T) {
	t.Parallel()

	mismatch := &Contract{
		Version: ContractVersion,
		DerivedKeys: []DerivedKey{{
			Name:     "Mismatch",
			Join:     "",
			Segments: []Segment{{Value: ValueSource{Input: "id"}}},
			Fixtures: []Fixture{{Name: "case", Input: map[string]any{"id": "got"}, Expect: "want"}},
		}},
	}
	require.Error(t, VerifyFixtures(mismatch))

	errorCase := &Contract{
		Version: ContractVersion,
		DerivedKeys: []DerivedKey{{
			Name:     "UnsupportedValue",
			Join:     "",
			Segments: []Segment{{Value: ValueSource{Input: "id"}}},
			Fixtures: []Fixture{{Name: "case", Input: map[string]any{"id": []string{"bad"}}, Expect: ""}},
		}},
	}
	require.Error(t, VerifyFixtures(errorCase))
}

func TestLoadFileScopedErrors(t *testing.T) {
	t.Parallel()

	_, err := LoadFile(filepath.Join(t.TempDir(), "missing.yml"))
	require.Error(t, err)

	_, err = readFileScoped(t.TempDir())
	require.Error(t, err)
}

func validKey(name string) DerivedKey {
	return DerivedKey{
		Name:     name,
		Join:     "",
		Segments: []Segment{{Value: ValueSource{Input: "id"}}},
	}
}
