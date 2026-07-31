package reflectutil

import (
	"reflect"
	"time"
)

var timeType = reflect.TypeOf(time.Time{})

// IsEmpty reports whether v should be treated as "empty" for omitempty semantics.
//
// Container emptiness follows the DMS wire shape: arrays/lists are empty only
// at length zero, and map/object/struct carriers for M are empty only when they
// have zero entries/declared fields. time.Time retains its native zero rule.
func IsEmpty(v reflect.Value) bool {
	if !v.IsValid() {
		return true
	}

	switch v.Kind() {
	case reflect.Array:
		return v.Len() == 0

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

func isEmptyStruct(v reflect.Value) bool {
	if v.Type() == timeType {
		if v.CanInterface() {
			if t, ok := v.Interface().(time.Time); ok {
				return t.IsZero()
			}
		}
		return v.IsZero()
	}

	return v.NumField() == 0
}
