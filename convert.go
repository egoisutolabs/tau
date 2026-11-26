package tau

import (
	"encoding"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
)

// Convert builds a To from a value of another type by matching fields. It
// is for the mapping Go makes you write out by hand: a database row into an
// API response, a request into a command.
//
//	resp := tau.Convert[OrderResponse](order)
//	list := tau.Convert[[]OrderResponse](orders)
//
// A destination field takes the source field with the same Go name, else
// the same json name. Every exported destination field must find a source,
// or carry a tag saying where its value comes from:
//
//	Email string `tau:"Customer.Email"` // a path of Go field names
//	Total Money  `tau:"-"`              // set by the caller afterwards
//
// Source fields with no destination are ignored: leaving things out is what
// a response type is for.
//
// Field values convert as follows; anything else is refused.
//
//   - The same type is assigned as it is. Slices and maps of the same type
//     are shared with the source, not copied.
//   - Structs, slices, maps and pointers convert element by element. A nil
//     pointer, slice or map stays nil; a nil pointer converted into a
//     non-pointer field leaves the zero value.
//   - A named type converts to and from its basic kind (an enum declared as
//     a string, to a string). Numbers only widen: int32 to int, never int32
//     to int16.
//   - A type with a text form (encoding.TextMarshaler: most IDs,
//     time.Time) converts to a string, and a string converts into a type
//     that can read one (encoding.TextUnmarshaler).
//
// Convert throws a *ConvertError. Whether two types can be converted is
// decided from the types alone, once, so a wrong mapping throws on every
// call whatever the values are: one test that runs a conversion proves its
// field mapping. Only reading a string into a TextUnmarshaler can fail
// because of a value.
//
// Convert works by reflection and costs several times a hand-written
// mapper (about 1 µs for a response of twenty fields). Use it to build
// responses; write the loop by hand for thousands of rows.
func Convert[To any](from any) To {
	var out To
	src := reflect.ValueOf(from)
	to := reflect.TypeFor[To]()
	if !src.IsValid() {
		Throw(&ConvertError{To: to, Err: errors.New("the source is nil")})
	}
	convertPlanFor(src.Type(), to)(reflect.ValueOf(&out).Elem(), src)
	return out
}

// ConvertError is what Convert throws. Field is the path of the destination
// field at fault, such as "Items[].Price", or empty when the fault is with
// the value as a whole.
type ConvertError struct {
	From, To reflect.Type // From is nil when the source was nil
	Field    string
	Err      error
}

func (e *ConvertError) Error() string {
	from := "nil"
	if e.From != nil {
		from = e.From.String()
	}
	if e.Field == "" {
		return fmt.Sprintf("tau: cannot convert %s to %s: %v", from, e.To, e.Err)
	}
	return fmt.Sprintf("tau: cannot convert %s to %s: field %s: %v", from, e.To, e.Field, e.Err)
}

func (e *ConvertError) Unwrap() error { return e.Err }

// A convertStep copies src into dst. Steps are built once per pair of types.
type convertStep func(dst, src reflect.Value)

type convertPair struct{ from, to reflect.Type }

type convertCached struct {
	run convertStep
	err error
}

var convertPlans sync.Map // convertPair to convertCached

func convertPlanFor(from, to reflect.Type) convertStep {
	key := convertPair{from, to}
	if c, ok := convertPlans.Load(key); ok {
		c := c.(convertCached)
		Check(c.err)
		return c.run
	}
	b := &convertBuilder{from: from, to: to, building: map[convertPair]*convertStep{}}
	c := convertCached{}
	c.err = Try(func() { c.run = b.build(from, to, "") })
	convertPlans.Store(key, c)
	Check(c.err)
	return c.run
}

type convertBuilder struct {
	from, to reflect.Type
	// Pairs being built further up the stack: a type that contains itself
	// (a User with a Manager *User) refers back to its own unfinished step.
	building map[convertPair]*convertStep
}

func (b *convertBuilder) fail(path string, err error) {
	Throw(&ConvertError{From: b.from, To: b.to, Field: strings.TrimPrefix(path, "."), Err: err})
}

func (b *convertBuilder) failf(path, format string, args ...any) {
	b.fail(path, fmt.Errorf(format, args...))
}

