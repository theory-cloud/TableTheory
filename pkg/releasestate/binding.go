package releasestate

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	theorydbErrors "github.com/theory-cloud/tabletheory/v6/pkg/errors"
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
	actualRow, err := describeBindingRow(input.Actual)
	if err != nil {
		return err
	}

	if err := validateDeclaredBindings("event", input.Event, input.EventBinding, input, actualRow); err != nil {
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
	if err := validateSharedPartition(input.Actual, actualRow, input.Outbox); err != nil {
		return err
	}

	return validateDeclaredBindings("outbox", input.Outbox, input.OutboxBinding, input, actualRow)
}

func validateSharedPartition(actual any, actualRow *bindingRow, row any) error {
	rowMetadata, err := describeBindingRow(row)
	if err != nil {
		return err
	}
	if rowMetadata.partitionAttribute == "" {
		return fmt.Errorf("%w: outbox row partition key is required", theorydbErrors.ErrInvalidModel)
	}
	if actualRow.partitionAttribute == "" ||
		rowMetadata.partitionAttribute != actualRow.partitionAttribute {
		return nil
	}

	actualPartition, ok := bindingRowValue(actual, actualRow, actualRow.partitionAttribute)
	if !ok {
		return fmt.Errorf("%w: actual row partition key is required", theorydbErrors.ErrInvalidModel)
	}
	rowPartition, ok := bindingRowValue(row, rowMetadata, rowMetadata.partitionAttribute)
	if !ok {
		return fmt.Errorf("%w: outbox row partition key is required", theorydbErrors.ErrInvalidModel)
	}
	if !bindingValuesEqual(actualPartition, rowPartition) {
		return fmt.Errorf(
			"%w: outbox release partition %v does not match actual row partition %v",
			theorydbErrors.ErrInvalidModel, rowPartition, actualPartition,
		)
	}
	return nil
}

func validateDeclaredBindings(label string, row any, bindings map[string]string, input TransitionAppendEventInput, actualRow *bindingRow) error {
	if len(bindings) == 0 {
		return nil
	}

	rowMetadata, err := describeBindingRow(row)
	if err != nil {
		return err
	}

	for attribute, source := range bindings {
		expected, err := resolveBindingSource(source, input, actualRow)
		if err != nil {
			return fmt.Errorf("%w: %s binding for %q: %v", theorydbErrors.ErrInvalidModel, label, attribute, err)
		}

		actual, found := bindingRowValue(row, rowMetadata, attribute)
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

func resolveBindingSource(source string, input TransitionAppendEventInput, actualRow *bindingRow) (any, error) {
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
			if actualRow.matchesFieldName(field, name) {
				return value, nil
			}
		}
		return nil, fmt.Errorf("set field %q is not part of the transition", name)
	case strings.HasPrefix(source, bindingSourceActualPrefix):
		name := strings.TrimSpace(strings.TrimPrefix(source, bindingSourceActualPrefix))
		value, found := bindingRowValue(input.Actual, actualRow, name)
		if !found {
			return nil, fmt.Errorf("actual field %q does not exist", name)
		}
		return value, nil
	default:
		return nil, fmt.Errorf("unsupported binding source %q", source)
	}
}

// bindingField is one exported, nameable field on a release-state row.
type bindingField struct {
	name      string
	dbName    string
	indexPath []int
}

// bindingRow is a lightweight, policy-free view of a release-state row. It
// deliberately avoids the model registry: the registry validates write-policy
// metadata on registration, and a binding check must not fail merely because a
// row carries a protected attribute this helper does not interpret.
type bindingRow struct {
	fieldsByDBName     map[string]bindingField
	fieldsByName       map[string]bindingField
	partitionAttribute string
	sortAttribute      string
}

func describeBindingRow(instance any) (*bindingRow, error) {
	value := reflect.ValueOf(instance)
	for value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil, fmt.Errorf("%w: release-state row must not be a nil pointer", theorydbErrors.ErrInvalidModel)
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return nil, fmt.Errorf("%w: release-state row must be a struct", theorydbErrors.ErrInvalidModel)
	}

	metadata := &bindingRow{
		fieldsByDBName: make(map[string]bindingField),
		fieldsByName:   make(map[string]bindingField),
	}
	collectBindingFields(value.Type(), nil, metadata)
	return metadata, nil
}

type bindingRoles struct {
	partition bool
	sort      bool
}

func collectBindingFields(modelType reflect.Type, prefix []int, metadata *bindingRow) {
	for i := 0; i < modelType.NumField(); i++ {
		field := modelType.Field(i)
		path := make([]int, 0, len(prefix)+1)
		path = append(path, prefix...)
		path = append(path, i)

		if collectEmbeddedBindingFields(field, path, metadata) {
			continue
		}
		if !field.IsExported() || field.Tag.Get("theorydb") == "-" {
			continue
		}

		entry, roles := bindingFieldFor(field, path)
		metadata.add(entry, roles)
	}
}

func collectEmbeddedBindingFields(field reflect.StructField, path []int, metadata *bindingRow) bool {
	if !field.Anonymous {
		return false
	}
	embedded := field.Type
	for embedded.Kind() == reflect.Pointer {
		embedded = embedded.Elem()
	}
	if embedded.Kind() != reflect.Struct {
		return false
	}
	collectBindingFields(embedded, path, metadata)
	return true
}

func bindingFieldFor(field reflect.StructField, path []int) (bindingField, bindingRoles) {
	dbName := ""
	var roles bindingRoles
	for _, raw := range strings.Split(field.Tag.Get("theorydb"), ",") {
		part := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(part, "attr:"):
			dbName = strings.TrimSpace(strings.TrimPrefix(part, "attr:"))
		case part == "pk":
			roles.partition = true
		case part == "sk":
			roles.sort = true
		}
	}
	if dbName == "" {
		jsonName := strings.Split(field.Tag.Get("json"), ",")[0]
		if jsonName != "" && jsonName != "-" {
			dbName = jsonName
		}
	}
	if dbName == "" {
		dbName = field.Name
	}
	return bindingField{name: field.Name, dbName: dbName, indexPath: path}, roles
}

func (r *bindingRow) add(entry bindingField, roles bindingRoles) {
	if _, exists := r.fieldsByDBName[entry.dbName]; !exists {
		r.fieldsByDBName[entry.dbName] = entry
	}
	if _, exists := r.fieldsByName[entry.name]; !exists {
		r.fieldsByName[entry.name] = entry
	}
	if roles.partition && r.partitionAttribute == "" {
		r.partitionAttribute = entry.dbName
	}
	if roles.sort && r.sortAttribute == "" {
		r.sortAttribute = entry.dbName
	}
}

func (r *bindingRow) field(name string) (bindingField, bool) {
	if r == nil {
		return bindingField{}, false
	}
	if field, ok := r.fieldsByDBName[name]; ok {
		return field, true
	}
	field, ok := r.fieldsByName[name]
	return field, ok
}

func (r *bindingRow) matchesFieldName(candidate, wanted string) bool {
	if candidate == wanted {
		return true
	}
	candidateField, ok := r.field(candidate)
	if !ok {
		return false
	}
	wantedField, ok := r.field(wanted)
	if !ok {
		return false
	}
	return candidateField.dbName == wantedField.dbName
}

func bindingRowValue(instance any, metadata *bindingRow, name string) (any, bool) {
	field, ok := metadata.field(name)
	if !ok {
		return nil, false
	}
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
	fieldValue, ok := fieldByIndexSafe(value, field.indexPath)
	if !ok {
		return nil, false
	}
	return fieldValue.Interface(), true
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
