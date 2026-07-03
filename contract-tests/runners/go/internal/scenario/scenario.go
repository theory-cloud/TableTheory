package scenario

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Scenario struct {
	Name                 string           `yaml:"name"`
	DMSVersion           string           `yaml:"dms_version"`
	RequiresCapabilities []string         `yaml:"requires_capabilities"`
	Model                string           `yaml:"model"`
	Table                Table            `yaml:"table"`
	Encryption           EncryptionConfig `yaml:"encryption"`
	Steps                []Step           `yaml:"steps"`
	SeedRuntime          string           `yaml:"seed_runtime"`
	SeedSteps            []Step           `yaml:"seed_steps"`
	ReadSteps            []Step           `yaml:"read_steps"`
}

type Table struct {
	Name string `yaml:"name"`
}

type EncryptionConfig struct {
	Provider string `yaml:"provider"`
	Seed     string `yaml:"seed"`
}

type Step struct {
	Op                  string            `yaml:"op"`
	Model               string            `yaml:"model"`
	IfNotExists         bool              `yaml:"if_not_exists"`
	Fields              []string          `yaml:"fields"`
	ProtectedAttributes []string          `yaml:"protected_attributes"`
	Item                map[string]any    `yaml:"item"`
	Key                 map[string]any    `yaml:"key"`
	Query               *ReadRequest      `yaml:"query"`
	Scan                *ReadRequest      `yaml:"scan"`
	Actual              *TransitionActual `yaml:"actual"`
	Event               *TransitionEvent  `yaml:"event"`
	Ms                  int               `yaml:"ms"`
	Save                map[string]string `yaml:"save"`
	Expect              Expectation       `yaml:"expect"`
}

type ReadRequest struct {
	Index          string          `yaml:"index"`
	Partition      *ReadCondition  `yaml:"partition"`
	Sort           *ReadCondition  `yaml:"sort"`
	Filter         []ReadCondition `yaml:"filter"`
	SortDirection  string          `yaml:"sort_direction"`
	Limit          int             `yaml:"limit"`
	Projection     []string        `yaml:"projection"`
	Cursor         string          `yaml:"cursor"`
	ConsistentRead *bool           `yaml:"consistent_read"`
}

type ReadCondition struct {
	Attribute string `yaml:"attribute"`
	Operator  string `yaml:"operator"`
	Value     any    `yaml:"value"`
	Values    []any  `yaml:"values"`
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
	Errors                []string          `yaml:"errors"`
	ItemContains          map[string]any    `yaml:"item_contains"`
	ItemEquals            map[string]any    `yaml:"item_equals"`
	RawItemContains       map[string]any    `yaml:"raw_item_contains"`
	ItemsContains         []map[string]any  `yaml:"items_contains"`
	ItemsMissingFields    []string          `yaml:"items_missing_fields"`
	ItemCount             *int              `yaml:"item_count"`
	CursorEquals          *string           `yaml:"cursor_equals"`
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
	if s.SeedRuntime != "" {
		if len(s.SeedSteps) == 0 {
			return fmt.Errorf("seed_steps are required when seed_runtime is set")
		}
		if len(s.ReadSteps) == 0 {
			return fmt.Errorf("read_steps are required when seed_runtime is set")
		}
	}
	if s.SeedRuntime == "" && len(s.Steps) == 0 {
		return fmt.Errorf("scenario steps are required")
	}

	for label, steps := range map[string][]Step{
		"steps":      s.Steps,
		"seed_steps": s.SeedSteps,
		"read_steps": s.ReadSteps,
	} {
		for i, step := range steps {
			if err := validateStep(label, i, step); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateStep(label string, i int, step Step) error {
	prefix := fmt.Sprintf("%s[%d]", label, i)
	if step.Op == "" {
		return fmt.Errorf("%s: op is required", prefix)
	}
	switch step.Op {
	case "sleep":
		return nil
	case "create", "update", "save":
		if len(step.Item) == 0 {
			return fmt.Errorf("%s %s: item is required", prefix, step.Op)
		}
	case "get", "delete":
		if len(step.Key) == 0 {
			return fmt.Errorf("%s %s: key is required", prefix, step.Op)
		}
	case "query":
		if step.Query == nil {
			return fmt.Errorf("%s query: query is required", prefix)
		}
		if step.Query.Partition == nil {
			return fmt.Errorf("%s query: query.partition is required", prefix)
		}
		if err := validateReadCondition(step.Query.Partition, fmt.Sprintf("%s query.partition", prefix)); err != nil {
			return err
		}
		if step.Query.Sort != nil {
			if err := validateReadCondition(step.Query.Sort, fmt.Sprintf("%s query.sort", prefix)); err != nil {
				return err
			}
		}
		for j := range step.Query.Filter {
			if err := validateReadCondition(&step.Query.Filter[j], fmt.Sprintf("%s query.filter[%d]", prefix, j)); err != nil {
				return err
			}
		}
	case "scan":
		if step.Scan == nil {
			return fmt.Errorf("%s scan: scan is required", prefix)
		}
		for j := range step.Scan.Filter {
			if err := validateReadCondition(&step.Scan.Filter[j], fmt.Sprintf("%s scan.filter[%d]", prefix, j)); err != nil {
				return err
			}
		}
	case "transition_append_event":
		if step.Actual == nil {
			return fmt.Errorf("%s transition_append_event: actual is required", prefix)
		}
		if step.Actual.Model == "" {
			return fmt.Errorf("%s transition_append_event: actual.model is required", prefix)
		}
		if len(step.Actual.Key) == 0 {
			return fmt.Errorf("%s transition_append_event: actual.key is required", prefix)
		}
		if len(step.Actual.Set) == 0 {
			return fmt.Errorf("%s transition_append_event: actual.set is required", prefix)
		}
		if step.Event == nil {
			return fmt.Errorf("%s transition_append_event: event is required", prefix)
		}
		if step.Event.Model == "" {
			return fmt.Errorf("%s transition_append_event: event.model is required", prefix)
		}
		if len(step.Event.Item) == 0 {
			return fmt.Errorf("%s transition_append_event: event.item is required", prefix)
		}
	case "validate_provenance":
		if len(step.Item) == 0 {
			return fmt.Errorf("%s validate_provenance: item is required", prefix)
		}
	default:
		return fmt.Errorf("%s: unsupported op %q", prefix, step.Op)
	}
	return nil
}

func validateReadCondition(cond *ReadCondition, prefix string) error {
	if cond == nil {
		return fmt.Errorf("%s is required", prefix)
	}
	if cond.Attribute == "" {
		return fmt.Errorf("%s.attribute is required", prefix)
	}
	if cond.Operator == "" {
		return fmt.Errorf("%s.operator is required", prefix)
	}
	if cond.Value == nil && len(cond.Values) == 0 && !readOperatorAllowsNoValue(cond.Operator) {
		return fmt.Errorf("%s.value or %s.values is required", prefix, prefix)
	}
	return nil
}

func readOperatorAllowsNoValue(operator string) bool {
	switch strings.ToLower(operator) {
	case "exists", "attribute_exists", "not_exists", "attribute_not_exists":
		return true
	default:
		return false
	}
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
