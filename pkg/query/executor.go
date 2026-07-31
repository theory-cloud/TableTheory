package query

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/theory-cloud/tabletheory/v3/internal/fieldcodec"
	"github.com/theory-cloud/tabletheory/v3/internal/reflectutil"
	customerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/naming"
)

// UnmarshalItems unmarshals DynamoDB items into the destination.
// This function is exported for use with DynamoDB streams and other external data sources.
func UnmarshalItems(items []map[string]types.AttributeValue, dest any) error {
	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr {
		return fmt.Errorf("destination must be a pointer")
	}

	destElem := destValue.Elem()

	// Handle single item result
	if destElem.Kind() != reflect.Slice {
		if len(items) == 0 {
			return fmt.Errorf("no items found")
		}
		// For single item, unmarshal the first item
		return UnmarshalItem(items[0], dest)
	}

	// Handle slice result
	sliceType := destElem.Type()
	itemType := sliceType.Elem()

	// Create a new slice with the appropriate capacity
	newSlice := reflect.MakeSlice(sliceType, 0, len(items))

	for _, item := range items {
		// Create a new instance of the item type
		newItem := reflect.New(itemType)
		if itemType.Kind() == reflect.Ptr {
			newItem = reflect.New(itemType.Elem())
		}

		// Unmarshal the item
		if err := UnmarshalItem(item, newItem.Interface()); err != nil {
			return fmt.Errorf("failed to unmarshal item: %w", err)
		}

		// Append to slice
		if itemType.Kind() == reflect.Ptr {
			newSlice = reflect.Append(newSlice, newItem)
		} else {
			newSlice = reflect.Append(newSlice, newItem.Elem())
		}
	}

	// Set the result
	destElem.Set(newSlice)
	return nil
}

// UnmarshalItem unmarshals a single DynamoDB item into a Go struct.
// This function respects both "dynamodb" and "theorydb" struct tags.
func UnmarshalItem(item map[string]types.AttributeValue, dest any) error {
	destElem, destType, convention, err := resolveUnmarshalTarget(dest)
	if err != nil {
		return err
	}

	fieldPlans, err := buildUnmarshalFieldPlans(destType, convention)
	if err != nil {
		return err
	}

	for _, fieldPlan := range fieldPlans {
		if err := unmarshalItemFieldWithPlan(item, fieldPlan, destElem.FieldByIndex(fieldPlan.IndexPath), convention); err != nil {
			return err
		}
	}

	return nil
}

func resolveUnmarshalTarget(dest any) (reflect.Value, reflect.Type, naming.Convention, error) {
	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr || destValue.IsNil() {
		return reflect.Value{}, nil, naming.CamelCase, fmt.Errorf("destination must be a pointer")
	}

	destElem := destValue.Elem()
	if destElem.Kind() != reflect.Struct {
		return reflect.Value{}, nil, naming.CamelCase, fmt.Errorf("destination must be a pointer to a struct")
	}

	destType := destElem.Type()
	return destElem, destType, detectNamingConvention(destType), nil
}

func unmarshalItemField(item map[string]types.AttributeValue, field reflect.StructField, dest reflect.Value, convention naming.Convention) error {
	return unmarshalItemFieldWithPlan(item, reflectutil.VisibleFieldPlan{
		Field:     field,
		IndexPath: field.Index,
	}, dest, convention)
}

