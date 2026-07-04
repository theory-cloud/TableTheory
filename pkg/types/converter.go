// Package types provides type conversion between Go types and DynamoDB AttributeValues
package types

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/theory-cloud/tabletheory/internal/expr"
	"github.com/theory-cloud/tabletheory/internal/reflectutil"
	"github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/naming"
)

// Converter handles conversion between Go types and DynamoDB AttributeValues
type Converter struct {
	// customConverters allows registration of custom type converters
	customConverters           map[reflect.Type]CustomConverter
	flatAnonymousEmbedEncoding bool
	mu                         sync.RWMutex
}

var timeType = reflect.TypeOf(time.Time{})

// CustomConverter defines the interface for custom type converters
type CustomConverter interface {
	// ToAttributeValue converts a Go value to DynamoDB AttributeValue
	ToAttributeValue(value any) (types.AttributeValue, error)

	// FromAttributeValue converts a DynamoDB AttributeValue to Go value
	FromAttributeValue(av types.AttributeValue, target any) error
}

type customConverterAdapter struct {
	converter *Converter
}

func (a customConverterAdapter) HasCustomConverter(typ reflect.Type) bool {
	return a.converter != nil && a.converter.HasCustomConverter(typ)
}

func (a customConverterAdapter) ToAttributeValue(value any) (types.AttributeValue, error) {
	if a.converter == nil || value == nil {
		return &types.AttributeValueMemberNULL{Value: true}, nil
	}
	v, isNull := indirectValueOrNull(reflect.ValueOf(value))
	if isNull {
		return &types.AttributeValueMemberNULL{Value: true}, nil
	}
	custom, ok := a.converter.lookupConverter(v.Type())
	if !ok {
		return expr.ConvertToAttributeValueWithOptions(value, a.converter.convertOptions())
	}
	return custom.ToAttributeValue(v.Interface())
}

// NewConverter creates a new type converter
func NewConverter() *Converter {
	return &Converter{
		customConverters: make(map[reflect.Type]CustomConverter),
	}
}

// RegisterConverter registers a custom converter for a specific type
func (c *Converter) RegisterConverter(typ reflect.Type, converter CustomConverter) {
	if typ == nil || converter == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.customConverters[typ] = converter
}

// HasCustomConverter returns true if a custom converter exists for the given type.
func (c *Converter) HasCustomConverter(typ reflect.Type) bool {
	_, ok := c.lookupConverter(typ)
	return ok
}

// lookupConverter returns a registered converter for the provided type, walking pointer
// indirections until a match is found or no further pointer element exists.
func (c *Converter) lookupConverter(typ reflect.Type) (CustomConverter, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if typ == nil {
		return nil, false
	}

	for {
		if converter, ok := c.customConverters[typ]; ok {
			return converter, true
		}

		if typ.Kind() != reflect.Ptr {
			break
		}
		typ = typ.Elem()
	}

	return nil, false
}

func (c *Converter) convertOptions() expr.ConvertOptions {
	opts := expr.ConvertOptions{
		Converter:                  customConverterAdapter{converter: c},
		LegacyStructFieldNames:     true,
		OmitZeroFieldsByDefault:    true,
		FixedFloatFormat:           true,
		FlatAnonymousEmbedEncoding: c.FlatAnonymousEmbedEncodingEnabled(),
	}
	return opts
}

// ToAttributeValue converts a Go value to DynamoDB AttributeValue
func (c *Converter) ToAttributeValue(value any) (types.AttributeValue, error) {
	if c == nil {
		c = NewConverter()
	}
	return expr.ConvertToAttributeValueWithOptions(value, c.convertOptions())
}

func (c *Converter) toAttributeValueWithConvention(v reflect.Value, inheritedConvention naming.Convention, inheritNaming bool) (types.AttributeValue, error) {
	indirect, isNull := indirectValueOrNull(v)
	if isNull {
		return &types.AttributeValueMemberNULL{Value: true}, nil
	}

	if av, handled, err := c.specialAttributeValue(indirect); handled {
		return av, err
	}

	return c.attributeValueByKind(indirect, inheritedConvention, inheritNaming)
}

