package scenario

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Scenario struct {
	Name       string `yaml:"name"`
	DMSVersion string `yaml:"dms_version"`
	Model      string `yaml:"model"`
	Table      Table  `yaml:"table"`
	Steps      []Step `yaml:"steps"`
}

type Table struct {
	Name string `yaml:"name"`
}

type Step struct {
	Op          string            `yaml:"op"`
	IfNotExists bool              `yaml:"if_not_exists"`
	Fields      []string          `yaml:"fields"`
	Item        map[string]any    `yaml:"item"`
	Key         map[string]any    `yaml:"key"`
	Ms          int               `yaml:"ms"`
	Save        map[string]string `yaml:"save"`
	Expect      Expectation       `yaml:"expect"`
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
	return &s, nil
}
