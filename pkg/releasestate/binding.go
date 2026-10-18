package releasestate

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	theorydbErrors "github.com/theory-cloud/tabletheory/v6/pkg/errors"
	"github.com/theory-cloud/tabletheory/v6/pkg/model"
)

// Binding sources accepted in TransitionAppendEventInput.EventBinding and
// OutboxBinding. A binding makes the audit row's field explicit about which
// part of the authorizing transition it must equal; validation then rejects a
// row that disagrees before anything is submitted.
const (
	bindingSourceSetPrefix    = "set:"
	bindingSourceActualPrefix = "actual:"
	bindingSourceNextVersion  = "nextVersion"
)

// validateEventAndOutboxBindings enforces the generically knowable identity
// checks plus any caller-declared field bindings.
func validateEventAndOutboxBindings(input TransitionAppendEventInput) error {
	actualMetadata, err := registryMetadata(input.Actual)
	if err != nil {
		return err
	}

	if err := validateDeclaredBindings("event", input.Event, input.EventBinding, input, actualMetadata); err != nil {
		return err
	}

	if input.Outbox == nil {
		if len(input.OutboxBinding) > 0 {
			return fmt.Errorf("%w: outbox binding requires an outbox row", theorydbErrors.ErrInvalidModel)
		}
		return nil
	}

	// A shared single-table shape puts the outbox row in the same release
	// partition as the actual row. When both models name the partition key the
	// same way, a mismatch cannot describe the same release.
	if err := validateSharedPartition("outbox", input.Actual, input.Outbox, actualMetadata); err != nil {
		return err
	}

	return validateDeclaredBindings("outbox", input.Outbox, input.OutboxBinding, input, actualMetadata)
}

func validateSharedPartition(label string, actual any, row any, actualMetadata *model.Metadata) error {
	if actualMetadata == nil || actualMetadata.PrimaryKey == nil || actualMetadata.PrimaryKey.PartitionKey == nil {
		return nil
	}
	rowMetadata, err := registryMetadata(row)
	if err != nil {
		return err
	}
	if rowMetadata == nil || rowMetadata.PrimaryKey == nil || rowMetadata.PrimaryKey.PartitionKey == nil {
		return fmt.Errorf("%w: %s row partition key is required", theorydbErrors.ErrInvalidModel, label)
	}
	if rowMetadata.PrimaryKey.PartitionKey.DBName != actualMetadata.PrimaryKey.PartitionKey.DBName {
		return nil
	}

	actualPartition, err := partitionKeyValue(actual)
	if err != nil {
		return err
	}
	rowPartition, err := partitionKeyValue(row)
	if err != nil {
		return err
	}
	if actualPartition != rowPartition {
		return fmt.Errorf(
			"%w: %s release partition %q does not match actual row partition %q",
			theorydbErrors.ErrInvalidModel, label, rowPartition, actualPartition,
		)
	}
	return nil
}

func validateDeclaredBindings(label string, row any, bindings map[string]string, input TransitionAppendEventInput, actualMetadata *model.Metadata) error {
	if len(bindings) == 0 {
		return nil
	}

	rowMetadata, err := registryMetadata(row)
	if err != nil {
		return err
	}

	for attribute, source := range bindings {
		expected, err := resolveBindingSource(source, input, actualMetadata)
		if err != nil {
			return fmt.Errorf("%w: %s binding for %q: %v", theorydbErrors.ErrInvalidModel, label, attribute, err)
		}

		actual, found := fieldValueByDBName(row, rowMetadata, attribute)
		if !found {
			return fmt.Errorf(
				"%w: %s row %s is missing bound attribute %q",
				theorydbErrors.ErrInvalidModel, label, describeModel(row), attribute,
			)
		}
		if !bindingValuesEqual(expected, actual) {
			return fmt.Errorf(
				"%w: %s attribute %q does not match its authorizing transition value",
				theorydbErrors.ErrInvalidModel, label, attribute,
			)
		}
	}

	return nil
}

