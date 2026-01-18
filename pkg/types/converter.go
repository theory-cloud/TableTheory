// Package types provides type conversion between Go types and DynamoDB AttributeValues
package types

import (
	"fmt"
	"reflect"
	"strconv"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/naming"
)

// Converter handles conversion between Go types and DynamoDB AttributeValues
type Converter struct {
	// customConverters allows registration of custom type converters
	customConverters map[reflect.Type]CustomConverter
	mu               sync.RWMutex
}

var timeType = reflect.TypeOf(time.Time{})

// CustomConverter defines the interface for custom type converters
type CustomConverter interface {
	// ToAttributeValue converts a Go value to DynamoDB AttributeValue
	ToAttributeValue(value any) (types.AttributeValue, error)

	// FromAttributeValue converts a DynamoDB AttributeValue to Go value
	FromAttributeValue(av types.AttributeValue, target any) error
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

// ToAttributeValue converts a Go value to DynamoDB AttributeValue
func (c *Converter) ToAttributeValue(value any) (types.AttributeValue, error) {
	if value == nil {
		return &types.AttributeValueMemberNULL{Value: true}, nil
	}

	v := reflect.ValueOf(value)
	return c.toAttributeValue(v)
}

// toAttributeValue handles the actual conversion based on reflection
func (c *Converter) toAttributeValue(v reflect.Value) (types.AttributeValue, error) {
	// Handle pointer types
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return &types.AttributeValueMemberNULL{Value: true}, nil
		}
		v = v.Elem()
	}

	// Check for custom converter
	if converter, exists := c.lookupConverter(v.Type()); exists {
		return converter.ToAttributeValue(v.Interface())
	}

	// Handle time.Time specially
	if v.Type() == reflect.TypeOf(time.Time{}) {
		t, ok := v.Interface().(time.Time)
		if !ok {
			return nil, fmt.Errorf("expected time.Time, got %T", v.Interface())
		}
		return &types.AttributeValueMemberS{Value: t.Format(time.RFC3339Nano)}, nil
	}

	// Handle basic types
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
			// []byte -> Binary
			return &types.AttributeValueMemberB{Value: v.Bytes()}, nil
		}
		// Handle other slices as lists
		return c.sliceToList(v)

	case reflect.Map:
		return c.mapToAttributeValueMap(v)

	case reflect.Struct:
		return c.structToMap(v)

	default:
		return nil, fmt.Errorf("%w: %s", errors.ErrUnsupportedType, v.Type())
	}
}

// sliceToList converts a slice to DynamoDB List
func (c *Converter) sliceToList(v reflect.Value) (types.AttributeValue, error) {
	list := make([]types.AttributeValue, v.Len())

	for i := 0; i < v.Len(); i++ {
		av, err := c.toAttributeValue(v.Index(i))
		if err != nil {
			return nil, fmt.Errorf("index %d: %w", i, err)
		}
		list[i] = av
	}

	return &types.AttributeValueMemberL{Value: list}, nil
}

// mapToAttributeValueMap converts a map to DynamoDB Map
func (c *Converter) mapToAttributeValueMap(v reflect.Value) (types.AttributeValue, error) {
	if v.Type().Key().Kind() != reflect.String {
		return nil, fmt.Errorf("%w: map keys must be strings", errors.ErrUnsupportedType)
	}

	m := make(map[string]types.AttributeValue)

	for _, key := range v.MapKeys() {
		keyStr := key.String()
		val := v.MapIndex(key)

		av, err := c.toAttributeValue(val)
		if err != nil {
			return nil, fmt.Errorf("key %s: %w", keyStr, err)
		}
		m[keyStr] = av
	}

	return &types.AttributeValueMemberM{Value: m}, nil
}

// structToMap converts a struct to DynamoDB Map
func (c *Converter) structToMap(v reflect.Value) (types.AttributeValue, error) {
	m := make(map[string]types.AttributeValue)
	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		fieldValue := v.Field(i)
		if fieldValue.IsZero() {
			continue // Skip zero values for now
		}

		av, err := c.toAttributeValue(fieldValue)
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", field.Name, err)
		}

		m[field.Name] = av
	}

	return &types.AttributeValueMemberM{Value: m}, nil
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

	return c.fromAttributeValue(av, targetValue.Elem())
}