var (
	convertTextMarshaler   = reflect.TypeFor[encoding.TextMarshaler]()
	convertTextUnmarshaler = reflect.TypeFor[encoding.TextUnmarshaler]()
)

func (b *convertBuilder) build(st, dt reflect.Type, path string) convertStep {
	if st == dt {
		return func(dst, src reflect.Value) { dst.Set(src) }
	}
	key := convertPair{st, dt}
	if pending, ok := b.building[key]; ok {
		return func(dst, src reflect.Value) { (*pending)(dst, src) }
	}
	var self convertStep
	b.building[key] = &self
	defer delete(b.building, key)
	self = b.buildNew(st, dt, path)
	return self
}

func (b *convertBuilder) buildNew(st, dt reflect.Type, path string) convertStep {
	switch {
	case dt.Kind() == reflect.Pointer && st.Kind() == reflect.Pointer:
		inner := b.build(st.Elem(), dt.Elem(), path)
		return func(dst, src reflect.Value) {
			if src.IsNil() {
				dst.SetZero()
				return
			}
			p := reflect.New(dt.Elem())
			inner(p.Elem(), src.Elem())
			dst.Set(p)
		}
	case dt.Kind() == reflect.Pointer: // a value becomes a pointer to a copy
		inner := b.build(st, dt.Elem(), path)
		return func(dst, src reflect.Value) {
			p := reflect.New(dt.Elem())
			inner(p.Elem(), src)
			dst.Set(p)
		}
	case st.Kind() == reflect.Pointer: // a nil pointer becomes the zero value
		inner := b.build(st.Elem(), dt, path)
		return func(dst, src reflect.Value) {
			if src.IsNil() {
				dst.SetZero()
				return
			}
			inner(dst, src.Elem())
		}
	case dt.Kind() == reflect.String && st.Implements(convertTextMarshaler):
		// An ID, a time, an enum: anything that knows its own text form.
		return func(dst, src reflect.Value) {
			text, err := src.Interface().(encoding.TextMarshaler).MarshalText()
			if err != nil {
				b.fail(path, err)
			}
			dst.SetString(string(text))
		}
	case st.Kind() == reflect.String && reflect.PointerTo(dt).Implements(convertTextUnmarshaler):
		return func(dst, src reflect.Value) {
			if err := dst.Addr().Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(src.String())); err != nil {
				b.fail(path, err)
			}
		}
	case st.Kind() == reflect.Struct && dt.Kind() == reflect.Struct:
		return b.structs(st, dt, path)
	case (st.Kind() == reflect.Slice || st.Kind() == reflect.Array) && dt.Kind() == reflect.Slice:
		inner := b.build(st.Elem(), dt.Elem(), path+"[]")
		return func(dst, src reflect.Value) {
			if src.Kind() == reflect.Slice && src.IsNil() {
				dst.SetZero()
				return
			}
			out := reflect.MakeSlice(dt, src.Len(), src.Len())
			for i := range src.Len() {
				inner(out.Index(i), src.Index(i))
			}
			dst.Set(out)
		}
	case st.Kind() == reflect.Map && dt.Kind() == reflect.Map:
		keys := b.build(st.Key(), dt.Key(), path+"[key]")
		values := b.build(st.Elem(), dt.Elem(), path+"[]")
		return func(dst, src reflect.Value) {
			if src.IsNil() {
				dst.SetZero()
				return
			}
			out := reflect.MakeMapWithSize(dt, src.Len())
			for iter := src.MapRange(); iter.Next(); {
				k, v := reflect.New(dt.Key()).Elem(), reflect.New(dt.Elem()).Elem()
				keys(k, iter.Key())
				values(v, iter.Value())
				out.SetMapIndex(k, v)
			}
			dst.Set(out)
		}
	case dt.Kind() == reflect.Interface && st.Implements(dt):
		return func(dst, src reflect.Value) { dst.Set(src) }
	case convertSafeScalar(st, dt):
		return func(dst, src reflect.Value) { dst.Set(src.Convert(dt)) }
	}
	b.failf(path, "no conversion from %s to %s", st, dt)
	return nil
}