func indirectValueOrNull(v reflect.Value) (reflect.Value, bool) {
	for {
		if !v.IsValid() {
			return reflect.Value{}, true
		}

		if v.Kind() != reflect.Ptr && v.Kind() != reflect.Interface {
			return v, false
		}
		if v.IsNil() {
			return reflect.Value{}, true
		}
		v = v.Elem()
	}
}

func (c *Converter) specialAttributeValue(v reflect.Value) (types.AttributeValue, bool, error) {
	if converter, exists := c.lookupConverter(v.Type()); exists {
		av, err := converter.ToAttributeValue(v.Interface())
		return av, true, err
	}

	if v.Type() != timeType {
		return nil, false, nil
	}

	t, ok := v.Interface().(time.Time)
	if !ok {
		return nil, true, fmt.Errorf("expected time.Time, got %T", v.Interface())
	}

	return &types.AttributeValueMemberS{Value: t.Format(time.RFC3339Nano)}, true, nil
}

func (c *Converter) attributeValueByKind(v reflect.Value, inheritedConvention naming.Convention, inheritNaming bool) (types.AttributeValue, error) {
	switch v.Kind() {
	case reflect.String:
		return &types.AttributeValueMemberS{Value: v.String()}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &types.AttributeValueMemberN{Value: strconv.FormatInt(v.Int(), 10)}, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &types.AttributeValueMemberN{Value: strconv.FormatUint(v.Uint(), 10)}, nil
	case reflect.Float32, reflect.Float64:
		return &types.AttributeValueMemberN{Value: strconv.FormatFloat(v.Float(), 'f', -1, 64)}, nil
	case reflect.Bool:
		return &types.AttributeValueMemberBOOL{Value: v.Bool()}, nil
	case reflect.Slice:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return &types.AttributeValueMemberB{Value: v.Bytes()}, nil
		}
		return c.sliceToListWithConvention(v, inheritedConvention, inheritNaming)
	case reflect.Map:
		return c.mapToAttributeValueMapWithConvention(v, inheritedConvention, inheritNaming)
	case reflect.Struct:
		return c.structToMapWithConvention(v, inheritedConvention, inheritNaming)
	default:
		return nil, fmt.Errorf("%w: %s", errors.ErrUnsupportedType, v.Type())
	}
}

func (c *Converter) sliceToListWithConvention(v reflect.Value, inheritedConvention naming.Convention, inheritNaming bool) (types.AttributeValue, error) {
	list := make([]types.AttributeValue, v.Len())

	for i := 0; i < v.Len(); i++ {
		av, err := c.toAttributeValueWithConvention(v.Index(i), inheritedConvention, inheritNaming)
		if err != nil {
			return nil, fmt.Errorf("index %d: %w", i, err)
		}
		list[i] = av
	}

	return &types.AttributeValueMemberL{Value: list}, nil
}

func (c *Converter) mapToAttributeValueMapWithConvention(v reflect.Value, inheritedConvention naming.Convention, inheritNaming bool) (types.AttributeValue, error) {
	if v.Type().Key().Kind() != reflect.String {
		return nil, fmt.Errorf("%w: map keys must be strings", errors.ErrUnsupportedType)
	}

	m := make(map[string]types.AttributeValue)

	for _, key := range v.MapKeys() {
		keyStr := key.String()
		val := v.MapIndex(key)

		av, err := c.toAttributeValueWithConvention(val, inheritedConvention, inheritNaming)
		if err != nil {
			return nil, fmt.Errorf("key %s: %w", keyStr, err)
		}
		m[keyStr] = av
	}

	return &types.AttributeValueMemberM{Value: m}, nil
}

func (c *Converter) structToMapWithConvention(v reflect.Value, inheritedConvention naming.Convention, inheritNaming bool) (types.AttributeValue, error) {
	if c.FlatAnonymousEmbedEncodingEnabled() {
		return c.structToFlatMapWithConvention(v, inheritedConvention, inheritNaming)
	}

	return c.structToLegacyMapWithConvention(v, inheritedConvention, inheritNaming)
}

