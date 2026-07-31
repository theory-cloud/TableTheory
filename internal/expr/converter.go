package expr

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/theory-cloud/tabletheory/v3/internal/anonymous"
	"github.com/theory-cloud/tabletheory/v3/internal/reflectutil"
	"github.com/theory-cloud/tabletheory/v3/pkg/naming"
)

// Marshaler interface for custom marshaling
type Marshaler interface {
	MarshalDynamoDBAttributeValue() (types.AttributeValue, error)
}

// Unmarshaler interface for custom unmarshaling
type Unmarshaler interface {
	UnmarshalDynamoDBAttributeValue(av types.AttributeValue) error
}

// ConvertToAttributeValue converts a Go value to a DynamoDB AttributeValue
func ConvertToAttributeValue(value any) (types.AttributeValue, error) {
	return ConvertToAttributeValueWithOptions(value, ConvertOptions{})
}

// ConvertToAttributeValueWithOptions converts a Go value to a DynamoDB
// AttributeValue using the provided helper-marshal options.
func ConvertToAttributeValueWithOptions(value any, opts ConvertOptions) (types.AttributeValue, error) {
	return convertToAttributeValueWithConvention(reflect.ValueOf(value), naming.CamelCase, false, opts)
}

func convertToAttributeValueWithConvention(v reflect.Value, inheritedConvention naming.Convention, inheritNaming bool, opts ConvertOptions) (types.AttributeValue, error) {
	prepared, terminalAV, done, err := prepareValueForConversion(v, opts)
	if done || err != nil {
		return terminalAV, err
	}

	if opts.Converter != nil && prepared.CanInterface() && opts.Converter.HasCustomConverter(prepared.Type()) {
		av, err := opts.Converter.ToAttributeValue(prepared.Interface())
		return av, err
	}

	return convertConcreteValueToAttributeValue(prepared, inheritedConvention, inheritNaming, opts)
}

func prepareValueForConversion(v reflect.Value, _ ConvertOptions) (reflect.Value, types.AttributeValue, bool, error) {
	if !v.IsValid() {
		return reflect.Value{}, &types.AttributeValueMemberNULL{Value: true}, true, nil
	}

	for {
		if v.CanInterface() {
			if marshaler, ok := v.Interface().(Marshaler); ok {
				av, err := marshaler.MarshalDynamoDBAttributeValue()
				return reflect.Value{}, av, true, err
			}
		}
		if v.Kind() != reflect.Ptr {
			if marshaler, ok := addressableMarshaler(v); ok {
				av, err := marshaler.MarshalDynamoDBAttributeValue()
				return reflect.Value{}, av, true, err
			}
		}

		if v.Kind() != reflect.Interface && v.Kind() != reflect.Ptr {
			return v, nil, false, nil
		}
		if v.IsNil() {
			return reflect.Value{}, &types.AttributeValueMemberNULL{Value: true}, true, nil
		}
		v = v.Elem()
	}
}

func addressableMarshaler(v reflect.Value) (Marshaler, bool) {
	if !v.IsValid() || v.Kind() == reflect.Ptr {
		return nil, false
	}

	if v.CanAddr() {
		marshaler, ok := v.Addr().Interface().(Marshaler)
		return marshaler, ok
	}

	copyValue := reflect.New(v.Type()).Elem()
	copyValue.Set(v)
	marshaler, ok := copyValue.Addr().Interface().(Marshaler)
	return marshaler, ok
}

