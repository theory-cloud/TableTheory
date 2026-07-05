package keycontract

import (
	"math"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEvaluateDerivedKeyTransformsDefaultsAndOmission(t *testing.T) {
	t.Parallel()

	manual := "manual"
	key := DerivedKey{
		Name: "ExampleKey",
		Join: "|",
		Inputs: []Input{
			{Name: "scope", Optional: true},
			{Name: "mode", Optional: true},
		},
		Segments: []Segment{
			{
				Name:       "scope",
				Prefix:     "scope=",
				Value:      ValueSource{Input: "scope"},
				Transforms: []string{TransformTrim, TransformWildcardEmpty},
			},
			{
				Name:       "mode",
				Prefix:     "mode=",
				Value:      ValueSource{Input: "mode"},
				Transforms: []string{TransformTrim},
				Default:    &manual,
				Optional:   true,
				OmitWhen:   OmitWhen{Default: true},
			},
		},
	}

	got, err := EvaluateDerivedKey(key, map[string]any{"scope": " keybank "})
	require.NoError(t, err)
	require.Equal(t, "scope=keybank", got)

	got, err = EvaluateDerivedKey(key, map[string]any{"scope": "", "mode": "auto"})
	require.NoError(t, err)
	require.Equal(t, "scope=*|mode=auto", got)

	got, err = EvaluateDerivedKey(key, map[string]any{"scope": "\u0085keybank\ufeff"})
	require.NoError(t, err)
	require.Equal(t, "scope=keybank", got)
}

func TestEvaluateDerivedKeyEscapesUserSuppliedReservedBytes(t *testing.T) {
	t.Parallel()

	key := DerivedKey{
		Name: "Composite",
		Join: "|",
		Inputs: []Input{
			{Name: "tenant"},
			{Name: "resource"},
		},
		Segments: []Segment{
			{Name: "tenant", Prefix: "tenant=", Value: ValueSource{Input: "tenant"}, Transforms: []string{TransformTrim}},
			{Name: "resource", Prefix: "resource=", Value: ValueSource{Input: "resource"}, Transforms: []string{TransformTrim}},
		},
	}

	left, err := EvaluateDerivedKey(key, map[string]any{"tenant": "a|resource=b", "resource": "c"})
	require.NoError(t, err)
	require.Equal(t, "tenant=a%7Cresource%3Db|resource=c", left)

	right, err := EvaluateDerivedKey(key, map[string]any{"tenant": "a", "resource": "b|resource=c"})
	require.NoError(t, err)
	require.Equal(t, "tenant=a|resource=b%7Cresource%3Dc", right)
	require.NotEqual(t, left, right)

	got, err := EvaluateDerivedKey(key, map[string]any{"tenant": "user/*", "resource": "café"})
	require.NoError(t, err)
	require.Equal(t, "tenant=user%2F%2A|resource=caf%C3%A9", got)
}

func TestEvaluateDerivedKeyV02Transforms(t *testing.T) {
	t.Parallel()

	key := DerivedKey{
		Name: "V02",
		Join: "|",
		Inputs: []Input{
			{Name: "namespace"},
			{Name: "repository"},
		},
		Segments: []Segment{
			{
				Name:       "namespace",
				Prefix:     "ns=",
				Value:      ValueSource{Input: "namespace"},
				Transforms: []string{TransformTrim, TransformLowercase, TransformURLEncode},
			},
			{
				Name:       "repository",
				Prefix:     "repo=",
				Value:      ValueSource{Input: "repository"},
				Transforms: []string{TransformTrim, TransformLowercase, TransformURLEncode},
			},
		},
	}

	got, err := EvaluateDerivedKey(key, map[string]any{
		"namespace":  " İSTANBUL ",
		"repository": "CAFÉ/Docs",
	})
	require.NoError(t, err)
	require.Equal(t, "ns=%C4%B0stanbul|repo=caf%C3%89%2Fdocs", got)
}

func TestEvaluateDerivedKeyRejectsExplicitNullInput(t *testing.T) {
	t.Parallel()

	key := DerivedKey{
		Name:     "Required",
		Segments: []Segment{{Value: ValueSource{Input: "id"}}},
	}

	_, err := EvaluateDerivedKey(key, map[string]any{"id": nil})
	require.Error(t, err)
	require.Contains(t, err.Error(), `input "id"`)
	require.Contains(t, err.Error(), "must not be null")
}

func TestScalarToStringCanonicalNumberFormat(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		value any
		want  string
	}{
		{name: "large_without_exponent", value: float64(1e21), want: "1000000000000000000000"},
		{name: "small_without_exponent", value: float64(1e-6), want: "0.000001"},
		{name: "smaller_without_exponent", value: float64(1e-7), want: "0.0000001"},
		{name: "negative_zero_float64", value: math.Copysign(0, -1), want: "0"},
		{name: "negative_zero_float32", value: float32(math.Copysign(0, -1)), want: "0"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := scalarToString(tc.value)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestScalarToStringRejectsNonFiniteNumbers(t *testing.T) {
	t.Parallel()

	for _, value := range []any{math.NaN(), math.Inf(1), math.Inf(-1), float32(math.NaN()), float32(math.Inf(1)), float32(math.Inf(-1))} {
		_, err := scalarToString(value)
		require.Error(t, err)
		require.Contains(t, err.Error(), "finite number")
	}
}

func TestEvaluateDerivedKeyMissingRequiredInput(t *testing.T) {
	t.Parallel()

	key := DerivedKey{
		Name:     "Required",
		Segments: []Segment{{Value: ValueSource{Input: "id"}}},
	}

	_, err := EvaluateDerivedKey(key, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), `missing required input "id"`)
}

func TestTheoryMCPDerivedKeyFixtures(t *testing.T) {
	t.Parallel()

	contract := loadTheoryMCPFixtureContract(t)
	require.NoError(t, VerifyFixtures(contract))

	assertFixtureCount(t, contract, "WildcardScope", 4)
	assertFixtureCount(t, contract, "CanonicalPolicyKey", 3)
	assertFixtureCount(t, contract, "ScalarNumberKey", 3)
	assertFixtureCount(t, contract, "CanonicalBindingKey", 2)
	assertFixtureCount(t, contract, "InterfaceScopeKey", 1)
	assertFixtureCount(t, contract, "SkillScopeKey", 1)
	assertFixtureCount(t, contract, "AgentScopeKey", 2)
	assertFixtureCount(t, contract, "EmailBindingSortKey", 2)
	assertFixtureCount(t, contract, "GitHubRepositoryLookupKey", 1)
	assertFixtureCount(t, contract, "ImportSessionScopeKey", 1)
}

func TestV02DerivedKeyTransformFixtures(t *testing.T) {
	t.Parallel()

	contract, err := LoadFile(filepath.Join("..", "..", "contract-tests", "key-contracts", "v0.2", "derived-key-transforms.yml"))
	require.NoError(t, err)
	require.NoError(t, VerifyFixtures(contract))

	assertFixtureCount(t, contract, "LowercaseLookupKey", 2)
	assertFixtureCount(t, contract, "LiteralUrlEncodedKey", 1)
}

func TestParseDocumentRejectsInvalidContracts(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "version",
			raw:  "tabletheory_model_contract_version: \"9.9\"\nderived_keys: []",
			want: "unsupported tabletheory_model_contract_version",
		},
		{
			name: "unknown_transform",
			raw: `tabletheory_model_contract_version: "0.1"
derived_keys:
  - name: Bad
    join: ""
    segments:
      - value: { input: id }
        transforms: [lower]
`,
			want: "unsupported transform",
		},
		{
			name: "duplicate_key",
			raw: `tabletheory_model_contract_version: "0.1"
derived_keys:
  - name: Dup
    join: ""
    segments: [{ literal: "a" }]
  - name: Dup
    join: ""
    segments: [{ literal: "b" }]
`,
			want: "duplicate derived key",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseDocument([]byte(tc.raw))
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.want)
		})
	}
}

func loadTheoryMCPFixtureContract(t *testing.T) *Contract {
	t.Helper()

	contract, err := LoadFile(filepath.Join("..", "..", "contract-tests", "key-contracts", "v0.1", "theorymcp-derived-keys.yml"))
	require.NoError(t, err)
	return contract
}

func assertFixtureCount(t *testing.T, contract *Contract, keyName string, want int) {
	t.Helper()

	key, ok := FindDerivedKey(contract, keyName)
	require.True(t, ok, "missing derived key %s among %s", keyName, strings.Join(DerivedKeyNames(contract), ", "))
	require.Len(t, key.Fixtures, want)
}
