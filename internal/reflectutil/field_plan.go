package reflectutil

import (
	"fmt"
	"reflect"
)

// LegacyAliasResolver resolves the legacy container aliases that older helper
// surfaces used when anonymous embedded structs were encoded as nested maps.
//
// When skip is true, no legacy container aliases are recorded for the field.
// This is useful for callers that treat tag-based skips (for example "-") as
// ineligible for compatibility lookup while still wanting the promoted leaf
// fields themselves to remain visible.
type LegacyAliasResolver func(field reflect.StructField) (aliases []string, skip bool, err error)

// VisibleFieldPlan describes one exported visible field on a struct,
// including promoted fields that arrived through exported anonymous embedded
// structs. Anonymous struct containers themselves are intentionally omitted.
type VisibleFieldPlan struct {
	Field            reflect.StructField
	IndexPath        []int
	LegacyContainers []LegacyContainerPlan
}

// LegacyContainerPlan records one anonymous embedded struct container on the
// path to a promoted visible field, along with the aliases legacy helper
// surfaces used for nested compatibility lookups.
type LegacyContainerPlan struct {
	Field     reflect.StructField
	IndexPath []int
	Aliases   []string
}

// BuildVisibleFieldPlan enumerates exported visible fields for a struct using
// Go promotion rules. Promoted fields are included only when they arrive
// through exported anonymous embedded structs, matching TableTheory's current
// metadata recursion scope. Anonymous struct containers themselves are omitted.
func BuildVisibleFieldPlan(modelType reflect.Type, aliasResolver LegacyAliasResolver) ([]VisibleFieldPlan, error) {
	structType, err := resolveStructType(modelType)
	if err != nil {
		return nil, err
	}

	visibleFields := reflect.VisibleFields(structType)
	plans := make([]VisibleFieldPlan, 0, len(visibleFields))
	for _, field := range visibleFields {
		if !field.IsExported() || isAnonymousEmbeddedStruct(field) {
			continue
		}

		legacyContainers, include, err := buildLegacyContainerPlan(structType, field.Index, aliasResolver)
		if err != nil {
			return nil, err
		}
		if !include {
			continue
		}

		plans = append(plans, VisibleFieldPlan{
			Field:            field,
			IndexPath:        cloneIndexPath(field.Index),
			LegacyContainers: legacyContainers,
		})
	}

	return plans, nil
}

func resolveStructType(modelType reflect.Type) (reflect.Type, error) {
	if modelType == nil {
		return nil, fmt.Errorf("model type cannot be nil")
	}

	for modelType.Kind() == reflect.Ptr {
		modelType = modelType.Elem()
	}

	if modelType.Kind() != reflect.Struct {
		return nil, fmt.Errorf("model type must be a struct or pointer to struct, got %s", modelType)
	}

	return modelType, nil
}

func buildLegacyContainerPlan(modelType reflect.Type, indexPath []int, aliasResolver LegacyAliasResolver) ([]LegacyContainerPlan, bool, error) {
	if len(indexPath) <= 1 {
		return nil, true, nil
	}

	containers := make([]LegacyContainerPlan, 0, len(indexPath)-1)
	for depth := 1; depth < len(indexPath); depth++ {
		containerIndexPath := cloneIndexPath(indexPath[:depth])
		containerField := modelType.FieldByIndex(containerIndexPath)
		if !containerField.Anonymous || containerField.Type.Kind() != reflect.Struct {
			return nil, false, nil
		}
		if !containerField.IsExported() {
			return nil, false, nil
		}

		aliases, skip, err := resolveLegacyAliases(containerField, aliasResolver)
		if err != nil {
			return nil, false, err
		}
		if skip {
			continue
		}

		containers = append(containers, LegacyContainerPlan{
			Field:     containerField,
			IndexPath: containerIndexPath,
			Aliases:   aliases,
		})
	}

	return containers, true, nil
}

func resolveLegacyAliases(field reflect.StructField, aliasResolver LegacyAliasResolver) ([]string, bool, error) {
	if aliasResolver == nil {
		return nil, false, nil
	}

	aliases, skip, err := aliasResolver(field)
	if err != nil || skip {
		return nil, skip, err
	}

	return sanitizeAliases(aliases), false, nil
}

func sanitizeAliases(aliases []string) []string {
	if len(aliases) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(aliases))
	sanitized := make([]string, 0, len(aliases))
	for _, alias := range aliases {
		if alias == "" || alias == "-" {
			continue
		}
		if _, exists := seen[alias]; exists {
			continue
		}
		seen[alias] = struct{}{}
		sanitized = append(sanitized, alias)
	}

	if len(sanitized) == 0 {
		return nil
	}

	return sanitized
}

func cloneIndexPath(indexPath []int) []int {
	if len(indexPath) == 0 {
		return nil
	}

	cloned := make([]int, len(indexPath))
	copy(cloned, indexPath)
	return cloned
}

func isAnonymousEmbeddedStruct(field reflect.StructField) bool {
	return field.Anonymous && field.Type.Kind() == reflect.Struct
}
