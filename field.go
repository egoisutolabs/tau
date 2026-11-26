package tau

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
)

// Field is a JSON field that remembers whether it was sent. It has three
// states, which a plain pointer cannot tell apart:
//
//   - omitted: the key was not in the JSON ("leave it unchanged")
//   - null: the key was there with the value null ("clear it")
//   - a value ("replace it")
//
// It is meant for PATCH-style requests:
//
//	type UpdateProfile struct {
//		Bio Field[string] `json:"bio,omitzero" validate:"omitnil,max=200"`
//	}
//
//	profile.Bio = req.Bio.OrElse(profile.Bio)
//
// The zero Field is omitted. Give the field the `omitzero` JSON option so an
// omitted Field is also left out when encoding; without it, it encodes as
// null.
//
// For validation a Field behaves like a pointer to its value that is nil
// when the field is omitted or null. Start its rules with `omitnil` to skip
// them in that case, or with `required` to insist on a value. A bare rule
// such as `max=200` fails for an omitted field.
type Field[T any] struct {
	value T
	set   bool // the key was present
	null  bool // and its value was null
}

// FieldOf returns a Field holding v.
func FieldOf[T any](v T) Field[T] { return Field[T]{value: v, set: true} }

// NullField returns a Field that is explicitly null.
func NullField[T any]() Field[T] { return Field[T]{set: true, null: true} }

// IsOmitted reports whether the field was not sent at all.
func (f Field[T]) IsOmitted() bool { return !f.set }

// IsNull reports whether the field was sent as null.
func (f Field[T]) IsNull() bool { return f.set && f.null }

// Get returns the value and true if the field was sent with a value.
func (f Field[T]) Get() (T, bool) { return f.value, f.set && !f.null }

// OrElse resolves the field against the current value, the way a PATCH
// applies: the sent value if there is one, the zero value if the field was
// null, and current if it was omitted.
func (f Field[T]) OrElse(current T) T {
	switch {
	case !f.set:
		return current
	case f.null:
		var zero T
		return zero
	}
	return f.value
}

// IsZero reports whether the field is omitted. It is what the `omitzero`
// JSON option consults.
func (f Field[T]) IsZero() bool { return !f.set }

// UnmarshalJSONFrom implements json.UnmarshalerFrom.
func (f *Field[T]) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	var zero T
	f.set, f.null, f.value = true, false, zero
	if dec.PeekKind() == 'n' {
		f.null = true
		return dec.SkipValue()
	}
	return json.UnmarshalDecode(dec, &f.value)
}

// MarshalJSONTo implements json.MarshalerTo.
func (f Field[T]) MarshalJSONTo(enc *jsontext.Encoder) error {
	if !f.set || f.null {
		return enc.WriteToken(jsontext.Null)
	}
	return json.MarshalEncode(enc, f.value)
}

// validationPointer is what the validator checks in place of the Field: a
// pointer to the value, or a nil pointer when there is no value.
func (f Field[T]) validationPointer() any {
	if !f.set || f.null {
		return (*T)(nil)
	}
	return &f.value
}
