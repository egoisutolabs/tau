package tau

import (
	"reflect"
	"strings"
	"sync"

	"github.com/egoisutolabs/tau/internal/dighook"
)

// Dig walks nested data and returns the value at path if it is there and is
// a T. It stands in for optional chaining:
//
//	city, ok := tau.Dig[string](data, "user", "addresses", 0, "city")
//
// A string step looks up a key; an int step indexes a list, counting from
// the end if negative. Dig reports false for a missing key, an index out of
// range, a nil, or a value that is not a T.
//
// Data decoded from JSON into `any` (map[string]any and []any) is the fast
// case. Dig also reads, by reflection:
//
//   - any map with string keys and any slice or array, which covers
//     MongoDB's bson.M and bson.A;
//   - a slice of key/value pairs shaped like MongoDB's bson.D, where a
//     string step finds the pair with that key;
//   - structs, where a string step names a field by its json tag, else its
//     bson tag, else its Go name. Pointers are followed and unexported
//     fields are never returned.
//
// Numbers convert when nothing is lost: JSON gives float64 for every number
// and BSON gives int32, int64 or float64 depending on the value, so asking
// for an int works for any of them if the value is whole and fits.
//
// Reflection costs roughly three times a lookup on decoded JSON. Importing
// github.com/egoisutolabs/tau/taubson removes that cost for MongoDB
// documents. For a loop over many documents, decode into a struct and use
// its fields.
func Dig[T any](data any, path ...any) (T, bool) {
	var zero T
	// The walk holds either an `any` (cur) or, while it is inside typed
	// data, a reflect.Value (rv). Staying in reflect.Value form avoids
	// copying each struct or slice on the way into an interface.
	cur := data
	var rv reflect.Value
	for _, step := range path {
		var ok bool
		if !rv.IsValid() {
			handled := false
			switch key := step.(type) {
			case string:
				if m, isMap := cur.(map[string]any); isMap {
					cur, ok = m[key]
					handled = true
				} else if dighook.Key != nil {
					// Not assigned straight to cur: an unrecognised type
					// must reach the reflection below untouched.
					var next any
					if next, ok, handled = dighook.Key(cur, key); handled {
						cur = next
					}
				}
			case int:
				if s, isSlice := cur.([]any); isSlice {
					if key < 0 {
						key += len(s)
					}
					if ok = key >= 0 && key < len(s); ok {
						cur = s[key]
					}
					handled = true
				} else if dighook.Index != nil {
					var next any
					if next, ok, handled = dighook.Index(cur, key); handled {
						cur = next
					}
				}
			default:
				return zero, false
			}
			if handled {
				if !ok {
					return zero, false
				}
				continue
			}
			rv = reflect.ValueOf(cur)
		}
		if rv, ok = digDeref(rv); !ok {
			return zero, false
		}
		switch key := step.(type) {
		case string:
			rv, ok = digKey(rv, key)
		case int:
			rv, ok = digIndex(rv, key)
		default:
			ok = false
		}
		if !ok {
			return zero, false
		}
		// Back to the fast path as soon as the value is an interface.
		if rv.Kind() == reflect.Interface {
			cur, rv = rv.Interface(), reflect.Value{}
		}
	}
	if !rv.IsValid() {
		if v, ok := cur.(T); ok {
			return v, true
		}
		rv = reflect.ValueOf(cur)
	}
	return digConvert[T](rv)
}

// DigOr is Dig with a fallback: `data?.a?.b ?? fallback`.
func DigOr[T any](data any, fallback T, path ...any) T {
	if v, ok := Dig[T](data, path...); ok {
		return v
	}
	return fallback
}

// digDeref follows pointers and interfaces; a nil anywhere is a miss.
func digDeref(v reflect.Value) (reflect.Value, bool) {
	for v.IsValid() && (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) {
		if v.IsNil() {
			return v, false
		}
		v = v.Elem()
	}
	return v, v.IsValid()
}

// digKey resolves a string step on a string-keyed map, a list of key/value
// pairs, or a struct.
func digKey(v reflect.Value, key string) (reflect.Value, bool) {
	var none reflect.Value
	t := v.Type()
	switch v.Kind() {
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return none, false
		}
		found := v.MapIndex(reflect.ValueOf(key).Convert(t.Key()))
		return found, found.IsValid()
	case reflect.Slice, reflect.Array:
		pair, isPairs := digPairOf(t.Elem())
		if !isPairs {
			return none, false
		}
		for i := range v.Len() {
			if el := v.Index(i); el.Field(pair.key).String() == key {
				return el.Field(pair.value), true
			}
		}
	case reflect.Struct:
		path, found := digFieldsOf(t)[key]
		if !found {
			return none, false
		}
		// FieldByIndexErr, not FieldByIndex: a nil embedded pointer is a
		// miss, not a panic.
		f, err := v.FieldByIndexErr(path)
		return f, err == nil && f.CanInterface()
	}
	return none, false
}