func convertConcreteValueToAttributeValue(v reflect.Value, inheritedConvention naming.Convention, inheritNaming bool, opts ConvertOptions) (types.AttributeValue, error) {
	switch v.Kind() {
	case reflect.String:
		return &types.AttributeValueMemberS{Value: v.String()}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", v.Int())}, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", v.Uint())}, nil
	case reflect.Float32, reflect.Float64:
		if opts.FixedFloatFormat {
			return &types.AttributeValueMemberN{Value: strconv.FormatFloat(v.Float(), 'f', -1, 64)}, nil
		}
		return &types.AttributeValueMemberN{Value: fmt.Sprintf("%g", v.Float())}, nil
	case reflect.Bool:
		return &types.AttributeValueMemberBOOL{Value: v.Bool()}, nil
	case reflect.Slice:
		return convertSliceToAttributeValueWithConvention(v, inheritedConvention, inheritNaming, opts)
	case reflect.Array:
		return convertArrayToAttributeValueWithConvention(v, inheritedConvention, inheritNaming, opts)
	case reflect.Map:
		return convertMapToAttributeValueWithConvention(v, inheritedConvention, inheritNaming, opts)
	case reflect.Struct:
		if t, ok := v.Interface().(time.Time); ok {
			return &types.AttributeValueMemberS{Value: t.Format(time.RFC3339Nano)}, nil
		}
		return convertStructToAttributeValueWithConvention(v, inheritedConvention, inheritNaming, opts)
	default:
		return nil, fmt.Errorf("unsupported type: %v", v.Type())
	}
}

func convertArrayToAttributeValueWithConvention(v reflect.Value, inheritedConvention naming.Convention, inheritNaming bool, opts ConvertOptions) (types.AttributeValue, error) {
	list := make([]types.AttributeValue, v.Len())
	for i := 0; i < v.Len(); i++ {
		item, err := convertToAttributeValueWithConvention(v.Index(i), inheritedConvention, inheritNaming, opts)
		if err != nil {
			return nil, err
		}
		list[i] = item
	}
	return &types.AttributeValueMemberL{Value: list}, nil
}

func convertSliceToAttributeValueWithConvention(v reflect.Value, inheritedConvention naming.Convention, inheritNaming bool, opts ConvertOptions) (types.AttributeValue, error) {
	// Handle []byte as binary
	if v.Type().Elem().Kind() == reflect.Uint8 {
		return &types.AttributeValueMemberB{Value: v.Bytes()}, nil
	}

	// Handle other slices as lists
	list := make([]types.AttributeValue, v.Len())
	for i := 0; i < v.Len(); i++ {
		item, err := convertToAttributeValueWithConvention(v.Index(i), inheritedConvention, inheritNaming, opts)
		if err != nil {
			return nil, err
		}
		list[i] = item
	}
	return &types.AttributeValueMemberL{Value: list}, nil
}

func convertMapToAttributeValueWithConvention(v reflect.Value, inheritedConvention naming.Convention, inheritNaming bool, opts ConvertOptions) (types.AttributeValue, error) {
	// Handle map[string]any as M type
	if v.Type().Key().Kind() != reflect.String {
		if opts.LegacyStructFieldNames {
			return nil, fmt.Errorf("map keys must be strings")
		}
		return nil, fmt.Errorf("unsupported map type: %v", v.Type())
	}

	m := make(map[string]types.AttributeValue, v.Len())
	for _, key := range v.MapKeys() {
		val, err := convertToAttributeValueWithConvention(v.MapIndex(key), inheritedConvention, inheritNaming, opts)
		if err != nil {
			return nil, err
		}
		m[key.String()] = val
	}
	return &types.AttributeValueMemberM{Value: m}, nil
}

func convertStructToAttributeValueWithConvention(v reflect.Value, inheritedConvention naming.Convention, inheritNaming bool, opts ConvertOptions) (types.AttributeValue, error) {
	// General struct marshaling.
	// Explicit theorydb:"json" field handling is applied by higher-level field codecs
	// before values reach this generic converter.
	m := make(map[string]types.AttributeValue)
	t := v.Type()
	convention, useTheorydbNaming := resolveStructNaming(t, inheritedConvention, inheritNaming)

	fieldPlans, err := BuildMarshalVisibleFieldPlan(t, nil, true)
	if err != nil {
		return nil, err
	}

	for _, fieldPlan := range fieldPlans {
		fieldName, theorydbTag, jsonTag, ok := marshalFieldNameAndTags(fieldPlan.Field, convention, useTheorydbNaming, opts)
		if !ok {
			continue
		}

		fieldValue := v.FieldByIndex(fieldPlan.IndexPath)
		if shouldOmitEmptyField(fieldValue, theorydbTag, jsonTag) ||
			(opts.OmitZeroFieldsByDefault && fieldValue.IsZero()) {
			continue
		}

		av, err := convertToAttributeValueWithConvention(fieldValue, convention, true, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to convert field %s: %w", fieldPlan.Field.Name, err)
		}

		containerNames, skip := anonymous.MarshalContainerNamesForField(t, fieldPlan.IndexPath, func(field reflect.StructField) (string, bool) {
			name, _, _, ok := marshalFieldNameAndTags(field, convention, true, opts)
			return name, !ok
		}, convertOptionsRequestFlatAnonymousEmbeds(opts))
		if skip {
			continue
		}
		if err := anonymous.SetMarshaledAttributeValue(m, containerNames, fieldName, av, convertOptionsRequestFlatAnonymousEmbeds(opts)); err != nil {
			return nil, fmt.Errorf("failed to convert field %s: %w", fieldPlan.Field.Name, err)
		}
	}

	return &types.AttributeValueMemberM{Value: m}, nil
}