func unmarshalItemFieldWithPlan(item map[string]types.AttributeValue, fieldPlan reflectutil.VisibleFieldPlan, dest reflect.Value, convention naming.Convention) error {
	field := fieldPlan.Field
	attrNames, skip := resolveUnmarshalFieldLookupNames(field, convention)
	if skip {
		return nil
	}

	av, exists, err := lookupUnmarshalFieldPlanValue(item, fieldPlan, attrNames)
	if err != nil {
		return fmt.Errorf("failed to locate field %s: %w", field.Name, err)
	}
	if !exists {
		return nil
	}

	if fieldHasEncryptedTag(field) && looksLikeEncryptedEnvelope(av) {
		return &customerrors.EncryptedFieldError{
			Operation: "decrypt",
			Field:     field.Name,
			Err:       customerrors.ErrEncryptionNotConfigured,
		}
	}

	if fieldHasJSONTag(field) {
		if err := fieldcodec.UnmarshalJSONFieldValue(av, dest, func() error {
			return unmarshalAttributeValueWithConvention(av, dest, convention, true)
		}); err != nil {
			return fmt.Errorf("failed to unmarshal field %s: %w", field.Name, err)
		}
		return nil
	}

	if err := unmarshalAttributeValueWithConvention(av, dest, convention, true); err != nil {
		return fmt.Errorf("failed to unmarshal field %s: %w", field.Name, err)
	}

	return nil
}

func resolveUnmarshalFieldLookupNames(field reflect.StructField, convention naming.Convention) ([]string, bool) {
	dynamodbTag := field.Tag.Get("dynamodb")
	theorydbTag := field.Tag.Get("theorydb")
	jsonTag := field.Tag.Get("json")
	if dynamodbTag == "-" || theorydbTag == "-" || jsonTag == "-" {
		return nil, true
	}

	names := make([]string, 0, 4)
	if dynamodbTag != "" {
		attrName := parseAttributeName(dynamodbTag)
		if attrName == "" {
			attrName = field.Name
		}
		names = appendLookupName(names, attrName)
		return appendDefaultLookupNames(names, field, convention), false
	}

	if theorydbTag != "" {
		attrName, skip := naming.ResolveAttrNameWithConvention(field, convention)
		if skip || attrName == "" {
			return nil, true
		}
		names = appendLookupName(names, attrName)
	}

	if jsonName := parseJSONAttributeName(jsonTag); jsonName != "" {
		names = appendLookupName(names, jsonName)
	}

	names = appendDefaultLookupNames(names, field, convention)
	if len(names) == 0 {
		return nil, true
	}

	return names, false
}

func buildUnmarshalFieldPlans(destType reflect.Type, convention naming.Convention) ([]reflectutil.VisibleFieldPlan, error) {
	return reflectutil.BuildVisibleFieldPlan(destType, func(field reflect.StructField) ([]string, bool, error) {
		names, skip := resolveUnmarshalFieldLookupNames(field, convention)
		return names, skip, nil
	})
}

func appendDefaultLookupNames(names []string, field reflect.StructField, convention naming.Convention) []string {
	names = appendLookupName(names, naming.ConvertAttrName(field.Name, convention))
	names = appendLookupName(names, field.Name)
	return names
}

func appendLookupName(names []string, name string) []string {
	if name == "" || name == "-" {
		return names
	}
	for _, existing := range names {
		if existing == name {
			return names
		}
	}
	return append(names, name)
}

func lookupAttributeValue(values map[string]types.AttributeValue, names ...string) (types.AttributeValue, bool) {
	for _, name := range names {
		av, exists := values[name]
		if exists {
			return av, true
		}
	}
	return nil, false
}

func lookupUnmarshalFieldPlanValue(
	values map[string]types.AttributeValue,
	fieldPlan reflectutil.VisibleFieldPlan,
	names []string,
) (types.AttributeValue, bool, error) {
	if av, exists := lookupAttributeValue(values, names...); exists {
		return av, true, nil
	}

	return lookupLegacyUnmarshalFieldPlanValue(values, fieldPlan.LegacyContainers, names)
}

func lookupLegacyUnmarshalFieldPlanValue(
	values map[string]types.AttributeValue,
	containers []reflectutil.LegacyContainerPlan,
	names []string,
) (types.AttributeValue, bool, error) {
	if len(containers) == 0 {
		return nil, false, nil
	}

	current := values
	for _, container := range containers {
		if len(container.Aliases) == 0 {
			return nil, false, nil
		}

		av, exists := lookupAttributeValue(current, container.Aliases...)
		if !exists {
			return nil, false, nil
		}

		nested, ok := av.(*types.AttributeValueMemberM)
		if !ok {
			return nil, false, fmt.Errorf(
				"legacy anonymous container %s must decode from a map, got %T",
				container.Field.Name,
				av,
			)
		}

		current = nested.Value
	}

	av, exists := lookupAttributeValue(current, names...)
	return av, exists, nil
}

