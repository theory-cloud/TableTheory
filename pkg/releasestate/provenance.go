package releasestate

import (
	"fmt"
	"reflect"
	"time"

	theorydbErrors "github.com/theory-cloud/tabletheory/pkg/errors"
)

var (
	allowedProvenanceKeys = map[string]struct{}{
		"mode":          {},
		"system":        {},
		"kind":          {},
		"ref":           {},
		"commit_sha":    {},
		"observed_at":   {},
		"recorded_at":   {},
		"import_run_id": {},
		"evidence":      {},
	}
	allowedEvidenceKeys = map[string]struct{}{
		"kind":        {},
		"source":      {},
		"ref":         {},
		"observed_at": {},
		"digest":      {},
	}
	allowedConfidenceKeys = map[string]struct{}{
		"level":   {},
		"reasons": {},
	}
	allowedProvenanceModes = map[string]struct{}{
		"native":   {},
		"imported": {},
		"inferred": {},
	}
)

// ValidateDeployAuthorityMetadata validates deterministic provenance and
// confidence metadata for deploy-authoritative release-state actual rows.
//
// Ambiguous/conflicting or low-confidence evidence is rejected with
// ErrRejectedDeployAuthorityEvidence so callers do not accidentally persist it
// as deploy authority. Such evidence should be stored as separate immutable
// visibility/event records instead.
func ValidateDeployAuthorityMetadata(item map[string]any) error {
	if item == nil {
		return fmt.Errorf("%w: release-state item is required", theorydbErrors.ErrInvalidModel)
	}
	provenanceRaw, hasProvenance := item["provenance"]
	confidenceRaw, hasConfidence := item["confidence"]
	if !hasProvenance && !hasConfidence {
		return nil
	}
	if !hasProvenance || !hasConfidence {
		return fmt.Errorf("%w: provenance and confidence must be provided together", theorydbErrors.ErrInvalidModel)
	}

	provenance, err := mapValue(provenanceRaw)
	if err != nil {
		return fmt.Errorf("%w: provenance must be an object", theorydbErrors.ErrInvalidModel)
	}
	confidence, err := mapValue(confidenceRaw)
	if err != nil {
		return fmt.Errorf("%w: confidence must be an object", theorydbErrors.ErrInvalidModel)
	}
	err = validateAllowedKeys("provenance", provenance, allowedProvenanceKeys)
	if err != nil {
		return err
	}
	err = validateAllowedKeys("confidence", confidence, allowedConfidenceKeys)
	if err != nil {
		return err
	}
	err = validateProvenanceShape(provenance)
	if err != nil {
		return err
	}

	reason, err := deriveDeployAuthorityReason(provenance)
	if err != nil {
		return err
	}
	return validateConfidence(confidence, reason)
}

func validateProvenanceShape(provenance map[string]any) error {
	mode, err := requiredString(provenance, "mode")
	if err != nil {
		return err
	}
	if _, ok := allowedProvenanceModes[mode]; !ok {
		return fmt.Errorf("%w: unsupported provenance.mode %q", theorydbErrors.ErrInvalidModel, mode)
	}

	for _, key := range []string{"system", "kind", "ref"} {
		if _, err := requiredString(provenance, key); err != nil {
			return err
		}
	}
	for _, key := range []string{"observed_at", "recorded_at"} {
		value, err := requiredString(provenance, key)
		if err != nil {
			return err
		}
		if err := validateRFC3339(key, value); err != nil {
			return err
		}
	}
	for _, key := range []string{"commit_sha", "import_run_id"} {
		if value, ok := provenance[key]; ok {
			if _, ok := value.(string); !ok {
				return fmt.Errorf("%w: provenance.%s must be a string", theorydbErrors.ErrInvalidModel, key)
			}
		}
	}
	return nil
}

func deriveDeployAuthorityReason(provenance map[string]any) (string, error) {
	evidenceList, err := evidenceValues(provenance["evidence"])
	if err != nil {
		return "", err
	}
	if len(evidenceList) == 0 {
		return "", fmt.Errorf("%w: deploy authority requires evidence", theorydbErrors.ErrRejectedDeployAuthorityEvidence)
	}

	var signature string
	var first deployEvidence
	for i, evidence := range evidenceList {
		parsed, err := parseDeployEvidence(evidence)
		if err != nil {
			return "", err
		}

		currentSignature := parsed.signature()
		if i == 0 {
			signature = currentSignature
			first = parsed
			continue
		}
		if currentSignature != signature {
			return "", fmt.Errorf("%w: conflicting deploy authority evidence", theorydbErrors.ErrRejectedDeployAuthorityEvidence)
		}
	}

	return authorityReasonForEvidence(first)
}

type deployEvidence struct {
	kind   string
	source string
	ref    string
}

func (e deployEvidence) signature() string {
	return e.kind + "|" + e.source + "|" + e.ref
}

