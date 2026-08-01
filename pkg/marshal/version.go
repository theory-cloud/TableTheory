package marshal

import (
	"reflect"

	"github.com/theory-cloud/tabletheory/v3/internal/reflectutil"
)

const maxInt64AsUint64 = ^uint64(0) >> 1

func versionNumberFromValue(v reflect.Value) (int64, error) {
	return reflectutil.VersionNumber(v)
}
