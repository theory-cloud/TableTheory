package releasestate

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	theorydbErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
)

func validDeployAuthorityItem() map[string]any {
	return map[string]any{
		"provenance": map[string]any{
			"mode":        "native",
			"system":      "release-control-plane",
			"kind":        "operator_command",
			"ref":         "operator://deploy/service-a/rel_001",
			"observed_at": "2026-04-24T19:00:00Z",
			"recorded_at": "2026-04-24T19:00:01Z",
			"evidence": []any{
				map[string]any{
					"kind":        "operator_command",
					"source":      "release-control-plane",
					"ref":         "operator://deploy/service-a/rel_001",
					"observed_at": "2026-04-24T19:00:00Z",
				},
			},
		},
		"confidence": map[string]any{
			"level":   "high",
			"reasons": []any{"operator_command_authority"},
		},
	}
}

func TestValidateDeployAuthorityMetadataAcceptsDeterministicHighConfidence(t *testing.T) {
	require.NoError(t, ValidateDeployAuthorityMetadata(validDeployAuthorityItem()))
	require.NoError(t, ValidateDeployAuthorityMetadata(map[string]any{"PK": "RELEASE#svc"}))
}

func TestValidateDeployAuthorityMetadataRejectsConflictingEvidence(t *testing.T) {
	item := validDeployAuthorityItem()
	provenance := requireMapValue(t, item, "provenance")
	provenance["mode"] = "imported"
	provenance["system"] = "partner-factory"
	provenance["kind"] = "factory_batch_manifest"
	provenance["ref"] = "s3://factory/manifests/ambiguous.json"
	provenance["evidence"] = []any{
		map[string]any{
			"kind":        "factory_batch_manifest",
			"source":      "partner-factory",
			"ref":         "s3://factory/manifests/a.json",
			"observed_at": "2026-04-24T19:09:59Z",
		},
		map[string]any{
			"kind":        "submodule_pin",
			"source":      "service-ci",
			"ref":         "https://github.com/acme/service-b/tree/conflicting",
			"observed_at": "2026-04-24T19:09:59Z",
		},
	}
	item["confidence"] = map[string]any{
		"level":   "low",
		"reasons": []any{"conflicting_evidence"},
	}

	err := ValidateDeployAuthorityMetadata(item)
	require.Error(t, err)
	require.True(t, errors.Is(err, theorydbErrors.ErrRejectedDeployAuthorityEvidence))
}

func TestValidateDeployAuthorityMetadataRejectsFreeFormNotes(t *testing.T) {
	item := validDeployAuthorityItem()
	requireMapValue(t, item, "provenance")["notes"] = "human note"

	err := ValidateDeployAuthorityMetadata(item)
	require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)
}

func TestValidateDeployAuthorityMetadata_InvalidShapes(t *testing.T) {
	for _, tt := range []struct {
		item map[string]any
		want error
		name string
	}{
		{
			name: "nil item",
			item: nil,
			want: theorydbErrors.ErrInvalidModel,
		},
		{
			name: "missing confidence",
			item: map[string]any{"provenance": map[string]any{}},
			want: theorydbErrors.ErrInvalidModel,
		},
		{
			name: "provenance not object",
			item: map[string]any{"provenance": "note", "confidence": map[string]any{}},
			want: theorydbErrors.ErrInvalidModel,
		},
		{
			name: "confidence not object",
			item: map[string]any{"provenance": map[string]any{}, "confidence": "low"},
			want: theorydbErrors.ErrInvalidModel,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDeployAuthorityMetadata(tt.item)
			require.ErrorIs(t, err, tt.want)
		})
	}
}

func TestValidateDeployAuthorityMetadata_ProvenanceShapeErrors(t *testing.T) {
	for _, tt := range []struct {
		mutate func(map[string]any)
		name   string
	}{
		{
			name: "unsupported mode",
			mutate: func(item map[string]any) {
				requireMapValue(t, item, "provenance")["mode"] = "guessed"
			},
		},
		{
			name: "missing required string",
			mutate: func(item map[string]any) {
				delete(requireMapValue(t, item, "provenance"), "system")
			},
		},
		{
			name: "invalid observed time",
			mutate: func(item map[string]any) {
				requireMapValue(t, item, "provenance")["observed_at"] = "2026/04/24"
			},
		},
		{
			name: "non-string commit",
			mutate: func(item map[string]any) {
				requireMapValue(t, item, "provenance")["commit_sha"] = 123
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			item := validDeployAuthorityItem()
			tt.mutate(item)
			err := ValidateDeployAuthorityMetadata(item)
			require.ErrorIs(t, err, theorydbErrors.ErrInvalidModel)
		})
	}
}

