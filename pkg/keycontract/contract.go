package keycontract

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// ParseDocument parses YAML or JSON tabletheory_model_contract sidecars.
func ParseDocument(data []byte) (*Contract, error) {
	var contract Contract
	if err := yaml.Unmarshal(data, &contract); err != nil {
		return nil, fmt.Errorf("parse tabletheory model contract: %w", err)
	}
	if err := contract.Validate(); err != nil {
		return nil, err
	}
	return &contract, nil
}

// LoadFile reads and parses a tabletheory_model_contract sidecar.
func LoadFile(path string) (*Contract, error) {
	data, err := readFileScoped(path)
	if err != nil {
		return nil, err
	}
	return ParseDocument(data)
}

func readFileScoped(path string) (data []byte, err error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	dir, name := filepath.Split(abs)
	if name == "" {
		return nil, fmt.Errorf("contract path must name a file: %s", path)
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := root.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()

	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := file.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()

	return io.ReadAll(file)
}

// MarshalJSONStable returns a deterministic JSON rendering suitable for
// generated helper modules and golden comparisons.
func MarshalJSONStable(contract *Contract) ([]byte, error) {
	if contract == nil {
		return nil, fmt.Errorf("tabletheory model contract is nil")
	}
	if err := contract.Validate(); err != nil {
		return nil, err
	}
	return json.MarshalIndent(contract, "", "  ")
}

// Validate checks structural sidecar invariants without interpreting product
// semantics. Product-specific authorization or route-token rules do not belong
// here.
func (c *Contract) Validate() error {
	if c == nil {
		return fmt.Errorf("tabletheory model contract is nil")
	}
	if !isSupportedContractVersion(c.Version) {
		return fmt.Errorf("unsupported tabletheory_model_contract_version: %q", c.Version)
	}
	if len(c.DerivedKeys) == 0 {
		return fmt.Errorf("tabletheory model contract must include derived_keys[]")
	}

	seenKeys := make(map[string]struct{}, len(c.DerivedKeys))
	for i := range c.DerivedKeys {
		if err := validateDerivedKey(c.DerivedKeys[i]); err != nil {
			return err
		}
		if _, ok := seenKeys[c.DerivedKeys[i].Name]; ok {
			return fmt.Errorf("duplicate derived key: %s", c.DerivedKeys[i].Name)
		}
		seenKeys[c.DerivedKeys[i].Name] = struct{}{}
	}

	for _, model := range c.Models {
		if model.Name == "" {
			return fmt.Errorf("model contract missing name")
		}
		for _, name := range model.DerivedKeys {
			if _, ok := seenKeys[name]; !ok {
				return fmt.Errorf("model %s references unknown derived key: %s", model.Name, name)
			}
		}
	}
	return nil
}

// FindDerivedKey returns a derived-key definition by name.
func FindDerivedKey(contract *Contract, name string) (*DerivedKey, bool) {
	if contract == nil {
		return nil, false
	}
	for i := range contract.DerivedKeys {
		if contract.DerivedKeys[i].Name == name {
			return &contract.DerivedKeys[i], true
		}
	}
	return nil, false
}

func validateDerivedKey(key DerivedKey) error {
	if key.Name == "" {
		return fmt.Errorf("derived key missing name")
	}
	if len(key.Segments) == 0 {
		return fmt.Errorf("derived key %s: missing segments[]", key.Name)
	}

	inputNames := make(map[string]Input, len(key.Inputs))
	for _, input := range key.Inputs {
		if input.Name == "" {
			return fmt.Errorf("derived key %s: input missing name", key.Name)
		}
		if _, ok := inputNames[input.Name]; ok {
			return fmt.Errorf("derived key %s: duplicate input %s", key.Name, input.Name)
		}
		inputNames[input.Name] = input
	}

	for i, segment := range key.Segments {
		if err := validateSegment(key.Name, i, segment, inputNames); err != nil {
			return err
		}
	}

	seenFixtures := make(map[string]struct{}, len(key.Fixtures))
	for _, fixture := range key.Fixtures {
		if fixture.Name == "" {
			return fmt.Errorf("derived key %s: fixture missing name", key.Name)
		}
		if _, ok := seenFixtures[fixture.Name]; ok {
			return fmt.Errorf("derived key %s: duplicate fixture %s", key.Name, fixture.Name)
		}
		seenFixtures[fixture.Name] = struct{}{}
		if fixture.Input == nil {
			return fmt.Errorf("derived key %s fixture %s: missing input", key.Name, fixture.Name)
		}
	}
	return nil
}

func validateSegment(keyName string, index int, segment Segment, inputNames map[string]Input) error {
	label := segment.Name
	if label == "" {
		label = fmt.Sprintf("#%d", index+1)
	}

	sources := 0
	if segment.Literal != nil {
		sources++
	}
	if segment.Value.Input != "" {
		sources++
	}
	if segment.Value.Literal != nil {
		sources++
	}
	if sources != 1 {
		return fmt.Errorf("derived key %s segment %s: exactly one value source is required", keyName, label)
	}
	if segment.Value.Input != "" && len(inputNames) > 0 {
		if _, ok := inputNames[segment.Value.Input]; !ok {
			return fmt.Errorf("derived key %s segment %s: unknown input %s", keyName, label, segment.Value.Input)
		}
	}
	for _, transform := range segment.Transforms {
		switch transform {
		case TransformTrim, TransformWildcardEmpty, TransformLowercase, TransformURLEncode:
		default:
			return fmt.Errorf("derived key %s segment %s: unsupported transform %q", keyName, label, transform)
		}
	}
	return nil
}

func isSupportedContractVersion(version string) bool {
	switch version {
	case ContractVersionV01, ContractVersionV02:
		return true
	default:
		return false
	}
}

// DerivedKeyNames returns sorted derived-key names for diagnostics.
func DerivedKeyNames(contract *Contract) []string {
	if contract == nil {
		return nil
	}
	names := make([]string, 0, len(contract.DerivedKeys))
	for _, key := range contract.DerivedKeys {
		names = append(names, key.Name)
	}
	sort.Strings(names)
	return names
}