func (c *Converter) structToLegacyMapWithConvention(v reflect.Value, inheritedConvention naming.Convention, inheritNaming bool) (types.AttributeValue, error) {
	m := make(map[string]types.AttributeValue)
	t := v.Type()
	convention, useTheorydbNaming := resolveStructNaming(t, inheritedConvention, inheritNaming)

	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		fieldValue := v.Field(i)
		if fieldValue.IsZero() {
			continue // Skip zero values for now
		}

		av, err := c.toAttributeValueWithConvention(fieldValue, convention, useTheorydbNaming)
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", field.Name, err)
		}

		attrName := field.Name
		if useTheorydbNaming {
			var skip bool
			attrName, skip = naming.ResolveAttrNameWithConvention(field, convention)
			if skip {
				continue
			}
			if err := naming.ValidateAttrName(attrName, convention); err != nil {
				return nil, fmt.Errorf("field %s: %w", field.Name, err)
			}
		}

		m[attrName] = av
	}

	return &types.AttributeValueMemberM{Value: m}, nil
}

func (c *Converter) structToFlatMapWithConvention(v reflect.Value, inheritedConvention naming.Convention, inheritNaming bool) (types.AttributeValue, error) {
	m := make(map[string]types.AttributeValue)
	t := v.Type()
	convention, useTheorydbNaming := resolveStructNaming(t, inheritedConvention, inheritNaming)

	fieldPlans, err := reflectutil.BuildVisibleFieldPlan(t, nil)
	if err != nil {
		return nil, err
	}

	for _, fieldPlan := range fieldPlans {
		fieldValue := v.FieldByIndex(fieldPlan.IndexPath)
		if fieldValue.IsZero() {
			continue
		}

		av, err := c.toAttributeValueWithConvention(fieldValue, convention, useTheorydbNaming)
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", fieldPlan.Field.Name, err)
		}

		attrName, skip, err := resolveStructMarshalFieldName(fieldPlan.Field, convention, useTheorydbNaming)
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", fieldPlan.Field.Name, err)
		}
		if skip {
			continue
		}

		m[attrName] = av
	}

	return &types.AttributeValueMemberM{Value: m}, nil
}

func resolveStructMarshalFieldName(field reflect.StructField, convention naming.Convention, useTheorydbNaming bool) (string, bool, error) {
	if !useTheorydbNaming {
		return field.Name, false, nil
	}

	attrName, skip := naming.ResolveAttrNameWithConvention(field, convention)
	if skip {
		return "", true, nil
	}
	if err := naming.ValidateAttrName(attrName, convention); err != nil {
		return "", false, err
	}
	return attrName, false, nil
}

// FromAttributeValue converts a DynamoDB AttributeValue to Go value
func (c *Converter) FromAttributeValue(av types.AttributeValue, target any) error {
	targetValue := reflect.ValueOf(target)
	if targetValue.Kind() != reflect.Ptr {
		return fmt.Errorf("target must be a pointer")
	}
	if targetValue.IsNil() {
		return fmt.Errorf("target pointer is nil")
	}

	return c.fromAttributeValueWithConvention(av, targetValue.Elem(), naming.CamelCase, true)
}

func (c *Converter) fromAttributeValueWithConvention(av types.AttributeValue, target reflect.Value, inheritedConvention naming.Convention, inheritNaming bool) error {
	if _, ok := av.(*types.AttributeValueMemberNULL); ok {
		return nil
	}

	if !target.CanSet() {
		return fmt.Errorf("target is not settable")
	}

	target = ensureSettableConcreteTarget(target)

	if target.Kind() == reflect.Interface && target.Type().NumMethod() == 0 {
		return c.anyFromAttributeValue(av, target)
	}

	if converter, exists := c.lookupConverter(target.Type()); exists {
		return converter.FromAttributeValue(av, target.Addr().Interface())
	}

	if target.Type() == timeType {
		return c.fromAttributeValueTime(av, target)
	}

	return c.fromAttributeValueByType(av, target, inheritedConvention, inheritNaming)
}

func (c *Converter) anyFromAttributeValue(av types.AttributeValue, target reflect.Value) error {
	value, err := attributeValueToAny(av)
	if err != nil {
		return err
	}
	if value == nil {
		target.Set(reflect.Zero(target.Type()))
		return nil
	}
	target.Set(reflect.ValueOf(value))
	return nil
}

func ensureSettableConcreteTarget(target reflect.Value) reflect.Value {
	if target.Kind() != reflect.Ptr {
		return target
	}

	if target.IsNil() {
		target.Set(reflect.New(target.Type().Elem()))
	}

	return target.Elem()
}

