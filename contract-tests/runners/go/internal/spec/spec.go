package spec

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Document struct {
	DMSVersion string  `yaml:"dms_version"`
	Namespace  string  `yaml:"namespace"`
	Models     []Model `yaml:"models"`
}

type Model struct {
	Name       string      `yaml:"name"`
	Table      Table       `yaml:"table"`
	Naming     Naming      `yaml:"naming"`
	Keys       Keys        `yaml:"keys"`
	Attributes []Attribute `yaml:"attributes"`
	Indexes    []Index     `yaml:"indexes"`
}

type Table struct {
	Name string `yaml:"name"`
}

type Naming struct {
	Convention string `yaml:"convention"`
}

type Keys struct {
	Partition KeyAttribute  `yaml:"partition"`
	Sort      *KeyAttribute `yaml:"sort"`
}

type KeyAttribute struct {
	Attribute string `yaml:"attribute"`
	Type      string `yaml:"type"`
}

type Attribute struct {
	Attribute string   `yaml:"attribute"`
	Type      string   `yaml:"type"`
	Format    string   `yaml:"format"`
	Roles     []string `yaml:"roles"`

	Required  bool `yaml:"required"`
	Optional  bool `yaml:"optional"`
	OmitEmpty bool `yaml:"omit_empty"`
}

type Index struct {
	Name       string        `yaml:"name"`
	Type       string        `yaml:"type"` // GSI | LSI
	Partition  KeyAttribute  `yaml:"partition"`
	Sort       *KeyAttribute `yaml:"sort"`
	Projection Projection    `yaml:"projection"`
}

type Projection struct {
	Type   string   `yaml:"type"` // ALL | KEYS_ONLY | INCLUDE
	Fields []string `yaml:"fields"`
}

func LoadModelsDir(dir string) (map[string]Model, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read models dir: %w", err)
	}

	models := make(map[string]Model)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) != ".yml" && filepath.Ext(name) != ".yaml" {
			continue
		}

		path := filepath.Join(dir, name)
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, fmt.Errorf("read model file %s: %w", path, readErr)
		}

		var doc Document
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("parse model file %s: %w", path, err)
		}

		for _, model := range doc.Models {
			if model.Name == "" {
				continue
			}
			models[model.Name] = model
		}
	}

	if len(models) == 0 {
		return nil, fmt.Errorf("no models found in %s", dir)
	}
	return models, nil
}

func (m Model) AttributeByName(attr string) *Attribute {
	for i := range m.Attributes {
		if m.Attributes[i].Attribute == attr {
			return &m.Attributes[i]
		}
	}
	return nil
}