func parseDeployEvidence(evidence map[string]any) (deployEvidence, error) {
	if err := validateAllowedKeys("provenance.evidence", evidence, allowedEvidenceKeys); err != nil {
		return deployEvidence{}, err
	}

	kind, err := requiredString(evidence, "kind")
	if err != nil {
		return deployEvidence{}, err
	}
	source, err := requiredString(evidence, "source")
	if err != nil {
		return deployEvidence{}, err
	}
	ref, err := requiredString(evidence, "ref")
	if err != nil {
		return deployEvidence{}, err
	}
	observedAt, err := requiredString(evidence, "observed_at")
	if err != nil {
		return deployEvidence{}, err
	}
	if err := validateRFC3339("evidence.observed_at", observedAt); err != nil {
		return deployEvidence{}, err
	}
	if digest, ok := evidence["digest"]; ok {
		if _, ok := digest.(string); !ok {
			return deployEvidence{}, fmt.Errorf("%w: evidence.digest must be a string", theorydbErrors.ErrInvalidModel)
		}
	}

	return deployEvidence{kind: kind, source: source, ref: ref}, nil
}

func authorityReasonForEvidence(evidence deployEvidence) (string, error) {
	reasons := map[deployEvidence]string{
		{kind: "operator_command", source: "release-control-plane"}: "operator_command_authority",
		{kind: "factory_batch_manifest", source: "partner-factory"}: "unique_factory_manifest_match",
		{kind: "codepipeline_execution", source: "service-ci"}:      "codepipeline_execution_authority",
		{kind: "submodule_pin", source: "service-ci"}:               "unique_submodule_pin_match",
	}
	key := deployEvidence{kind: evidence.kind, source: evidence.source}
	if reason, ok := reasons[key]; ok {
		return reason, nil
	}
	return "", fmt.Errorf("%w: unsupported deploy authority evidence %s/%s", theorydbErrors.ErrRejectedDeployAuthorityEvidence, evidence.source, evidence.kind)
}

func validateConfidence(confidence map[string]any, expectedReason string) error {
	level, err := requiredString(confidence, "level")
	if err != nil {
		return err
	}
	if level != "high" {
		return fmt.Errorf("%w: %s confidence cannot authorize deploy state", theorydbErrors.ErrRejectedDeployAuthorityEvidence, level)
	}
	reasons, err := stringSliceValue(confidence["reasons"])
	if err != nil {
		return fmt.Errorf("%w: confidence.reasons must be a string array", theorydbErrors.ErrInvalidModel)
	}
	if len(reasons) != 1 || reasons[0] != expectedReason {
		return fmt.Errorf("%w: confidence reasons do not match deterministic authority", theorydbErrors.ErrRejectedDeployAuthorityEvidence)
	}
	return nil
}

func validateAllowedKeys(label string, values map[string]any, allowed map[string]struct{}) error {
	for key := range values {
		if _, ok := allowed[key]; !ok {
			return fmt.Errorf("%w: unsupported %s key %q", theorydbErrors.ErrInvalidModel, label, key)
		}
	}
	return nil
}

func requiredString(values map[string]any, key string) (string, error) {
	value, ok := values[key]
	if !ok {
		return "", fmt.Errorf("%w: %s is required", theorydbErrors.ErrInvalidModel, key)
	}
	text, ok := value.(string)
	if !ok || text == "" {
		return "", fmt.Errorf("%w: %s must be a non-empty string", theorydbErrors.ErrInvalidModel, key)
	}
	return text, nil
}

func validateRFC3339(label string, value string) error {
	if _, err := time.Parse(time.RFC3339Nano, value); err != nil {
		return fmt.Errorf("%w: %s must be RFC3339", theorydbErrors.ErrInvalidModel, label)
	}
	return nil
}

func mapValue(value any) (map[string]any, error) {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for k, v := range typed {
			out[k] = v
		}
		return out, nil
	case map[any]any:
		out := make(map[string]any, len(typed))
		for k, v := range typed {
			key, ok := k.(string)
			if !ok {
				return nil, fmt.Errorf("map key must be string")
			}
			out[key] = v
		}
		return out, nil
	default:
		return nil, fmt.Errorf("expected object, got %T", value)
	}
}

func evidenceValues(value any) ([]map[string]any, error) {
	if value == nil {
		return nil, fmt.Errorf("%w: provenance.evidence is required", theorydbErrors.ErrInvalidModel)
	}

	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return nil, fmt.Errorf("%w: provenance.evidence must be an array", theorydbErrors.ErrInvalidModel)
	}

	out := make([]map[string]any, 0, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		entry, err := mapValue(rv.Index(i).Interface())
		if err != nil {
			return nil, fmt.Errorf("%w: provenance.evidence entries must be objects", theorydbErrors.ErrInvalidModel)
		}
		out = append(out, entry)
	}
	return out, nil
}

func stringSliceValue(value any) ([]string, error) {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...), nil
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok || text == "" {
				return nil, fmt.Errorf("expected string")
			}
			out = append(out, text)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("expected string array")
	}
}