func marshalFieldNameAndTags(field reflect.StructField, convention naming.Convention, useTheorydbNaming bool, opts ConvertOptions) (string, string, string, bool) {
	theorydbTag := field.Tag.Get("theorydb")
	jsonTag := field.Tag.Get("json")
	if theorydbTag == "-" || jsonTag == "-" {
		return "", "", "", false
	}

	if theorydbTag != "" {
		fieldName := fieldNameFromTheorydbTag(naming.ConvertAttrName(field.Name, convention), theorydbTag, convention)
		return fieldName, theorydbTag, jsonTag, true
	}

	if opts.LegacyStructFieldNames && !useTheorydbNaming {
		return field.Name, theorydbTag, jsonTag, true
	}

	fieldName := naming.ConvertAttrName(field.Name, convention)
	if jsonTag != "" {
		fieldName = fieldNameFromJSONTag(fieldName, jsonTag)
	}

	return fieldName, theorydbTag, jsonTag, true
}

func fieldNameFromTheorydbTag(defaultName string, tag string, convention naming.Convention) string {
	if convention == naming.DynamORM {
		if hasStandaloneTagPart(tag, "pk") {
			return "PK"
		}
		if hasStandaloneTagPart(tag, "sk") {
			return "SK"
		}
	}

	fieldName := defaultName
	parts := strings.Split(tag, ",")
	if len(parts) > 0 && parts[0] != "" {
		firstPart := strings.TrimSpace(parts[0])
		if !strings.Contains(firstPart, ":") && !isPureModifierTag(firstPart) {
			fieldName = firstPart
		}
	}

	if attrName := parseAttrTag(tag); attrName != "" {
		fieldName = attrName
	}

	return fieldName
}

func fieldNameFromJSONTag(defaultName string, jsonTag string) string {
	parts := strings.Split(jsonTag, ",")
	if len(parts) > 0 && parts[0] != "" {
		return parts[0]
	}
	return defaultName
}

func shouldOmitEmptyField(fieldValue reflect.Value, theorydbTag string, jsonTag string) bool {
	if !hasOmitEmpty(theorydbTag) && !hasStandaloneTagPart(jsonTag, "omitempty") {
		return false
	}
	return isZeroValue(fieldValue)
}

// ConvertFromAttributeValue converts a DynamoDB AttributeValue to a Go value
func ConvertFromAttributeValue(av types.AttributeValue, target any) error {
	targetValue := reflect.ValueOf(target)
	if targetValue.Kind() != reflect.Ptr || targetValue.IsNil() {
		return fmt.Errorf("target must be a non-nil pointer")
	}

	// Check for custom unmarshaler
	if unmarshaler, ok := target.(Unmarshaler); ok {
		return unmarshaler.UnmarshalDynamoDBAttributeValue(av)
	}

	targetElem := targetValue.Elem()
	return unmarshalAttributeValueWithConvention(av, targetElem, naming.CamelCase, false)
}

func unmarshalAttributeValueWithConvention(av types.AttributeValue, v reflect.Value, inheritedConvention naming.Convention, inheritNaming bool) error {
	if isEmptyInterfaceValue(v) {
		return unmarshalIntoEmptyInterface(av, v)
	}
	if v.Kind() == reflect.Ptr {
		return unmarshalIntoPointerWithConvention(av, v, inheritedConvention, inheritNaming)
	}
	return unmarshalAttributeValueNonPtrWithConvention(av, v, inheritedConvention, inheritNaming)
}

