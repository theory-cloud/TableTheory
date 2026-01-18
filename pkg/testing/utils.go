package testing

import (
	"reflect"
)

// getTypeString returns the type string for use with mock.AnythingOfType
func getTypeString(v interface{}) string {
	return reflect.TypeOf(v).String()
}

// copyValue copies the value from src to dst using reflection
func copyValue(dst, src interface{}) {
	dstVal := reflect.ValueOf(dst)
	srcVal := reflect.ValueOf(src)

	if dstVal.Kind() == reflect.Ptr && srcVal.Kind() == reflect.Ptr {
		dstVal.Elem().Set(srcVal.Elem())
	} else if dstVal.Kind() == reflect.Ptr {
		dstVal.Elem().Set(srcVal)
	}
}
