package expr

import (
	"fmt"
	"reflect"

	"github.com/theory-cloud/tabletheory/v3/internal/reflectutil"
)

type marshalFieldConverterLookup interface {
	HasCustomConverter(typ reflect.Type) bool
}

var marshalerInterfaceType = reflect.TypeOf((*Marshaler)(nil)).Elem()

// BuildMarshalVisibleFieldPlan enumerates the exported fields that helper
// marshaling should visit for a struct. Unlike reflectutil.BuildVisibleFieldPlan,
// this helper can preserve anonymous embedded struct containers as terminal
// fields when they carry custom converter or Marshaler hooks so those hooks run
// before promoted-field traversal would otherwise bypass them.
func BuildMarshalVisibleFieldPlan(modelType reflect.Type, converter any, honorMarshalers bool) ([]reflectutil.VisibleFieldPlan, error) {
	structType, err := resolveMarshalStructType(modelType)
	if err != nil {
		return nil, err
	}

	visibleFields := reflect.VisibleFields(structType)
	plans := make([]reflectutil.VisibleFieldPlan, 0, len(visibleFields))
	terminalPrefixes := make([][]int, 0, len(visibleFields))

	for _, field := range visibleFields {
		if !field.IsExported() {
			continue
		}
		if indexHasTerminalPrefix(field.Index, terminalPrefixes) {
			continue
		}

		if isMarshalTerminalAnonymousEmbed(field, converter, honorMarshalers) {
			plans = append(plans, reflectutil.VisibleFieldPlan{
				Field:     field,
				IndexPath: cloneMarshalIndexPath(field.Index),
			})
			terminalPrefixes = append(terminalPrefixes, cloneMarshalIndexPath(field.Index))
			continue
		}

		if isMarshalAnonymousStructContainer(field) {
			continue
		}

		include, err := includeMarshalVisibleField(structType, field.Index)
		if err != nil {
			return nil, err
		}
		if !include {
			continue
		}

		plans = append(plans, reflectutil.VisibleFieldPlan{
			Field:     field,
			IndexPath: cloneMarshalIndexPath(field.Index),
		})
	}

	return plans, nil
}

func resolveMarshalStructType(modelType reflect.Type) (reflect.Type, error) {
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

func isMarshalTerminalAnonymousEmbed(field reflect.StructField, converter any, honorMarshalers bool) bool {
	if !field.Anonymous {
		return false
	}

	fieldType := field.Type
	if fieldType.Kind() == reflect.Ptr {
		if fieldType.Elem().Kind() != reflect.Struct {
			return false
		}
	} else if fieldType.Kind() != reflect.Struct {
		return false
	}

	if honorMarshalers {
		if fieldType.Implements(marshalerInterfaceType) {
			return true
		}
		if fieldType.Kind() != reflect.Ptr && reflect.PointerTo(fieldType).Implements(marshalerInterfaceType) {
			return true
		}
	}

	lookup, ok := converter.(marshalFieldConverterLookup)
	if !ok || isNilMarshalFieldLookup(converter) {
		return false
	}
	if lookup.HasCustomConverter(fieldType) {
		return true
	}
	if fieldType.Kind() != reflect.Ptr && lookup.HasCustomConverter(reflect.PointerTo(fieldType)) {
		return true
	}
	return false
}

func isNilMarshalFieldLookup(converter any) bool {
	if converter == nil {
		return true
	}

	rv := reflect.ValueOf(converter)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

func isMarshalAnonymousStructContainer(field reflect.StructField) bool {
	return field.Anonymous && field.Type.Kind() == reflect.Struct
}

func includeMarshalVisibleField(modelType reflect.Type, indexPath []int) (bool, error) {
	if len(indexPath) <= 1 {
		return true, nil
	}

	for depth := 1; depth < len(indexPath); depth++ {
		containerField := modelType.FieldByIndex(indexPath[:depth])
		if !containerField.Anonymous {
			return false, nil
		}
		if !containerField.IsExported() {
			return false, nil
		}
		if containerField.Type.Kind() != reflect.Struct {
			return false, nil
		}
	}

	return true, nil
}

func indexHasTerminalPrefix(indexPath []int, terminalPrefixes [][]int) bool {
	for _, prefix := range terminalPrefixes {
		if len(prefix) == 0 || len(prefix) >= len(indexPath) {
			continue
		}
		if reflect.DeepEqual(indexPath[:len(prefix)], prefix) {
			return true
		}
	}
	return false
}

func cloneMarshalIndexPath(indexPath []int) []int {
	if len(indexPath) == 0 {
		return nil
	}

	cloned := make([]int, len(indexPath))
	copy(cloned, indexPath)
	return cloned
}
