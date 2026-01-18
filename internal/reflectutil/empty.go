package reflectutil

import (
	"reflect"
	"time"
)

var timeType = reflect.TypeOf(time.Time{})

// IsEmpty reports whether v should be treated as "empty" for omitempty semantics.
//
// This is similar to encoding/json's emptiness rules but also treats structs
// (including nested structs) as empty when all fields are empty, and treats
// time.Time as empty when IsZero() is true.
func IsEmpty(v reflect.Value) bool {
	if !v.IsValid() {
		return true
	}

	switch v.Kind() {
	case reflect.Array:
		return isEmptyArray(v)

	case reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0

	case reflect.Bool:
		return !v.Bool()

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0

	case reflect.Float32, reflect.Float64:
		return v.Float() == 0

	case reflect.Interface, reflect.Ptr:
		return v.IsNil()

	case reflect.Struct:
		return isEmptyStruct(v)

	default:
		return v.IsZero()
	}
}

func isEmptyArray(v reflect.Value) bool {
	for i := 0; i < v.Len(); i++ {
		if !IsEmpty(v.Index(i)) {
			return false
		}
	}
	return true
}

func isEmptyStruct(v reflect.Value) bool {
	if v.Type() == timeType {
		if v.CanInterface() {
			if t, ok := v.Interface().(time.Time); ok {
				return t.IsZero()
			}
		}
		return v.IsZero()
	}

	for i := 0; i < v.NumField(); i++ {
		if !IsEmpty(v.Field(i)) {
			return false
		}
	}
	return true
}