func isEmptyInterfaceValue(v reflect.Value) bool {
	return v.Kind() == reflect.Interface && v.Type().NumMethod() == 0
}

func unmarshalIntoEmptyInterface(av types.AttributeValue, v reflect.Value) error {
	val, err := attributeValueToInterface(av)
	if err != nil {
		return err
	}
	v.Set(reflect.ValueOf(val))
	return nil
}

func unmarshalIntoPointerWithConvention(av types.AttributeValue, v reflect.Value, inheritedConvention naming.Convention, inheritNaming bool) error {
	if av == nil || isNullAttributeValue(av) {
		v.Set(reflect.Zero(v.Type()))
		return nil
	}

	// Create new value if pointer is nil
	if v.IsNil() {
		v.Set(reflect.New(v.Type().Elem()))
	}
	return unmarshalAttributeValueWithConvention(av, v.Elem(), inheritedConvention, inheritNaming)
}

func unmarshalAttributeValueNonPtrWithConvention(av types.AttributeValue, v reflect.Value, inheritedConvention naming.Convention, inheritNaming bool) error {
	switch av := av.(type) {
	case *types.AttributeValueMemberS:
		return unmarshalString(av.Value, v)

	case *types.AttributeValueMemberN:
		return unmarshalNumber(av.Value, v)

	case *types.AttributeValueMemberB:
		return unmarshalBinary(av.Value, v)

	case *types.AttributeValueMemberBOOL:
		return unmarshalBool(av.Value, v)

	case *types.AttributeValueMemberNULL:
		v.Set(reflect.Zero(v.Type()))
		return nil

	case *types.AttributeValueMemberL:
		return unmarshalListWithConvention(av.Value, v, inheritedConvention, inheritNaming)

	case *types.AttributeValueMemberM:
		return unmarshalMapWithConvention(av.Value, v, inheritedConvention, inheritNaming)

	case *types.AttributeValueMemberSS:
		return unmarshalStringSet(av.Value, v)

	case *types.AttributeValueMemberNS:
		return unmarshalNumberSet(av.Value, v)

	case *types.AttributeValueMemberBS:
		return unmarshalBinarySet(av.Value, v)

	default:
		return fmt.Errorf("unknown AttributeValue type: %T", av)
	}
}

// unmarshalString unmarshals a string value
func unmarshalString(s string, v reflect.Value) error {
	switch v.Kind() {
	case reflect.String:
		v.SetString(s)
		return nil

	case reflect.Struct:
		// Special handling for time.Time
		if v.Type() == reflect.TypeOf(time.Time{}) {
			t, err := time.Parse(time.RFC3339Nano, s)
			if err != nil {
				// Try other common formats
				t, err = time.Parse(time.RFC3339, s)
				if err != nil {
					return fmt.Errorf("failed to parse time: %w", err)
				}
			}
			v.Set(reflect.ValueOf(t))
			return nil
		}

		return fmt.Errorf("cannot unmarshal string into %v", v.Type())

	default:
		return fmt.Errorf("cannot unmarshal string into %v", v.Type())
	}
}

// unmarshalNumber unmarshals a number value
func unmarshalNumber(n string, v reflect.Value) error {
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i, err := strconv.ParseInt(n, 10, 64)
		if err != nil {
			return err
		}
		v.SetInt(i)
		return nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		u, err := strconv.ParseUint(n, 10, 64)
		if err != nil {
			return err
		}
		v.SetUint(u)
		return nil

	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(n, 64)
		if err != nil {
			return err
		}
		v.SetFloat(f)
		return nil

	default:
		return fmt.Errorf("cannot unmarshal number into %v", v.Type())
	}
}

// unmarshalBinary unmarshals binary data
func unmarshalBinary(b []byte, v reflect.Value) error {
	if v.Kind() == reflect.Slice && v.Type().Elem().Kind() == reflect.Uint8 {
		v.SetBytes(b)
		return nil
	}
	return fmt.Errorf("cannot unmarshal binary into %v", v.Type())
}

