package keycontract

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateTypeScriptHelpers(t *testing.T) {
	t.Parallel()

	contract := loadTheoryMCPFixtureContract(t)
	got, err := GenerateTypeScript(contract, GenerateTypeScriptOptions{})
	require.NoError(t, err)

	require.Contains(t, got, `from "@theory-cloud/tabletheory-ts";`)
	require.Contains(t, got, "export const tabletheoryModelContract = {")
	require.Contains(t, got, "export interface CanonicalPolicyKeyInput")
	require.Contains(t, got, "readonly clientNamespace?: string;")
	require.Contains(t, got, "readonly policyVersion: number;")
	require.Contains(t, got, "export function canonicalPolicyKey(input: CanonicalPolicyKeyInput): string")
	require.Contains(t, got, `values["client_namespace"] = input.clientNamespace;`)
	require.Contains(t, got, `return evaluateDerivedKey(tabletheoryModelContract, "CanonicalPolicyKey", values);`)
}

func TestGenerateTypeScriptOptionsAndErrors(t *testing.T) {
	t.Parallel()

	_, err := GenerateTypeScript(nil, GenerateTypeScriptOptions{})
	require.Error(t, err)

	contract := &Contract{Version: ContractVersion, DerivedKeys: []DerivedKey{{
		Name: "no_helper_name",
		Join: "",
		Inputs: []Input{
			{Name: "some_value", Type: "scalar"},
			{Name: "flag", Type: "boolean"},
		},
		Segments: []Segment{{Value: ValueSource{Input: "some_value"}}},
	}}}
	got, err := GenerateTypeScript(contract, GenerateTypeScriptOptions{RuntimeImport: "./runtime.js"})
	require.NoError(t, err)
	require.Contains(t, got, `from "./runtime.js";`)
	require.Contains(t, got, "export interface NoHelperNameInput")
	require.Contains(t, got, "readonly someValue: KeyContractInputValue;")
	require.Contains(t, got, "readonly flag: boolean;")

	invalidHelper := &Contract{Version: ContractVersion, DerivedKeys: []DerivedKey{{
		Name:     "Bad",
		Helper:   "not-valid",
		Join:     "",
		Segments: []Segment{{Value: ValueSource{Input: "id"}}},
	}}}
	_, err = GenerateTypeScript(invalidHelper, GenerateTypeScriptOptions{})
	require.Error(t, err)
}
