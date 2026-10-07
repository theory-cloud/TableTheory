package transaction

import (
	"fmt"
	"reflect"
)

// fieldByIndexPath resolves a metadata field index path against a struct value.
//
// The path is validated against the struct shape before reflect dereferences it,
// so metadata that disagrees with the model produces a guard failure instead of a
// reflect panic. For metadata derived from the same type it resolves exactly as
// Value.FieldByIndex does.
func fieldByIndexPath(modelValue reflect.Value, indexPath []int) (reflect.Value, error) {
	if !modelValue.IsValid() || modelValue.Kind() != reflect.Struct {
		return reflect.Value{}, fmt.Errorf("model must be a struct or pointer to struct")
	}
	if len(indexPath) == 0 || !validFieldIndexPath(modelValue.Type(), indexPath) {
		return reflect.Value{}, fmt.Errorf("model field path %v does not resolve on %s", indexPath, modelValue.Type())
	}

	return modelValue.FieldByIndex(indexPath), nil
}

func validFieldIndexPath(structType reflect.Type, indexPath []int) bool {
	current := structType
	for _, index := range indexPath {
		for current.Kind() == reflect.Ptr {
			current = current.Elem()
		}
		if current.Kind() != reflect.Struct || index < 0 || index >= current.NumField() {
			return false
		}
		current = current.Field(index).Type
	}

	return true
}

// modelStructValue resolves a model into its struct value, applying the same guard
// the explicit query path applies so that a nil model is reported rather than
// dereferenced.
func modelStructValue(model any) (reflect.Value, error) {
	modelValue := reflect.ValueOf(model)
	if !modelValue.IsValid() {
		return reflect.Value{}, fmt.Errorf("model cannot be nil")
	}
	if modelValue.Kind() == reflect.Ptr {
		if modelValue.IsNil() {
			return reflect.Value{}, fmt.Errorf("model cannot be nil")
		}
		modelValue = modelValue.Elem()
	}
	if modelValue.Kind() != reflect.Struct {
		return reflect.Value{}, fmt.Errorf("model must be a struct or pointer to struct")
	}

	return modelValue, nil
}