func parseJSONAttributeName(tag string) string {
	if tag == "" || tag == "-" {
		return ""
	}

	parts := strings.Split(tag, ",")
	if len(parts) == 0 {
		return ""
	}

	name := strings.TrimSpace(parts[0])
	if name == "" || name == "-" {
		return ""
	}

	return name
}

// unmarshalAttributeValue unmarshals a DynamoDB attribute value into a reflect.Value
func unmarshalAttributeValue(av types.AttributeValue, dest reflect.Value) error {
	return unmarshalAttributeValueWithConvention(av, dest, naming.CamelCase, true)
}

func unmarshalAttributeValueWithConvention(av types.AttributeValue, dest reflect.Value, inheritedConvention naming.Convention, inheritNaming bool) error {
	if !dest.CanSet() {
		return fmt.Errorf("cannot set value")
	}

	if dest.Kind() == reflect.Ptr {
		return unmarshalPointerAttributeValueWithConvention(av, dest, inheritedConvention, inheritNaming)
	}

	if dest.Kind() == reflect.Interface && dest.Type().NumMethod() == 0 {
		return unmarshalAnyAttributeValue(av, dest)
	}

	switch v := av.(type) {
	case *types.AttributeValueMemberS:
		return unmarshalStringAttribute(v.Value, dest)
	case *types.AttributeValueMemberN:
		return unmarshalNumberAttribute(v.Value, dest)
	case *types.AttributeValueMemberBOOL:
		return unmarshalBoolAttribute(v.Value, dest)
	case *types.AttributeValueMemberNULL:
		dest.Set(reflect.Zero(dest.Type()))
		return nil
	case *types.AttributeValueMemberL:
		return unmarshalListAttributeWithConvention(v.Value, dest, inheritedConvention, inheritNaming)
	case *types.AttributeValueMemberM:
		return unmarshalMapAttributeWithConvention(v.Value, dest, inheritedConvention, inheritNaming)
	case *types.AttributeValueMemberSS:
		return unmarshalStringSetAttribute(v.Value, dest)
	case *types.AttributeValueMemberNS:
		return unmarshalNumberSetAttribute(v.Value, dest)
	case *types.AttributeValueMemberBS:
		return unmarshalBinarySetAttribute(v.Value, dest)
	case *types.AttributeValueMemberB:
		return unmarshalBinaryAttribute(v.Value, dest)
	default:
		return fmt.Errorf("unsupported attribute value type: %T", av)
	}
}

func unmarshalPointerAttributeValueWithConvention(av types.AttributeValue, dest reflect.Value, inheritedConvention naming.Convention, inheritNaming bool) error {
	if av == nil {
		dest.Set(reflect.Zero(dest.Type()))
		return nil
	}
	if _, ok := av.(*types.AttributeValueMemberNULL); ok {
		dest.Set(reflect.Zero(dest.Type()))
		return nil
	}
	if dest.IsNil() {
		dest.Set(reflect.New(dest.Type().Elem()))
	}
	return unmarshalAttributeValueWithConvention(av, dest.Elem(), inheritedConvention, inheritNaming)
}

func unmarshalAnyAttributeValue(av types.AttributeValue, dest reflect.Value) error {
	value, err := attributeValueToInterface(av)
	if err != nil {
		return err
	}
	if value == nil {
		dest.Set(reflect.Zero(dest.Type()))
		return nil
	}
	dest.Set(reflect.ValueOf(value))
	return nil
}

func unmarshalStringAttribute(value string, dest reflect.Value) error {
	switch dest.Kind() {
	case reflect.String:
		dest.SetString(value)
		return nil
	case reflect.Struct:
		return unmarshalStringToStruct(value, dest)
	case reflect.Map, reflect.Slice:
		return unmarshalJSONString(value, dest)
	default:
		return fmt.Errorf("cannot unmarshal string into %v", dest.Kind())
	}
}