// unmarshalBool unmarshals a boolean value
func unmarshalBool(b bool, v reflect.Value) error {
	if v.Kind() == reflect.Bool {
		v.SetBool(b)
		return nil
	}
	return fmt.Errorf("cannot unmarshal bool into %v", v.Type())
}

// unmarshalList unmarshals a list of AttributeValues
func unmarshalList(list []types.AttributeValue, v reflect.Value) error {
	return unmarshalListWithConvention(list, v, naming.CamelCase, false)
}

func unmarshalListWithConvention(list []types.AttributeValue, v reflect.Value, inheritedConvention naming.Convention, inheritNaming bool) error {
	if v.Kind() != reflect.Slice {
		return fmt.Errorf("cannot unmarshal list into %v", v.Type())
	}

	// Create new slice
	slice := reflect.MakeSlice(v.Type(), len(list), len(list))

	// Unmarshal each element
	for i, item := range list {
		if err := unmarshalAttributeValueWithConvention(item, slice.Index(i), inheritedConvention, inheritNaming); err != nil {
			return fmt.Errorf("failed to unmarshal list item %d: %w", i, err)
		}
	}

	v.Set(slice)
	return nil
}

// unmarshalMap unmarshals a map of AttributeValues
func unmarshalMap(m map[string]types.AttributeValue, v reflect.Value) error {
	return unmarshalMapWithConvention(m, v, naming.CamelCase, false)
}

func unmarshalMapWithConvention(m map[string]types.AttributeValue, v reflect.Value, inheritedConvention naming.Convention, inheritNaming bool) error {
	switch v.Kind() {
	case reflect.Map:
		return unmarshalMapIntoMapWithConvention(m, v, inheritedConvention, inheritNaming)

	case reflect.Struct:
		return unmarshalMapIntoStructWithConvention(m, v, inheritedConvention, inheritNaming)

	default:
		return fmt.Errorf("cannot unmarshal map into %v", v.Type())
	}
}

func unmarshalMapIntoMap(m map[string]types.AttributeValue, v reflect.Value) error {
	return unmarshalMapIntoMapWithConvention(m, v, naming.CamelCase, false)
}

func unmarshalMapIntoMapWithConvention(m map[string]types.AttributeValue, v reflect.Value, inheritedConvention naming.Convention, inheritNaming bool) error {
	// Ensure map is string-keyed
	if v.Type().Key().Kind() != reflect.String {
		return fmt.Errorf("map must have string keys")
	}

	// Create new map if nil
	if v.IsNil() {
		v.Set(reflect.MakeMap(v.Type()))
	}

	// Unmarshal each value
	for key, value := range m {
		mapValue := reflect.New(v.Type().Elem()).Elem()
		if err := unmarshalAttributeValueWithConvention(value, mapValue, inheritedConvention, inheritNaming); err != nil {
			return fmt.Errorf("failed to unmarshal map value for key %s: %w", key, err)
		}
		v.SetMapIndex(reflect.ValueOf(key), mapValue)
	}
	return nil
}

func unmarshalMapIntoStructWithConvention(m map[string]types.AttributeValue, v reflect.Value, inheritedConvention naming.Convention, inheritNaming bool) error {
	t := v.Type()
	convention, _ := resolveStructNaming(t, inheritedConvention, inheritNaming)

	fieldPlans, err := buildExprUnmarshalFieldPlans(t, convention)
	if err != nil {
		return err
	}

	for _, fieldPlan := range fieldPlans {
		fieldNames, skip := unmarshalFieldNames(fieldPlan.Field, convention)
		if skip {
			continue
		}

		av, ok, err := lookupExprFieldPlanValue(m, fieldPlan, fieldNames)
		if err != nil {
			return fmt.Errorf("failed to unmarshal field %s: %w", fieldPlan.Field.Name, err)
		}
		if !ok {
			continue
		}
		if err := unmarshalAttributeValueWithConvention(av, v.FieldByIndex(fieldPlan.IndexPath), convention, true); err != nil {
			return fmt.Errorf("failed to unmarshal field %s: %w", fieldPlan.Field.Name, err)
		}
	}
	return nil
}

