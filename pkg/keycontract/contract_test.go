package keycontract

import (
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

	assertFixtureCount(t, contract, "WildcardScope", 2)
	assertFixtureCount(t, contract, "CanonicalPolicyKey", 3)
	assertFixtureCount(t, contract, "CanonicalBindingKey", 2)
	assertFixtureCount(t, contract, "InterfaceScopeKey", 1)
	assertFixtureCount(t, contract, "SkillScopeKey", 1)
	assertFixtureCount(t, contract, "AgentScopeKey", 2)
	assertFixtureCount(t, contract, "EmailBindingSortKey", 2)
	assertFixtureCount(t, contract, "GitHubRepositoryLookupKey", 1)
	assertFixtureCount(t, contract, "ImportSessionScopeKey", 1)
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
