package releasestate

import (
	"fmt"
	"math/big"
	"reflect"
	"strings"

	theorydbErrors "github.com/theory-cloud/tabletheory/v6/pkg/errors"
	"github.com/theory-cloud/tabletheory/v6/pkg/model"
	"github.com/theory-cloud/tabletheory/v6/pkg/naming"
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

	// Prefer the model registry: it is the same metadata the write path uses
	// to choose key roles and attribute names, including index-clause scoping
	// (`index:gsiN,pk`), embedded exclusion tags, and the model's naming
	// convention.
	if metadata, ok := describeBindingRowFromRegistry(instance); ok {
		return metadata, nil
	}
	return describeBindingRowStructurally(value.Type())
}

// describeBindingRowFromRegistry builds the binding view from registry
// metadata. Validation-triggering registrations (for example, an invalid
// write-policy tag) fall back to the structural parser below so a binding
// check never fails merely because it cannot interpret an unrelated policy.
func describeBindingRowFromRegistry(instance any) (*bindingRow, bool) {
	registry := model.NewRegistry()
	if err := registry.Register(instance); err != nil {
		return nil, false
	}
	modelMetadata, err := registry.GetMetadata(instance)
	if err != nil || modelMetadata.PrimaryKey == nil || modelMetadata.PrimaryKey.PartitionKey == nil {
		return nil, false
	}

	metadata := newBindingRow()
	for _, field := range modelMetadata.FieldsByDBName {
		entry := bindingField{
			name:      field.Name,
			dbName:    field.DBName,
			indexPath: field.IndexPath,
		}
		metadata.fieldsByDBName[entry.dbName] = entry
		if _, exists := metadata.fieldsByName[entry.name]; !exists {
			metadata.fieldsByName[entry.name] = entry
		}
	}
	metadata.partitionAttribute = modelMetadata.PrimaryKey.PartitionKey.DBName
	if modelMetadata.PrimaryKey.SortKey != nil {
		metadata.sortAttribute = modelMetadata.PrimaryKey.SortKey.DBName
	}
	return metadata, true
}

func newBindingRow() *bindingRow {
	return &bindingRow{
		fieldsByDBName: make(map[string]bindingField),
		fieldsByName:   make(map[string]bindingField),
	}
}

type bindingRoles struct {
	partition bool
	sort      bool
}

// describeBindingRowStructurally mirrors the registry's field rules for rows
// that cannot register. It resolves attribute names through the naming
// convention (never json tags), scopes index-clause key modifiers, honors
// embedded exclusion tags, and rejects duplicate database names.
func describeBindingRowStructurally(modelType reflect.Type) (*bindingRow, error) {
	metadata := newBindingRow()
	convention := detectBindingNamingConvention(modelType)
	if err := collectBindingFields(modelType, nil, metadata, convention); err != nil {
		return nil, err
	}
	return metadata, nil
}

func collectBindingFields(modelType reflect.Type, prefix []int, metadata *bindingRow, convention naming.Convention) error {
	for i := 0; i < modelType.NumField(); i++ {
		field := modelType.Field(i)
		path := make([]int, 0, len(prefix)+1)
		path = append(path, prefix...)
		path = append(path, i)

		if isEmbeddedBindingStruct(field) {
			if hasAnonymousEmbedExclusionTag(field) {
				continue
			}
			if err := collectBindingFields(field.Type, path, metadata, convention); err != nil {
				return err
			}
			continue
		}
		if !field.IsExported() || field.Tag.Get("theorydb") == "-" {
			continue
		}

		dbName, skip := naming.ResolveAttrNameWithConvention(field, convention)
		if skip || dbName == "" {
			continue
		}
		entry := bindingField{name: field.Name, dbName: dbName, indexPath: path}
		if err := metadata.add(entry, bindingRolesFor(field.Tag.Get("theorydb"))); err != nil {
			return err
		}
	}
	return nil
}

func isEmbeddedBindingStruct(field reflect.StructField) bool {
	return field.Anonymous && field.Type.Kind() == reflect.Struct
}

func hasAnonymousEmbedExclusionTag(field reflect.StructField) bool {
	return field.Tag.Get("theorydb") == "-" || field.Tag.Get("json") == "-"
}

// bindingRolesFor extracts key roles from theorydb tag tokens. Key modifiers
// inside an index/lsi clause describe the index, not the table partition, so
// they are skipped — matching the registry's splitTags behavior.
func bindingRolesFor(tag string) bindingRoles {
	var roles bindingRoles
	inIndexClause := false
	for _, raw := range strings.Split(tag, ",") {
		part := strings.TrimSpace(raw)
		if part == "" {
			continue
		}
		if inIndexClause {
			switch part {
			case "pk", "sk", "sparse":
				continue
			}
			inIndexClause = false
		}
		switch {
		case strings.HasPrefix(part, "index:"), strings.HasPrefix(part, "lsi:"):
			inIndexClause = true
		case part == "pk":
			roles.partition = true
		case part == "sk":
			roles.sort = true
		}
	}
	return roles
}

