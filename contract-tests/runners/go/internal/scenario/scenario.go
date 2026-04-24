package scenario

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Scenario struct {
	Name                 string   `yaml:"name"`
	DMSVersion           string   `yaml:"dms_version"`
	RequiresCapabilities []string `yaml:"requires_capabilities"`
	Model                string   `yaml:"model"`
	Table                Table    `yaml:"table"`
	Steps                []Step   `yaml:"steps"`
}

type Table struct {
	Name string `yaml:"name"`
}

type Step struct {
	Op                  string            `yaml:"op"`
	Model               string            `yaml:"model"`
	IfNotExists         bool              `yaml:"if_not_exists"`
	Fields              []string          `yaml:"fields"`
	ProtectedAttributes []string          `yaml:"protected_attributes"`
	Item                map[string]any    `yaml:"item"`
	Key                 map[string]any    `yaml:"key"`
	Actual              *TransitionActual `yaml:"actual"`
	Event               *TransitionEvent  `yaml:"event"`
	Ms                  int               `yaml:"ms"`
	Save                map[string]string `yaml:"save"`
	Expect              Expectation       `yaml:"expect"`
}

type TransitionActual struct {
	Model           string         `yaml:"model"`
	Key             map[string]any `yaml:"key"`
	Set             map[string]any `yaml:"set"`
	ExpectedVersion *int64         `yaml:"expected_version"`
}

type TransitionEvent struct {
	Model string         `yaml:"model"`
	Item  map[string]any `yaml:"item"`
}

type Expectation struct {
	Ok                    *bool             `yaml:"ok"`
	Error                 string            `yaml:"error"`
	ItemContains          map[string]any    `yaml:"item_contains"`
	ItemHasFields         []string          `yaml:"item_has_fields"`
	ItemMissingFields     []string          `yaml:"item_missing_fields"`
	RawAttributeTypes     map[string]string `yaml:"raw_attribute_types"`
	ItemFieldEqualsVar    map[string]string `yaml:"item_field_equals_var"`
	ItemFieldNotEqualsVar map[string]string `yaml:"item_field_not_equals_var"`
}

func LoadFile(path string) (*Scenario, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read scenario: %w", err)
	}

	var s Scenario
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse scenario: %w", err)
	}

	if s.Name == "" {
		return nil, fmt.Errorf("scenario name is required")
	}
	if s.Model == "" {
		return nil, fmt.Errorf("scenario model is required")
	}
	if err := validateSteps(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

func validateSteps(s *Scenario) error {
	for i, step := range s.Steps {
		if step.Op == "" {
			return fmt.Errorf("step %d: op is required", i)
		}
		switch step.Op {
		case "sleep":
			continue
		case "create", "update", "save":
			if len(step.Item) == 0 {
				return fmt.Errorf("step %d %s: item is required", i, step.Op)
			}
		case "get", "delete":
			if len(step.Key) == 0 {
				return fmt.Errorf("step %d %s: key is required", i, step.Op)
			}
		case "transition_append_event":
			if step.Actual == nil {
				return fmt.Errorf("step %d transition_append_event: actual is required", i)
			}
			if step.Actual.Model == "" {
				return fmt.Errorf("step %d transition_append_event: actual.model is required", i)
			}
			if len(step.Actual.Key) == 0 {
				return fmt.Errorf("step %d transition_append_event: actual.key is required", i)
			}
			if len(step.Actual.Set) == 0 {
				return fmt.Errorf("step %d transition_append_event: actual.set is required", i)
			}
			if step.Event == nil {
				return fmt.Errorf("step %d transition_append_event: event is required", i)
			}
			if step.Event.Model == "" {
				return fmt.Errorf("step %d transition_append_event: event.model is required", i)
			}
			if len(step.Event.Item) == 0 {
				return fmt.Errorf("step %d transition_append_event: event.item is required", i)
			}
		case "validate_provenance":
			if len(step.Item) == 0 {
				return fmt.Errorf("step %d validate_provenance: item is required", i)
			}
		default:
			return fmt.Errorf("step %d: unsupported op %q", i, step.Op)
		}
	}
	return nil
}

func MissingCapabilities(required []string, supported []string) []string {
	if len(required) == 0 {
		return nil
	}
	supportedSet := make(map[string]struct{}, len(supported))
	for _, capability := range supported {
		supportedSet[capability] = struct{}{}
	}
	var missing []string
	for _, capability := range required {
		if _, ok := supportedSet[capability]; !ok {
			missing = append(missing, capability)
		}
	}
	return missing
}