func (c *Converter) fromAttributeValueTime(av types.AttributeValue, target reflect.Value) error {
	s, ok := av.(*types.AttributeValueMemberS)
	if !ok {
		return fmt.Errorf("expected string for time.Time, got %T", av)
	}

	t, err := time.Parse(time.RFC3339Nano, s.Value)
	if err != nil {
		return fmt.Errorf("invalid time format: %w", err)
	}

	target.Set(reflect.ValueOf(t))
	return nil
}

func (c *Converter) fromAttributeValueByType(av types.AttributeValue, target reflect.Value, inheritedConvention naming.Convention, inheritNaming bool) error {
	switch v := av.(type) {
	case *types.AttributeValueMemberS:
		return c.stringToValue(v.Value, target)
	case *types.AttributeValueMemberN:
		return c.numberToValue(v.Value, target)
	case *types.AttributeValueMemberBOOL:
		if target.Kind() != reflect.Bool {
			return fmt.Errorf("cannot convert bool to %s", target.Type())
		}
		target.SetBool(v.Value)
		return nil
	case *types.AttributeValueMemberB:
		if target.Kind() != reflect.Slice || target.Type().Elem().Kind() != reflect.Uint8 {
			return fmt.Errorf("cannot convert binary to %s", target.Type())
		}
		target.SetBytes(v.Value)
		return nil
	case *types.AttributeValueMemberL:
		return c.listToSliceWithConvention(v.Value, target, inheritedConvention, inheritNaming)
	case *types.AttributeValueMemberM:
		return c.fromAttributeValueMapWithConvention(v.Value, target, inheritedConvention, inheritNaming)
	case *types.AttributeValueMemberSS:
		return c.stringSetToSlice(v.Value, target)
	case *types.AttributeValueMemberNS:
		return c.numberSetToSlice(v.Value, target)
	case *types.AttributeValueMemberBS:
		return c.binarySetToSlice(v.Value, target)
	default:
		return fmt.Errorf("unsupported AttributeValue type: %T", av)
	}
}

func (c *Converter) fromAttributeValueMapWithConvention(value map[string]types.AttributeValue, target reflect.Value, inheritedConvention naming.Convention, inheritNaming bool) error {
	switch target.Kind() {
	case reflect.Map:
		return c.attributeValueMapToMapWithConvention(value, target, inheritedConvention, inheritNaming)
	case reflect.Struct:
		return c.mapToStructWithConvention(value, target, inheritedConvention, inheritNaming)
	default:
		return fmt.Errorf("cannot convert map to %s", target.Type())
	}
}

// stringToValue converts string AttributeValue to various Go types
func (c *Converter) stringToValue(s string, target reflect.Value) error {
	switch target.Kind() {
	case reflect.String:
		target.SetString(s)
		return nil
	default:
		return fmt.Errorf("cannot convert string to %s", target.Type())
	}
}

// numberToValue converts number AttributeValue to various Go types
func (c *Converter) numberToValue(n string, target reflect.Value) error {
	switch target.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i, err := strconv.ParseInt(n, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid number: %w", err)
		}
		target.SetInt(i)
		return nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		u, err := strconv.ParseUint(n, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid number: %w", err)
		}
		target.SetUint(u)
		return nil

	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(n, 64)
		if err != nil {
			return fmt.Errorf("invalid number: %w", err)
		}
		target.SetFloat(f)
		return nil

	default:
		return fmt.Errorf("cannot convert number to %s", target.Type())
	}
}

func (c *Converter) listToSliceWithConvention(list []types.AttributeValue, target reflect.Value, inheritedConvention naming.Convention, inheritNaming bool) error {
	if target.Kind() != reflect.Slice {
		return fmt.Errorf("target must be slice, got %s", target.Type())
	}

	slice := reflect.MakeSlice(target.Type(), len(list), len(list))

	for i, av := range list {
		if err := c.fromAttributeValueWithConvention(av, slice.Index(i), inheritedConvention, inheritNaming); err != nil {
			return fmt.Errorf("index %d: %w", i, err)
		}
	}

	target.Set(slice)
	return nil
}

