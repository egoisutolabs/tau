package tau

import "reflect"

// Dig walks untyped data, as produced by decoding JSON into `any`, and
// returns the value at path if it is there and is a T. It stands in for
// optional chaining:
//
//	city, ok := tau.Dig[string](data, "user", "addresses", 0, "city")
//
// A string step looks up a key in a map[string]any; an int step indexes a
// []any, counting from the end if negative. JSON numbers decode as float64;
// Dig converts a whole float64 to any integer type asked for.
func Dig[T any](data any, path ...any) (T, bool) {
	var zero T
	cur := data
	for _, step := range path {
		switch key := step.(type) {
		case string:
			m, ok := cur.(map[string]any)
			if !ok {
				return zero, false
			}
			if cur, ok = m[key]; !ok {
				return zero, false
			}
		case int:
			s, ok := cur.([]any)
			if !ok {
				return zero, false
			}
			if key < 0 {
				key += len(s)
			}
			if key < 0 || key >= len(s) {
				return zero, false
			}
			cur = s[key]
		default:
			return zero, false
		}
	}
	if v, ok := cur.(T); ok {
		return v, true
	}
	// A whole JSON number asked for as an integer type. The range check comes
	// first: converting an out-of-range float64 to int64 is not defined.
	if f, ok := cur.(float64); ok && f >= -1<<63 && f < 1<<63 && f == float64(int64(f)) {
		out := reflect.New(reflect.TypeFor[T]()).Elem()
		switch {
		case out.CanInt() && !out.OverflowInt(int64(f)):
			out.SetInt(int64(f))
			return out.Interface().(T), true
		case out.CanUint() && f >= 0 && !out.OverflowUint(uint64(f)):
			out.SetUint(uint64(f))
			return out.Interface().(T), true
		}
	}
	return zero, false
}

// DigOr is Dig with a fallback: `data?.a?.b ?? fallback`.
func DigOr[T any](data any, fallback T, path ...any) T {
	if v, ok := Dig[T](data, path...); ok {
		return v
	}
	return fallback
}