func unmarshalStringToStruct(value string, dest reflect.Value) error {
	if dest.Type() == reflect.TypeOf(time.Time{}) {
		t, err := parseTimeString(value)
		if err != nil {
			return err
		}
		dest.Set(reflect.ValueOf(t))
		return nil
	}
	return unmarshalJSONString(value, dest)
}

func parseTimeString(value string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, value)
	if err == nil {
		return t, nil
	}

	t, err = time.Parse(time.RFC3339Nano, value)
	if err == nil {
		return t, nil
	}

	unix, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse time from string %q: %w", value, err)
	}
	return time.Unix(unix, 0), nil
}

func unmarshalJSONString(value string, dest reflect.Value) error {
	if !dest.CanAddr() {
		return fmt.Errorf("cannot unmarshal JSON string into %v", dest.Kind())
	}
	if err := json.Unmarshal([]byte(value), dest.Addr().Interface()); err != nil {
		return fmt.Errorf("failed to unmarshal JSON string: %w", err)
	}
	return nil
}

func unmarshalNumberAttribute(value string, dest reflect.Value) error {
	switch dest.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return err
		}
		dest.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return err
		}
		dest.SetUint(n)
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return err
		}
		dest.SetFloat(f)
	}
	return nil
}

func unmarshalBoolAttribute(value bool, dest reflect.Value) error {
	if dest.Kind() != reflect.Bool {
		return fmt.Errorf("cannot unmarshal bool into %v", dest.Kind())
	}
	dest.SetBool(value)
	return nil
}

func unmarshalListAttributeWithConvention(values []types.AttributeValue, dest reflect.Value, inheritedConvention naming.Convention, inheritNaming bool) error {
	if dest.Kind() != reflect.Slice && dest.Kind() != reflect.Array {
		return fmt.Errorf("cannot unmarshal list into non-slice type")
	}

	destination := dest
	if dest.Kind() == reflect.Slice {
		destination = reflect.MakeSlice(dest.Type(), len(values), len(values))
	} else if dest.Len() != len(values) {
		return fmt.Errorf("list length %d does not match array length %d", len(values), dest.Len())
	}

	for i, item := range values {
		if err := unmarshalAttributeValueWithConvention(item, destination.Index(i), inheritedConvention, inheritNaming); err != nil {
			return err
		}
	}
	if dest.Kind() == reflect.Slice {
		dest.Set(destination)
	}
	return nil
}

func unmarshalMapAttributeWithConvention(values map[string]types.AttributeValue, dest reflect.Value, inheritedConvention naming.Convention, inheritNaming bool) error {
	switch dest.Kind() {
	case reflect.Map:
		return unmarshalMapIntoMapWithConvention(values, dest, inheritedConvention, inheritNaming)
	case reflect.Struct:
		return unmarshalMapIntoStructWithConvention(values, dest, inheritedConvention, inheritNaming)
	default:
		return nil
	}
}

func unmarshalMapIntoMapWithConvention(values map[string]types.AttributeValue, dest reflect.Value, inheritedConvention naming.Convention, inheritNaming bool) error {
	mapType := dest.Type()
	keyType := mapType.Key()
	if keyType.Kind() != reflect.String {
		return fmt.Errorf("cannot unmarshal map into %v", dest.Kind())
	}

	elemType := mapType.Elem()
	newMap := reflect.MakeMap(mapType)
	for key, mapVal := range values {
		keyValue := reflect.New(keyType).Elem()
		keyValue.SetString(key)

		elemValue := reflect.New(elemType).Elem()
		if err := unmarshalAttributeValueWithConvention(mapVal, elemValue, inheritedConvention, inheritNaming); err != nil {
			return err
		}
		newMap.SetMapIndex(keyValue, elemValue)
	}
	dest.Set(newMap)
	return nil
}