func (c *Converter) attributeValueMapToMapWithConvention(m map[string]types.AttributeValue, target reflect.Value, inheritedConvention naming.Convention, inheritNaming bool) error {
	if target.Kind() != reflect.Map {
		return fmt.Errorf("target must be map, got %s", target.Type())
	}

	if target.Type().Key().Kind() != reflect.String {
		return fmt.Errorf("map key must be string, got %s", target.Type().Key())
	}

	mapValue := reflect.MakeMap(target.Type())

	for k, av := range m {
		elem := reflect.New(target.Type().Elem()).Elem()
		if err := c.fromAttributeValueWithConvention(av, elem, inheritedConvention, inheritNaming); err != nil {
			return fmt.Errorf("key %s: %w", k, err)
		}
		mapValue.SetMapIndex(reflect.ValueOf(k), elem)
	}

	target.Set(mapValue)
	return nil
}

func (c *Converter) mapToStructWithConvention(m map[string]types.AttributeValue, target reflect.Value, inheritedConvention naming.Convention, inheritNaming bool) error {
	if target.Kind() != reflect.Struct {
		return fmt.Errorf("target must be struct, got %s", target.Type())
	}

	targetType := target.Type()
	convention, _ := resolveStructNaming(targetType, inheritedConvention, inheritNaming)

	fieldPlans, err := buildMapDecodeFieldPlans(targetType, convention)
	if err != nil {
		return err
	}

	for _, fieldPlan := range fieldPlans {
		attrNames, skip, err := resolveMapFieldLookupNames(fieldPlan.Field, convention)
		if err != nil {
			return err
		}
		if skip {
			continue
		}

		av, exists, err := lookupMapFieldPlanValue(m, fieldPlan, attrNames)
		if err != nil {
			return fmt.Errorf("field %s: %w", fieldPlan.Field.Name, err)
		}
		if !exists {
			continue
		}

		if err := c.fromAttributeValueWithConvention(av, target.FieldByIndex(fieldPlan.IndexPath), convention, true); err != nil {
			return fmt.Errorf("field %s: %w", fieldPlan.Field.Name, err)
		}
	}

	return nil
}

func resolveMapFieldLookupNames(field reflect.StructField, convention naming.Convention) ([]string, bool, error) {
	theorydbTag := field.Tag.Get("theorydb")
	jsonTag := field.Tag.Get("json")
	if theorydbTag == "-" || jsonTag == "-" {
		return nil, true, nil
	}

	names := make([]string, 0, 4)
	if theorydbTag != "" {
		attrName, skip := naming.ResolveAttrNameWithConvention(field, convention)
		if skip || attrName == "" {
			return nil, true, nil
		}
		if err := naming.ValidateAttrName(attrName, convention); err != nil {
			return nil, false, fmt.Errorf("field %s: %w", field.Name, err)
		}
		names = appendMapLookupName(names, attrName)
	}

	if jsonName := jsonTagName(jsonTag); jsonName != "" {
		names = appendMapLookupName(names, jsonName)
	}

	names = appendMapLookupName(names, naming.ConvertAttrName(field.Name, convention))
	names = appendMapLookupName(names, field.Name)
	if len(names) == 0 {
		return nil, true, nil
	}

	return names, false, nil
}

func buildMapDecodeFieldPlans(targetType reflect.Type, convention naming.Convention) ([]reflectutil.VisibleFieldPlan, error) {
	return reflectutil.BuildVisibleFieldPlan(targetType, func(field reflect.StructField) ([]string, bool, error) {
		return resolveMapFieldLookupNames(field, convention)
	})
}

func lookupMapFieldPlanValue(m map[string]types.AttributeValue, fieldPlan reflectutil.VisibleFieldPlan, attrNames []string) (types.AttributeValue, bool, error) {
	if av, exists := lookupMapFieldValue(m, attrNames...); exists {
		return av, true, nil
	}

	return lookupLegacyMapFieldPlanValue(m, fieldPlan.LegacyContainers, attrNames)
}