func resolveBindingSource(source string, input TransitionAppendEventInput, actualMetadata *model.Metadata) (any, error) {
	source = strings.TrimSpace(source)
	switch {
	case source == bindingSourceNextVersion:
		if input.ExpectedVersion == nil {
			return nil, fmt.Errorf("nextVersion requires ExpectedVersion")
		}
		return *input.ExpectedVersion + 1, nil
	case strings.HasPrefix(source, bindingSourceSetPrefix):
		name := strings.TrimSpace(strings.TrimPrefix(source, bindingSourceSetPrefix))
		for field, value := range input.Set {
			if fieldBelongsToModel(field, name, actualMetadata) {
				return value, nil
			}
		}
		return nil, fmt.Errorf("set field %q is not part of the transition", name)
	case strings.HasPrefix(source, bindingSourceActualPrefix):
		name := strings.TrimSpace(strings.TrimPrefix(source, bindingSourceActualPrefix))
		value, found := fieldValueByDBName(input.Actual, actualMetadata, name)
		if !found {
			value, found = fieldValueByName(input.Actual, actualMetadata, name)
		}
		if !found {
			return nil, fmt.Errorf("actual field %q does not exist", name)
		}
		return value, nil
	default:
		return nil, fmt.Errorf("unsupported binding source %q", source)
	}
}

func fieldBelongsToModel(setKey, wanted string, metadata *model.Metadata) bool {
	if setKey == wanted {
		return true
	}
	if metadata == nil {
		return false
	}
	if field, ok := metadata.Fields[setKey]; ok && field != nil {
		return field.DBName == wanted || field.Name == wanted
	}
	if field, ok := metadata.FieldsByDBName[setKey]; ok && field != nil {
		return field.DBName == wanted || field.Name == wanted
	}
	return false
}

func registryMetadata(instance any) (*model.Metadata, error) {
	if instance == nil {
		return nil, fmt.Errorf("%w: release-state row is required", theorydbErrors.ErrInvalidModel)
	}
	registry := model.NewRegistry()
	if err := registry.Register(instance); err != nil {
		return nil, fmt.Errorf("%w: register release-state row: %v", theorydbErrors.ErrInvalidModel, err)
	}
	metadata, err := registry.GetMetadata(instance)
	if err != nil {
		return nil, fmt.Errorf("%w: read release-state row metadata: %v", theorydbErrors.ErrInvalidModel, err)
	}
	return metadata, nil
}

func fieldValueByDBName(instance any, metadata *model.Metadata, dbName string) (any, bool) {
	if metadata == nil || dbName == "" {
		return nil, false
	}
	field, ok := metadata.FieldsByDBName[dbName]
	if !ok || field == nil {
		return nil, false
	}
	return fieldValueByIndexPath(instance, field.IndexPath)
}

func fieldValueByName(instance any, metadata *model.Metadata, name string) (any, bool) {
	if metadata == nil || name == "" {
		return nil, false
	}
	field, ok := metadata.Fields[name]
	if !ok || field == nil {
		return nil, false
	}
	return fieldValueByIndexPath(instance, field.IndexPath)
}

func fieldValueByIndexPath(instance any, indexPath []int) (any, bool) {
	value := reflect.ValueOf(instance)
	for value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil, false
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return nil, false
	}
	field, ok := fieldByIndexSafe(value, indexPath)
	if !ok {
		return nil, false
	}
	return field.Interface(), true
}

func fieldByIndexSafe(value reflect.Value, indexPath []int) (reflect.Value, bool) {
	for _, index := range indexPath {
		if value.Kind() == reflect.Pointer {
			if value.IsNil() {
				return reflect.Value{}, false
			}
			value = value.Elem()
		}
		if value.Kind() != reflect.Struct || index < 0 || index >= value.NumField() {
			return reflect.Value{}, false
		}
		value = value.Field(index)
	}
	if !value.IsValid() || !value.CanInterface() {
		return reflect.Value{}, false
	}
	return value, true
}

func bindingValuesEqual(left, right any) bool {
	if reflect.DeepEqual(left, right) {
		return true
	}

	leftNumber, leftOK := numericValue(left)
	rightNumber, rightOK := numericValue(right)
	if leftOK && rightOK {
		return leftNumber == rightNumber
	}
	return false
}

func numericValue(value any) (float64, bool) {
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() {
		return 0, false
	}
	switch reflected.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(reflected.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(reflected.Uint()), true
	case reflect.Float32, reflect.Float64:
		return reflected.Float(), true
	case reflect.String:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(reflected.String()), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func describeModel(instance any) string {
	if instance == nil {
		return "<nil>"
	}
	value := reflect.TypeOf(instance)
	for value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	return value.Name()
}
