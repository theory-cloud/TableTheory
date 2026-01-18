package numutil

import "math"

// ClampIntToInt32 converts n to int32, clamping to the int32 range.
func ClampIntToInt32(n int) int32 {
	if n > math.MaxInt32 {
		return math.MaxInt32
	}
	if n < math.MinInt32 {
		return math.MinInt32
	}
	return int32(n)
}