// convertSafeScalar reports whether every value of st is exactly representable as
// a dt: the same basic kind under a different name (an enum to a string),
// or a number widened.
func convertSafeScalar(st, dt reflect.Type) bool {
	sk, dk := st.Kind(), dt.Kind()
	if sk == dk {
		switch sk {
		case reflect.Bool, reflect.String, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64:
			return true
		}
		return false
	}
	signed := func(k reflect.Kind) bool { return k >= reflect.Int && k <= reflect.Int64 }
	unsigned := func(k reflect.Kind) bool { return k >= reflect.Uint && k <= reflect.Uint64 }
	switch {
	case signed(sk) && signed(dk), unsigned(sk) && unsigned(dk):
		return st.Bits() <= dt.Bits()
	case unsigned(sk) && signed(dk):
		return st.Bits() < dt.Bits()
	case (signed(sk) || unsigned(sk)) && dk == reflect.Float64:
		return st.Bits() <= 32
	case sk == reflect.Float32 && dk == reflect.Float64:
		return true
	}
	return false
}

// structs matches every destination field to a source.
func (b *convertBuilder) structs(st, dt reflect.Type, path string) convertStep {
	byName, byJSON := map[string][]int{}, map[string][]int{}
	for _, f := range reflect.VisibleFields(st) {
		if !f.IsExported() {
			continue
		}
		byName[f.Name] = f.Index
		if name := convertJSONName(f); name != "" {
			if _, taken := byJSON[name]; !taken {
				byJSON[name] = f.Index
			}
		}
	}
	type fieldStep struct {
		dst, src []int
		run      convertStep
	}
	var steps []fieldStep
	for _, f := range reflect.VisibleFields(dt) {
		if !f.IsExported() || (f.Anonymous && f.Type.Kind() == reflect.Struct) {
			continue // an embedded struct is filled through its promoted fields
		}
		at := path + "." + f.Name
		if (f.Anonymous && f.Type.Kind() == reflect.Pointer) || convertThroughPointer(dt, f.Index) {
			b.failf(at, "embedded pointers in the destination are not supported")
		}
		tag := f.Tag.Get("tau")
		if tag == "-" {
			continue
		}
		var src []int
		switch {
		case tag != "":
			src = b.sourcePath(st, tag, at)
		case byName[f.Name] != nil:
			src = byName[f.Name]
		case byJSON[convertJSONName(f)] != nil:
			src = byJSON[convertJSONName(f)]
		default:
			b.failf(at, "%s has no field for it; add one, or tag it `tau:\"-\"` and set it yourself", st)
		}
		steps = append(steps, fieldStep{f.Index, src, b.build(convertTypeAt(st, src), f.Type, at)})
	}
	return func(dst, src reflect.Value) {
		for _, s := range steps {
			// A nil pointer on the way to the source leaves the zero value.
			if v, err := src.FieldByIndexErr(s.src); err == nil {
				s.run(dst.FieldByIndex(s.dst), v)
			}
		}
	}
}

// sourcePath resolves a `tau:"Customer.Email"` tag to a field index path,
// following pointers.
func (b *convertBuilder) sourcePath(st reflect.Type, tag, at string) []int {
	var index []int
	t := st
	for name := range strings.SplitSeq(tag, ".") {
		for t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
		if t.Kind() != reflect.Struct {
			b.failf(at, "tag %q: %s is not a struct", tag, t)
		}
		f, ok := t.FieldByName(name)
		if !ok || !f.IsExported() {
			b.failf(at, "tag %q: %s has no exported field %s", tag, t, name)
		}
		index = append(index, f.Index...)
		t = f.Type
	}
	return index
}

func convertTypeAt(t reflect.Type, index []int) reflect.Type {
	for _, i := range index {
		for t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
		t = t.Field(i).Type
	}
	return t
}

func convertThroughPointer(t reflect.Type, index []int) bool {
	for _, i := range index[:len(index)-1] {
		f := t.Field(i)
		if f.Type.Kind() == reflect.Pointer {
			return true
		}
		t = f.Type
	}
	return false
}

func convertJSONName(f reflect.StructField) string {
	name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
	if name == "-" {
		return ""
	}
	return name
}