func unmarshalMapIntoStructWithConvention(values map[string]types.AttributeValue, dest reflect.Value, inheritedConvention naming.Convention, inheritNaming bool) error {
	destType := dest.Type()
	convention, _ := resolveStructNaming(destType, inheritedConvention, inheritNaming)

	fieldPlans, err := buildUnmarshalFieldPlans(destType, convention)
	if err != nil {
		return err
	}

	for _, fieldPlan := range fieldPlans {
		attrNames, skip := resolveUnmarshalFieldLookupNames(fieldPlan.Field, convention)
		if skip {
			continue
		}

		structVal, ok, err := lookupUnmarshalFieldPlanValue(values, fieldPlan, attrNames)
		if err != nil {
			return fmt.Errorf("field %s: %w", fieldPlan.Field.Name, err)
		}
		if !ok {
			continue
		}
		if err := unmarshalAttributeValueWithConvention(structVal, dest.FieldByIndex(fieldPlan.IndexPath), convention, true); err != nil {
			return err
		}
	}
	return nil
}

func unmarshalStringSetAttribute(values []string, dest reflect.Value) error {
	if dest.Kind() != reflect.Slice || dest.Type().Elem().Kind() != reflect.String {
		return fmt.Errorf("cannot unmarshal string set into %v", dest.Kind())
	}

	slice := reflect.MakeSlice(dest.Type(), len(values), len(values))
	for i, value := range values {
		slice.Index(i).SetString(value)
	}
	dest.Set(slice)
	return nil
}

func unmarshalNumberSetAttribute(values []string, dest reflect.Value) error {
	if dest.Kind() != reflect.Slice {
		return fmt.Errorf("cannot unmarshal number set into %v", dest.Kind())
	}

	slice := reflect.MakeSlice(dest.Type(), len(values), len(values))
	for i, value := range values {
		if err := unmarshalNumberAttribute(value, slice.Index(i)); err != nil {
			return err
		}
	}
	dest.Set(slice)
	return nil
}

func unmarshalBinarySetAttribute(values [][]byte, dest reflect.Value) error {
	if dest.Kind() != reflect.Slice || dest.Type().Elem().Kind() != reflect.Slice || dest.Type().Elem().Elem().Kind() != reflect.Uint8 {
		return fmt.Errorf("cannot unmarshal binary set into %v", dest.Kind())
	}

	slice := reflect.MakeSlice(dest.Type(), len(values), len(values))
	for i, value := range values {
		slice.Index(i).SetBytes(value)
	}
	dest.Set(slice)
	return nil
}

func unmarshalBinaryAttribute(value []byte, dest reflect.Value) error {
	if dest.Kind() != reflect.Slice || dest.Type().Elem().Kind() != reflect.Uint8 {
		return fmt.Errorf("cannot unmarshal binary into %v", dest.Kind())
	}
	dest.SetBytes(value)
	return nil
}

// parseAttributeName extracts the attribute name from a DynamoDB tag, ignoring modifiers like omitempty
func parseAttributeName(tag string) string {
	// Split by comma to separate attribute name from modifiers
	parts := strings.Split(tag, ",")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}