// fromAttributeValue handles the actual conversion from AttributeValue
func (c *Converter) fromAttributeValue(av types.AttributeValue, target reflect.Value) error {
	if _, ok := av.(*types.AttributeValueMemberNULL); ok {
		return nil
	}

	if !target.CanSet() {
		return fmt.Errorf("target is not settable")
	}

	target = ensureSettableConcreteTarget(target)

	if converter, exists := c.lookupConverter(target.Type()); exists {
		return converter.FromAttributeValue(av, target.Addr().Interface())
	}

	if target.Type() == timeType {
		return c.fromAttributeValueTime(av, target)
	}

	return c.fromAttributeValueByType(av, target)
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

func (c *Converter) fromAttributeValueByType(av types.AttributeValue, target reflect.Value) error {
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
		return c.listToSlice(v.Value, target)
	case *types.AttributeValueMemberM:
		return c.fromAttributeValueMap(v.Value, target)
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

func (c *Converter) fromAttributeValueMap(value map[string]types.AttributeValue, target reflect.Value) error {
	switch target.Kind() {
	case reflect.Map:
		return c.attributeValueMapToMap(value, target)
	case reflect.Struct:
		return c.mapToStruct(value, target)
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

// listToSlice converts DynamoDB List to Go slice
func (c *Converter) listToSlice(list []types.AttributeValue, target reflect.Value) error {
	if target.Kind() != reflect.Slice {
		return fmt.Errorf("target must be slice, got %s", target.Type())
	}

	slice := reflect.MakeSlice(target.Type(), len(list), len(list))

	for i, av := range list {
		if err := c.fromAttributeValue(av, slice.Index(i)); err != nil {
			return fmt.Errorf("index %d: %w", i, err)
		}
	}

	target.Set(slice)
	return nil
}

// attributeValueMapToMap converts DynamoDB Map to Go map
func (c *Converter) attributeValueMapToMap(m map[string]types.AttributeValue, target reflect.Value) error {
	if target.Kind() != reflect.Map {
		return fmt.Errorf("target must be map, got %s", target.Type())
	}

	if target.Type().Key().Kind() != reflect.String {
		return fmt.Errorf("map key must be string, got %s", target.Type().Key())
	}

	mapValue := reflect.MakeMap(target.Type())

	for k, av := range m {
		elem := reflect.New(target.Type().Elem()).Elem()
		if err := c.fromAttributeValue(av, elem); err != nil {
			return fmt.Errorf("key %s: %w", k, err)
		}
		mapValue.SetMapIndex(reflect.ValueOf(k), elem)
	}

	target.Set(mapValue)
	return nil
}

// mapToStruct converts DynamoDB Map to Go struct
func (c *Converter) mapToStruct(m map[string]types.AttributeValue, target reflect.Value) error {
	if target.Kind() != reflect.Struct {
		return fmt.Errorf("target must be struct, got %s", target.Type())
	}

	targetType := target.Type()

	// Detect naming convention from struct tags
	convention := detectNamingConvention(targetType)

	for i := 0; i < targetType.NumField(); i++ {
		field := targetType.Field(i)
		if !field.IsExported() {
			continue
		}

		attrName, skip := naming.ResolveAttrNameWithConvention(field, convention)
		if skip {
			continue
		}

		if err := naming.ValidateAttrName(attrName, convention); err != nil {
			return fmt.Errorf("field %s: %w", field.Name, err)
		}

		av, exists := m[attrName]
		if !exists {
			continue
		}

		if err := c.fromAttributeValue(av, target.Field(i)); err != nil {
			return fmt.Errorf("field %s: %w", field.Name, err)
		}
	}

	return nil
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
			av, err := c.toAttributeValue(v.Index(i))
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
				}
			}
		}
	}

	// Default to CamelCase
	return naming.CamelCase
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