func detectBindingNamingConvention(modelType reflect.Type) naming.Convention {
	for i := 0; i < modelType.NumField(); i++ {
		tag := modelType.Field(i).Tag.Get("theorydb")
		if tag == "" {
			continue
		}
		for _, raw := range strings.Split(tag, ",") {
			part := strings.TrimSpace(raw)
			if !strings.HasPrefix(part, "naming:") {
				continue
			}
			switch strings.TrimPrefix(part, "naming:") {
			case "snake_case":
				return naming.SnakeCase
			case "camel_case", "camelCase":
				return naming.CamelCase
			case "pascal_case", "pascalCase":
				return naming.PascalCase
			case "dynamorm", "legacy_dynamorm", "legacyDynamORM":
				return naming.DynamORM
			}
		}
	}
	return naming.CamelCase
}

func (r *bindingRow) add(entry bindingField, roles bindingRoles) error {
	if existing, exists := r.fieldsByDBName[entry.dbName]; exists {
		cause := theorydbErrors.ErrInvalidModel
		if roles.partition || roles.sort ||
			entry.dbName == r.partitionAttribute || entry.dbName == r.sortAttribute {
			cause = theorydbErrors.ErrDuplicatePrimaryKey
		}
		return fmt.Errorf(
			"%w: duplicate database attribute name %q for fields %s and %s",
			cause, entry.dbName, existing.name, entry.name,
		)
	}
	r.fieldsByDBName[entry.dbName] = entry
	if _, exists := r.fieldsByName[entry.name]; !exists {
		r.fieldsByName[entry.name] = entry
	}
	if roles.partition {
		if r.partitionAttribute != "" {
			return fmt.Errorf("%w: duplicate primary key definition", theorydbErrors.ErrDuplicatePrimaryKey)
		}
		r.partitionAttribute = entry.dbName
	}
	if roles.sort {
		if r.sortAttribute != "" {
			return fmt.Errorf("%w: duplicate sort key definition", theorydbErrors.ErrDuplicatePrimaryKey)
		}
		r.sortAttribute = entry.dbName
	}
	return nil
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

// bindingValuesEqual compares a bound row value with the value the
// authorizing transition supplies. Equality is type-directed: strings compare
// lexically and never satisfy numbers, while numeric kinds compare as exact
// rational values. In particular, no value is ever converted to binary64, so
// adjacent integers above 2^53 remain distinct.
func bindingValuesEqual(left, right any) bool {
	if reflect.DeepEqual(left, right) {
		return true
	}

	leftNumber, leftOK := exactNumber(left)
	rightNumber, rightOK := exactNumber(right)
	if leftOK || rightOK {
		return leftOK && rightOK && leftNumber.Cmp(rightNumber) == 0
	}
	return bindingContainersEqual(reflect.ValueOf(left), reflect.ValueOf(right))
}

func exactNumber(value any) (*big.Rat, bool) {
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() {
		return nil, false
	}
	switch reflected.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return new(big.Rat).SetInt64(reflected.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return new(big.Rat).SetUint64(reflected.Uint()), true
	case reflect.Float32, reflect.Float64:
		number := new(big.Rat).SetFloat64(reflected.Float())
		if number == nil {
			return nil, false
		}
		return number, true
	default:
		return nil, false
	}
}

// bindingContainersEqual recurses through maps, slices, and arrays so nested
// values follow the same exact rules as top-level bindings. Structs keep the
// DeepEqual behavior above.
func bindingContainersEqual(left, right reflect.Value) bool {
	if !left.IsValid() || !right.IsValid() || left.Kind() != right.Kind() {
		return false
	}
	switch left.Kind() {
	case reflect.Map:
		if left.IsNil() != right.IsNil() || left.Len() != right.Len() {
			return false
		}
		for _, key := range left.MapKeys() {
			rightValue := right.MapIndex(key)
			if !rightValue.IsValid() {
				return false
			}
			if !bindingValuesEqual(left.MapIndex(key).Interface(), rightValue.Interface()) {
				return false
			}
		}
		return true
	case reflect.Slice, reflect.Array:
		if left.Len() != right.Len() {
			return false
		}
		for index := 0; index < left.Len(); index++ {
			if !bindingValuesEqual(left.Index(index).Interface(), right.Index(index).Interface()) {
				return false
			}
		}
		return true
	default:
		return false
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
