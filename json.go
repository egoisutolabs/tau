package tau

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"math"
)

// InputKind says what was wrong with a piece of JSON input.
type InputKind string

const (
	// InputMalformed: not valid JSON. Syntax errors, duplicate keys, invalid
	// UTF-8 and data after the value all count.
	InputMalformed InputKind = "malformed"
	// InputUnknownField: an object has a key the target type does not.
	InputUnknownField InputKind = "unknown_field"
	// InputWrongType: a value has the wrong JSON type for its field, such as
	// a string where a number is expected.
	InputWrongType InputKind = "wrong_type"
	// InputInvalid: the JSON decoded, but failed validation. The cause is a
	// *ValidationError for `validate` tags, or whatever a Validate() method
	// returned.
	InputInvalid InputKind = "invalid"
	// InputTooLarge: ReadJSON's byte limit was exceeded.
	InputTooLarge InputKind = "too_large"
)

// InputError is thrown by ParseJSON and ReadJSON for every failure that is
// the fault of the input, so a request handler can recognise a bad request
// with one check:
//
//	if bad, ok := errors.AsType[*tau.InputError](err); ok {
//		http.Error(w, bad.Error(), http.StatusBadRequest)
//	}
//
// It wraps the underlying error, which errors.Is and errors.As still reach.
// A failure to read from ReadJSON's reader is not an InputError.
type InputError struct {
	Kind InputKind
	// Path is the location of the problem as a JSON Pointer such as
	// "/user/age", when the decoder reports one. It is empty for validation
	// failures, whose fields are listed in the wrapped *ValidationError.
	Path string
	// Err is the underlying error.
	Err error

	msg string
}

func (e *InputError) Error() string { return e.msg }
func (e *InputError) Unwrap() error { return e.Err }

// decodeError classifies an error from the JSON decoder.
func decodeError(target any, err error) *InputError {
	in := &InputError{Kind: InputMalformed, Err: err, msg: fmt.Sprintf("tau: parse %T: %v", target, err)}
	if syntactic, ok := errors.AsType[*jsontext.SyntacticError](err); ok {
		in.Path = string(syntactic.JSONPointer)
	}
	if semantic, ok := errors.AsType[*json.SemanticError](err); ok {
		in.Kind, in.Path = InputWrongType, string(semantic.JSONPointer)
		if errors.Is(err, json.ErrUnknownName) {
			in.Kind = InputUnknownField
		}
	}
	return in
}

// ParseJSON decodes data into a T and throws an *InputError if it cannot. It
// is strict: unknown fields, duplicate keys, invalid UTF-8 and trailing data
// are all errors. The result is then checked with Validate, so `validate`
// struct tags and a Validate() error method on T (or *T) both apply.
func ParseJSON[T any](data []byte) T {
	var v T
	if err := json.Unmarshal(data, &v, json.RejectUnknownMembers(true)); err != nil {
		Throw(decodeError(v, err))
	}
	if err := Try(func() { Validate(&v) }); err != nil {
		Throw(&InputError{Kind: InputInvalid, Err: err, msg: fmt.Sprintf("tau: invalid %T: %v", v, err)})
	}
	return v
}

// ErrTooLarge is the cause of the InputError that ReadJSON throws when the
// input exceeds its byte limit.
var ErrTooLarge = errors.New("tau: input is larger than the limit")

// ReadJSON reads at most maxBytes from r and parses them like ParseJSON:
// strictly, then validated. If r holds more than maxBytes it throws an
// *InputError matching ErrTooLarge, without reading the rest. It also throws
// if maxBytes is not positive, and throws r's own error if reading fails.
//
//	req := tau.ReadJSON[Request](r.Body, 1<<20) // at most 1 MiB
func ReadJSON[T any](r io.Reader, maxBytes int64) T {
	if maxBytes <= 0 {
		Throwf("tau: ReadJSON needs a positive byte limit, got %d", maxBytes)
	}
	// One byte past the limit is enough to tell "exactly at" from "over".
	data := Must(io.ReadAll(io.LimitReader(r, min(maxBytes, math.MaxInt64-1)+1)))
	if int64(len(data)) > maxBytes {
		err := fmt.Errorf("%w of %d bytes", ErrTooLarge, maxBytes)
		Throw(&InputError{Kind: InputTooLarge, Err: err, msg: err.Error()})
	}
	return ParseJSON[T](data)
}