func TestValidateDeployAuthorityMetadata_EvidenceAndConfidenceErrors(t *testing.T) {
	for _, tt := range []struct {
		mutate func(map[string]any)
		want   error
		name   string
	}{
		{
			name: "missing evidence",
			mutate: func(item map[string]any) {
				delete(requireMapValue(t, item, "provenance"), "evidence")
			},
			want: theorydbErrors.ErrInvalidModel,
		},
		{
			name: "empty evidence",
			mutate: func(item map[string]any) {
				requireMapValue(t, item, "provenance")["evidence"] = []any{}
			},
			want: theorydbErrors.ErrRejectedDeployAuthorityEvidence,
		},
		{
			name: "evidence not object",
			mutate: func(item map[string]any) {
				requireMapValue(t, item, "provenance")["evidence"] = []any{"note"}
			},
			want: theorydbErrors.ErrInvalidModel,
		},
		{
			name: "evidence missing source",
			mutate: func(item map[string]any) {
				evidence := firstEvidence(t, item)
				delete(evidence, "source")
			},
			want: theorydbErrors.ErrInvalidModel,
		},
		{
			name: "evidence invalid time",
			mutate: func(item map[string]any) {
				firstEvidence(t, item)["observed_at"] = "Friday"
			},
			want: theorydbErrors.ErrInvalidModel,
		},
		{
			name: "evidence digest not string",
			mutate: func(item map[string]any) {
				firstEvidence(t, item)["digest"] = 7
			},
			want: theorydbErrors.ErrInvalidModel,
		},
		{
			name: "unsupported authority",
			mutate: func(item map[string]any) {
				firstEvidence(t, item)["source"] = "unknown"
			},
			want: theorydbErrors.ErrRejectedDeployAuthorityEvidence,
		},
		{
			name: "low confidence",
			mutate: func(item map[string]any) {
				requireMapValue(t, item, "confidence")["level"] = "low"
			},
			want: theorydbErrors.ErrRejectedDeployAuthorityEvidence,
		},
		{
			name: "reasons not array",
			mutate: func(item map[string]any) {
				requireMapValue(t, item, "confidence")["reasons"] = "operator_command_authority"
			},
			want: theorydbErrors.ErrInvalidModel,
		},
		{
			name: "reason mismatch",
			mutate: func(item map[string]any) {
				requireMapValue(t, item, "confidence")["reasons"] = []any{"manual_note"}
			},
			want: theorydbErrors.ErrRejectedDeployAuthorityEvidence,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			item := validDeployAuthorityItem()
			tt.mutate(item)
			err := ValidateDeployAuthorityMetadata(item)
			require.ErrorIs(t, err, tt.want)
		})
	}
}

func TestValidateDeployAuthorityMetadata_AcceptsOtherDeterministicAuthorities(t *testing.T) {
	for _, tt := range []struct {
		kind   string
		source string
		reason string
	}{
		{kind: "factory_batch_manifest", source: "partner-factory", reason: "unique_factory_manifest_match"},
		{kind: "codepipeline_execution", source: "service-ci", reason: "codepipeline_execution_authority"},
		{kind: "submodule_pin", source: "service-ci", reason: "unique_submodule_pin_match"},
	} {
		t.Run(tt.reason, func(t *testing.T) {
			item := validDeployAuthorityItem()
			evidence := firstEvidence(t, item)
			evidence["kind"] = tt.kind
			evidence["source"] = tt.source
			requireMapValue(t, item, "confidence")["reasons"] = []string{tt.reason}

			require.NoError(t, ValidateDeployAuthorityMetadata(item))
		})
	}
}

func TestInternalValueHelpers(t *testing.T) {
	value, err := mapValue(map[any]any{"ok": "yes"})
	require.NoError(t, err)
	require.Equal(t, "yes", value["ok"])

	_, err = mapValue(map[any]any{1: "bad"})
	require.Error(t, err)

	values, err := stringSliceValue([]string{"a", "b"})
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b"}, values)

	_, err = stringSliceValue([]any{"a", 1})
	require.Error(t, err)
}

func requireMapValue(t *testing.T, item map[string]any, key string) map[string]any {
	t.Helper()
	value, ok := item[key]
	require.True(t, ok)
	typed, ok := value.(map[string]any)
	require.True(t, ok)
	return typed
}

func firstEvidence(t *testing.T, item map[string]any) map[string]any {
	t.Helper()
	provenance := requireMapValue(t, item, "provenance")
	values, ok := provenance["evidence"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, values)
	typed, ok := values[0].(map[string]any)
	require.True(t, ok)
	return typed
}