// digIndex resolves an int step on a slice or array.
func digIndex(v reflect.Value, i int) (reflect.Value, bool) {
	if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
		return reflect.Value{}, false
	}
	if i < 0 {
		i += v.Len()
	}
	if i < 0 || i >= v.Len() {
		return reflect.Value{}, false
	}
	return v.Index(i), true
}

// digPair holds the field indexes of a key/value pair type, or -1.
type digPair struct{ key, value int }

var digPairs, digFields sync.Map // reflect.Type to digPair, to map[string][]int

// digPairOf reports whether t is shaped like bson.E: a struct with an
// exported string Key and an exported Value.
func digPairOf(t reflect.Type) (digPair, bool) {
	if cached, ok := digPairs.Load(t); ok {
		p := cached.(digPair)
		return p, p.key >= 0
	}
	p := digPair{-1, -1}
	if t.Kind() == reflect.Struct {
		k, hasKey := t.FieldByName("Key")
		v, hasValue := t.FieldByName("Value")
		if hasKey && hasValue && k.Type.Kind() == reflect.String && len(k.Index) == 1 && len(v.Index) == 1 {
			p = digPair{k.Index[0], v.Index[0]}
		}
	}
	digPairs.Store(t, p)
	return p, p.key >= 0
}

// digFieldsOf maps every name a struct's fields answer to. A json tag wins
// over a bson tag, which wins over the Go field name; within one of those,
// the shallowest field wins, as in encoding/json.
func digFieldsOf(t reflect.Type) map[string][]int {
	if cached, ok := digFields.Load(t); ok {
		return cached.(map[string][]int)
	}
	names := map[string][]int{}
	for _, tag := range []string{"json", "bson", ""} {
		for _, f := range reflect.VisibleFields(t) {
			if !f.IsExported() {
				continue
			}
			name := f.Name
			if tag != "" {
				name, _, _ = strings.Cut(f.Tag.Get(tag), ",")
				if name == "" || name == "-" {
					continue
				}
			}
			if _, taken := names[name]; !taken {
				names[name] = f.Index
			}
		}
	}
	digFields.Store(t, names)
	return names
}

// digConvert returns src as a T: directly, through a pointer, or by a
// numeric conversion that loses nothing.
func digConvert[T any](src reflect.Value) (T, bool) {
	var zero T
	want := reflect.TypeFor[T]()
	// Asked for the pointer itself: return it before following it. A nil
	// pointer is still a miss.
	if src.IsValid() && src.Kind() == reflect.Pointer && src.Type() == want && !src.IsNil() {
		return reflect.TypeAssert[T](src)
	}
	src, ok := digDeref(src)
	if !ok {
		return zero, false
	}
	if src.Type() == want || want.Kind() == reflect.Interface {
		return reflect.TypeAssert[T](src)
	}
	out := reflect.New(want).Elem()
	const exact = 1 << 53 // every integer up to here is a float64
	switch {
	case src.CanInt():
		n := src.Int()
		switch {
		case out.CanInt() && !out.OverflowInt(n):
			out.SetInt(n)
		case out.CanUint() && n >= 0 && !out.OverflowUint(uint64(n)):
			out.SetUint(uint64(n))
		case out.CanFloat() && n >= -exact && n <= exact && !digLossy(out, float64(n)):
			out.SetFloat(float64(n))
		default:
			return zero, false
		}
	case src.CanUint():
		n := src.Uint()
		switch {
		case out.CanUint() && !out.OverflowUint(n):
			out.SetUint(n)
		case out.CanInt() && n < 1<<63 && !out.OverflowInt(int64(n)):
			out.SetInt(int64(n))
		case out.CanFloat() && n <= exact && !digLossy(out, float64(n)):
			out.SetFloat(float64(n))
		default:
			return zero, false
		}
	case src.CanFloat():
		f := src.Float()
		// The range check comes first: converting an out-of-range float64
		// to int64 is not defined.
		whole := f >= -1<<63 && f < 1<<63 && f == float64(int64(f))
		switch {
		case out.CanFloat() && !digLossy(out, f):
			out.SetFloat(f)
		case whole && out.CanInt() && !out.OverflowInt(int64(f)):
			out.SetInt(int64(f))
		case whole && f >= 0 && out.CanUint() && !out.OverflowUint(uint64(f)):
			out.SetUint(uint64(f))
		default:
			return zero, false
		}
	default:
		return zero, false
	}
	return out.Interface().(T), true
}

// digLossy reports whether f changes when stored in out (a float32 target).
func digLossy(out reflect.Value, f float64) bool {
	return out.Kind() == reflect.Float32 && float64(float32(f)) != f
}