func buildExprUnmarshalFieldPlans(targetType reflect.Type, convention naming.Convention) ([]reflectutil.VisibleFieldPlan, error) {
	return reflectutil.BuildVisibleFieldPlan(targetType, func(field reflect.StructField) ([]string, bool, error) {
		names, skip := unmarshalFieldNames(field, convention)
		return names, skip, nil
	})
}

func lookupExprFieldPlanValue(values map[string]types.AttributeValue, fieldPlan reflectutil.VisibleFieldPlan, names []string) (types.AttributeValue, bool, error) {
	if value, ok := lookupMapValue(values, names...); ok {
		return value, true, nil
	}

	return lookupLegacyExprFieldPlanValue(values, fieldPlan.LegacyContainers, names)
}

func lookupLegacyExprFieldPlanValue(values map[string]types.AttributeValue, containers []reflectutil.LegacyContainerPlan, names []string) (types.AttributeValue, bool, error) {
	if len(containers) == 0 {
		return nil, false, nil
	}

	current := values
	for _, container := range containers {
		if len(container.Aliases) == 0 {
			return nil, false, nil
		}

		av, ok := lookupMapValue(current, container.Aliases...)
		if !ok {
			return nil, false, nil
		}

		nested, ok := av.(*types.AttributeValueMemberM)
		if !ok {
			return nil, false, fmt.Errorf("legacy anonymous container %s must decode from a map, got %T", container.Field.Name, av)
		}

		current = nested.Value
	}

	av, ok := lookupMapValue(current, names...)
	return av, ok, nil
}

func unmarshalFieldNames(field reflect.StructField, convention naming.Convention) ([]string, bool) {
	theorydbTag := field.Tag.Get("theorydb")
	jsonTag := field.Tag.Get("json")
	if theorydbTag == "-" || jsonTag == "-" {
		return nil, true
	}

	names := make([]string, 0, 4)
	if theorydbTag != "" {
		fieldName := fieldNameFromTheorydbTag(naming.ConvertAttrName(field.Name, convention), theorydbTag, convention)
		if fieldName == "" {
			return nil, true
		}
		names = appendUniqueLookupName(names, fieldName)
	}

	if jsonName := jsonTagName(jsonTag); jsonName != "" {
		names = appendUniqueLookupName(names, jsonName)
	}

	names = appendUniqueLookupName(names, naming.ConvertAttrName(field.Name, convention))
	names = appendUniqueLookupName(names, field.Name)
	if len(names) == 0 {
		return nil, true
	}

	return names, false
}

// unmarshalStringSet unmarshals a string set
func unmarshalStringSet(ss []string, v reflect.Value) error {
	if v.Kind() != reflect.Slice || v.Type().Elem().Kind() != reflect.String {
		return fmt.Errorf("cannot unmarshal string set into %v", v.Type())
	}

	slice := reflect.MakeSlice(v.Type(), len(ss), len(ss))
	for i, s := range ss {
		slice.Index(i).SetString(s)
	}
	v.Set(slice)
	return nil
}

// unmarshalNumberSet unmarshals a number set
func unmarshalNumberSet(ns []string, v reflect.Value) error {
	if v.Kind() != reflect.Slice {
		return fmt.Errorf("cannot unmarshal number set into %v", v.Type())
	}

	slice := reflect.MakeSlice(v.Type(), len(ns), len(ns))
	for i, n := range ns {
		if err := unmarshalNumber(n, slice.Index(i)); err != nil {
			return fmt.Errorf("failed to unmarshal number set item %d: %w", i, err)
		}
	}
	v.Set(slice)
	return nil
}

// unmarshalBinarySet unmarshals a binary set
func unmarshalBinarySet(bs [][]byte, v reflect.Value) error {
	if v.Kind() != reflect.Slice || v.Type().Elem().Kind() != reflect.Slice {
		return fmt.Errorf("cannot unmarshal binary set into %v", v.Type())
	}

	slice := reflect.MakeSlice(v.Type(), len(bs), len(bs))
	for i, b := range bs {
		slice.Index(i).SetBytes(b)
	}
	v.Set(slice)
	return nil
}