func detectNamingConvention(modelType reflect.Type) naming.Convention {
	for i := 0; i < modelType.NumField(); i++ {
		field := modelType.Field(i)
		tag := field.Tag.Get("theorydb")
		if tag == "" {
			continue
		}

		parts := strings.Split(tag, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if !strings.HasPrefix(part, "naming:") {
				continue
			}

			convention := strings.TrimPrefix(part, "naming:")
			switch convention {
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

func explicitNamingConvention(modelType reflect.Type) (naming.Convention, bool) {
	for i := 0; i < modelType.NumField(); i++ {
		tag := modelType.Field(i).Tag.Get("theorydb")
		if tag == "" {
			continue
		}

		parts := strings.Split(tag, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if !strings.HasPrefix(part, "naming:") {
				continue
			}

			switch strings.TrimPrefix(part, "naming:") {
			case "snake_case":
				return naming.SnakeCase, true
			case "camel_case", "camelCase":
				return naming.CamelCase, true
			case "pascal_case", "pascalCase":
				return naming.PascalCase, true
			case "dynamorm", "legacy_dynamorm", "legacyDynamORM":
				return naming.DynamORM, true
			}
		}
	}

	return naming.CamelCase, false
}

func resolveStructNaming(modelType reflect.Type, inheritedConvention naming.Convention, inheritNaming bool) (naming.Convention, bool) {
	if convention, ok := explicitNamingConvention(modelType); ok {
		return convention, true
	}

	if inheritNaming {
		return inheritedConvention, true
	}

	return detectNamingConvention(modelType), false
}

func fieldHasEncryptedTag(field reflect.StructField) bool {
	tag := field.Tag.Get("theorydb")
	if tag == "" || tag == "-" {
		return false
	}

	parts := strings.Split(tag, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if part == "encrypted" || strings.HasPrefix(part, "encrypted:") {
			return true
		}
	}

	return false
}

func fieldHasJSONTag(field reflect.StructField) bool {
	return fieldcodec.HasJSONModifier(field.Tag.Get("theorydb"))
}

func looksLikeEncryptedEnvelope(av types.AttributeValue) bool {
	env, ok := av.(*types.AttributeValueMemberM)
	if !ok || env == nil || len(env.Value) == 0 {
		return false
	}

	v, ok := env.Value["v"].(*types.AttributeValueMemberN)
	if !ok || v == nil || v.Value == "" {
		return false
	}
	edk, ok := env.Value["edk"].(*types.AttributeValueMemberB)
	if !ok || edk == nil || len(edk.Value) == 0 {
		return false
	}
	nonce, ok := env.Value["nonce"].(*types.AttributeValueMemberB)
	if !ok || nonce == nil || len(nonce.Value) == 0 {
		return false
	}
	ct, ok := env.Value["ct"].(*types.AttributeValueMemberB)
	if !ok || ct == nil {
		return false
	}

	return true
}

// attributeValueToInterface converts a DynamoDB AttributeValue to a Go interface{} value.
func attributeValueToInterface(av types.AttributeValue) (interface{}, error) {
	switch v := av.(type) {
	case *types.AttributeValueMemberS:
		return v.Value, nil
	case *types.AttributeValueMemberN:
		return parseNumberToInterface(v.Value)
	case *types.AttributeValueMemberBOOL:
		return v.Value, nil
	case *types.AttributeValueMemberNULL:
		return nil, nil
	case *types.AttributeValueMemberL:
		return attributeValueListToInterface(v.Value)
	case *types.AttributeValueMemberM:
		return attributeValueMapToInterface(v.Value)
	case *types.AttributeValueMemberSS:
		return v.Value, nil
	case *types.AttributeValueMemberNS:
		return attributeValueNumberSetToFloat64(v.Value)
	case *types.AttributeValueMemberBS:
		return v.Value, nil
	case *types.AttributeValueMemberB:
		return v.Value, nil
	default:
		return nil, fmt.Errorf("unsupported attribute value type: %T", av)
	}
}

func parseNumberToInterface(value string) (interface{}, error) {
	if intVal, err := strconv.ParseInt(value, 10, 64); err == nil {
		return intVal, nil
	}
	if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
		return floatVal, nil
	}
	return nil, fmt.Errorf("invalid number format: %s", value)
}

func attributeValueListToInterface(values []types.AttributeValue) ([]interface{}, error) {
	result := make([]interface{}, len(values))
	for i, item := range values {
		val, err := attributeValueToInterface(item)
		if err != nil {
			return nil, err
		}
		result[i] = val
	}
	return result, nil
}

func attributeValueMapToInterface(values map[string]types.AttributeValue) (map[string]interface{}, error) {
	result := make(map[string]interface{}, len(values))
	for k, val := range values {
		converted, err := attributeValueToInterface(val)
		if err != nil {
			return nil, err
		}
		result[k] = converted
	}
	return result, nil
}

func attributeValueNumberSetToFloat64(values []string) ([]float64, error) {
	result := make([]float64, len(values))
	for i, numStr := range values {
		f, err := strconv.ParseFloat(numStr, 64)
		if err != nil {
			return nil, err
		}
		result[i] = f
	}
	return result, nil
}