func lookupLegacyMapFieldPlanValue(m map[string]types.AttributeValue, containers []reflectutil.LegacyContainerPlan, attrNames []string) (types.AttributeValue, bool, error) {
	if len(containers) == 0 {
		return nil, false, nil
	}

	current := m
	for _, container := range containers {
		if len(container.Aliases) == 0 {
			return nil, false, nil
		}

		av, exists := lookupMapFieldValue(current, container.Aliases...)
		if !exists {
			return nil, false, nil
		}

		nested, ok := av.(*types.AttributeValueMemberM)
		if !ok {
			return nil, false, fmt.Errorf("legacy anonymous container %s must decode from a map, got %T", container.Field.Name, av)
		}

		current = nested.Value
	}

	av, exists := lookupMapFieldValue(current, attrNames...)
	return av, exists, nil
}

func appendMapLookupName(names []string, name string) []string {
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

func lookupMapFieldValue(m map[string]types.AttributeValue, names ...string) (types.AttributeValue, bool) {
	for _, name := range names {
		av, exists := m[name]
		if exists {
			return av, true
		}
	}
	return nil, false
}

// stringSetToSlice converts string set to slice
func (c *Converter) stringSetToSlice(set []string, target reflect.Value) error {
	if target.Kind() != reflect.Slice || target.Type().Elem().Kind() != reflect.String {
		return fmt.Errorf("target must be []string, got %s", target.Type())
	}

	slice := reflect.MakeSlice(target.Type(), len(set), len(set))
	for i, s := range set {
		slice.Index(i).SetString(s)
	}

	target.Set(slice)
	return nil
}

// numberSetToSlice converts number set to slice
func (c *Converter) numberSetToSlice(set []string, target reflect.Value) error {
	if target.Kind() != reflect.Slice {
		return fmt.Errorf("target must be slice, got %s", target.Type())
	}

	slice := reflect.MakeSlice(target.Type(), len(set), len(set))

	for i, n := range set {
		if err := c.numberToValue(n, slice.Index(i)); err != nil {
			return fmt.Errorf("index %d: %w", i, err)
		}
	}

	target.Set(slice)
	return nil
}

// binarySetToSlice converts binary set to slice
func (c *Converter) binarySetToSlice(set [][]byte, target reflect.Value) error {
	if target.Kind() != reflect.Slice || target.Type().Elem().Kind() != reflect.Slice {
		return fmt.Errorf("target must be [][]byte, got %s", target.Type())
	}

	slice := reflect.MakeSlice(target.Type(), len(set), len(set))

	for i, b := range set {
		slice.Index(i).SetBytes(b)
	}

	target.Set(slice)
	return nil
}

func attributeValueToAny(av types.AttributeValue) (any, error) {
	switch v := av.(type) {
	case *types.AttributeValueMemberS:
		return v.Value, nil
	case *types.AttributeValueMemberN:
		return parseNumberToAny(v.Value)
	case *types.AttributeValueMemberBOOL:
		return v.Value, nil
	case *types.AttributeValueMemberNULL:
		return nil, nil
	case *types.AttributeValueMemberL:
		return attributeValueListToAny(v.Value)
	case *types.AttributeValueMemberM:
		return attributeValueMapToAny(v.Value)
	case *types.AttributeValueMemberSS:
		return v.Value, nil
	case *types.AttributeValueMemberNS:
		return attributeValueNumberSetToFloat64(v.Value)
	case *types.AttributeValueMemberBS:
		return v.Value, nil
	case *types.AttributeValueMemberB:
		return v.Value, nil
	default:
		return nil, fmt.Errorf("unsupported AttributeValue type: %T", av)
	}
}

func parseNumberToAny(value string) (any, error) {
	if intVal, err := strconv.ParseInt(value, 10, 64); err == nil {
		return intVal, nil
	}
	if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
		return floatVal, nil
	}
	return nil, fmt.Errorf("invalid number: %s", value)
}

func attributeValueListToAny(values []types.AttributeValue) ([]any, error) {
	out := make([]any, len(values))
	for i, value := range values {
		converted, err := attributeValueToAny(value)
		if err != nil {
			return nil, fmt.Errorf("index %d: %w", i, err)
		}
		out[i] = converted
	}
	return out, nil
}

func attributeValueMapToAny(values map[string]types.AttributeValue) (map[string]any, error) {
	out := make(map[string]any, len(values))
	for key, value := range values {
		converted, err := attributeValueToAny(value)
		if err != nil {
			return nil, fmt.Errorf("key %s: %w", key, err)
		}
		out[key] = converted
	}
	return out, nil
}