// Helper functions

func isNullAttributeValue(av types.AttributeValue) bool {
	if nullAV, ok := av.(*types.AttributeValueMemberNULL); ok {
		return nullAV.Value
	}
	return false
}

func parseAttrTag(tag string) string {
	parts := strings.Split(tag, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "attr:") {
			return strings.TrimPrefix(part, "attr:")
		}
	}
	return ""
}

func hasStandaloneTagPart(tag string, want string) bool {
	if tag == "" || want == "" {
		return false
	}

	parts := strings.Split(tag, ",")
	for _, part := range parts {
		if strings.TrimSpace(part) == want {
			return true
		}
	}
	return false
}

func detectNamingConvention(modelType reflect.Type) naming.Convention {
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

func jsonTagName(tag string) string {
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

func appendUniqueLookupName(names []string, name string) []string {
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

func lookupMapValue(values map[string]types.AttributeValue, names ...string) (types.AttributeValue, bool) {
	for _, name := range names {
		value, ok := values[name]
		if ok {
			return value, true
		}
	}
	return nil, false
}

func hasOmitEmpty(tag string) bool {
	return hasStandaloneTagPart(tag, "omitempty")
}

func isZeroValue(v reflect.Value) bool {
	return reflectutil.IsEmpty(v)
}

func isPureModifierTag(tag string) bool {
	modifiers := []string{"pk", "sk", "version", "ttl", "set", "omitempty", "binary", "json", "encrypted"}
	for _, mod := range modifiers {
		if tag == mod {
			return true
		}
	}
	return false
}

// attributeValueToInterface converts an AttributeValue to a native Go type
func attributeValueToInterface(av types.AttributeValue) (any, error) {
	switch av := av.(type) {
	case *types.AttributeValueMemberS:
		return av.Value, nil

	case *types.AttributeValueMemberN:
		return parseNumberString(av.Value)

	case *types.AttributeValueMemberB:
		return av.Value, nil

	case *types.AttributeValueMemberBOOL:
		return av.Value, nil

	case *types.AttributeValueMemberNULL:
		return nil, nil

	case *types.AttributeValueMemberL:
		return attributeValueListToInterface(av.Value)

	case *types.AttributeValueMemberM:
		return attributeValueMapToInterface(av.Value)

	case *types.AttributeValueMemberSS:
		return av.Value, nil

	case *types.AttributeValueMemberNS:
		return attributeValueNumberSetToInterface(av.Value)

	case *types.AttributeValueMemberBS:
		// Convert binary set to slice of []byte
		return av.Value, nil

	default:
		return nil, fmt.Errorf("unknown AttributeValue type: %T", av)
	}
}

func parseNumberString(value string) (any, error) {
	if i, err := strconv.ParseInt(value, 10, 64); err == nil {
		return i, nil
	}
	if f, err := strconv.ParseFloat(value, 64); err == nil {
		return f, nil
	}
	return nil, fmt.Errorf("cannot parse number: %s", value)
}

func attributeValueListToInterface(list []types.AttributeValue) ([]any, error) {
	result := make([]any, len(list))
	for i, item := range list {
		val, err := attributeValueToInterface(item)
		if err != nil {
			return nil, fmt.Errorf("failed to convert list item %d: %w", i, err)
		}
		result[i] = val
	}
	return result, nil
}

func attributeValueMapToInterface(m map[string]types.AttributeValue) (map[string]any, error) {
	result := make(map[string]any, len(m))
	for k, v := range m {
		val, err := attributeValueToInterface(v)
		if err != nil {
			return nil, fmt.Errorf("failed to convert map value for key %s: %w", k, err)
		}
		result[k] = val
	}
	return result, nil
}

func attributeValueNumberSetToInterface(values []string) ([]any, error) {
	nums := make([]any, len(values))
	for i, value := range values {
		num, err := parseNumberString(value)
		if err != nil {
			return nil, fmt.Errorf("cannot parse number in set: %s", value)
		}
		nums[i] = num
	}
	return nums, nil
}
