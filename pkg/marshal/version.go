package marshal

import (
	"fmt"
	"reflect"
)

const maxInt64AsUint64 = ^uint64(0) >> 1

func versionNumberFromValue(v reflect.Value) (int64, error) {
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int(), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		u := v.Uint()
		if u > maxInt64AsUint64 {
			return 0, fmt.Errorf("version value overflows int64")
		}
		return int64(u), nil
	default:
		return 0, fmt.Errorf("unsupported version kind: %s", v.Kind())
	}
}