func attributeValueNumberSetToFloat64(values []string) ([]float64, error) {
	out := make([]float64, len(values))
	for i, value := range values {
		converted, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return nil, fmt.Errorf("index %d: %w", i, err)
		}
		out[i] = converted
	}
	return out, nil
}

// ConvertToSet determines if a slice should be converted to a DynamoDB set
func (c *Converter) ConvertToSet(slice any, isSet bool) (types.AttributeValue, error) {
	if !isSet {
		return c.ToAttributeValue(slice)
	}

	v := reflect.ValueOf(slice)
	if v.Kind() != reflect.Slice {
		return nil, fmt.Errorf("%w: set tag requires slice type", errors.ErrInvalidTag)
	}

	// Handle empty slices
	if v.Len() == 0 {
		return &types.AttributeValueMemberNULL{Value: true}, nil
	}

	elemType := v.Type().Elem()

	switch elemType.Kind() {
	case reflect.String:
		set := make([]string, v.Len())
		for i := 0; i < v.Len(); i++ {
			set[i] = v.Index(i).String()
		}
		return &types.AttributeValueMemberSS{Value: set}, nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		set := make([]string, v.Len())
		for i := 0; i < v.Len(); i++ {
			av, err := c.toAttributeValueWithConvention(v.Index(i), naming.CamelCase, false)
			if err != nil {
				return nil, err
			}
			if n, ok := av.(*types.AttributeValueMemberN); ok {
				set[i] = n.Value
			} else {
				return nil, fmt.Errorf("expected number type for set")
			}
		}
		return &types.AttributeValueMemberNS{Value: set}, nil

	case reflect.Slice:
		if elemType.Elem().Kind() == reflect.Uint8 {
			// [][]byte
			set := make([][]byte, v.Len())
			for i := 0; i < v.Len(); i++ {
				set[i] = v.Index(i).Bytes()
			}
			return &types.AttributeValueMemberBS{Value: set}, nil
		}

	default:
		return nil, fmt.Errorf("%w: unsupported set element type: %s", errors.ErrUnsupportedType, elemType)
	}

	return nil, fmt.Errorf("%w: unsupported set type", errors.ErrUnsupportedType)
}

// detectNamingConvention scans struct fields for a naming convention tag.
// It looks for a field with tag `theorydb:"naming:snake_case"`.
// Returns CamelCase (default) if no naming tag is found.
func detectNamingConvention(modelType reflect.Type) naming.Convention {
	for i := 0; i < modelType.NumField(); i++ {
		field := modelType.Field(i)
		tag := field.Tag.Get("theorydb")

		if tag == "" {
			continue
		}

		// Look for naming:snake_case or naming:camel_case
		parts := splitTag(tag)
		for _, part := range parts {
			if len(part) > 7 && part[:7] == "naming:" {
				convention := part[7:]
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
	}

	// Default to CamelCase
	return naming.CamelCase
}

func structUsesTheorydbNaming(modelType reflect.Type) bool {
	for i := 0; i < modelType.NumField(); i++ {
		if modelType.Field(i).Tag.Get("theorydb") != "" {
			return true
		}
	}

	return false
}

// splitTag splits a tag string by commas
func splitTag(tag string) []string {
	if tag == "" {
		return nil
	}

	var parts []string
	current := ""

	for _, ch := range tag {
		if ch == ',' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else if ch != ' ' && ch != '\t' {
			current += string(ch)
		}
	}

	if current != "" {
		parts = append(parts, current)
	}

	return parts
}

func jsonTagName(tag string) string {
	if tag == "" || tag == "-" {
		return ""
	}

	parts := strings.SplitN(tag, ",", 2)
	if len(parts) == 0 {
		return ""
	}

	name := strings.TrimSpace(parts[0])
	if name == "" || name == "-" {
		return ""
	}

	return name
}

func explicitNamingConvention(modelType reflect.Type) (naming.Convention, bool) {
	for i := 0; i < modelType.NumField(); i++ {
		tag := modelType.Field(i).Tag.Get("theorydb")
		if tag == "" {
			continue
		}

		for _, part := range splitTag(tag) {
			if len(part) <= 7 || part[:7] != "naming:" {
				continue
			}

			switch part[7:] {
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

	return detectNamingConvention(modelType), structUsesTheorydbNaming(modelType)
}
