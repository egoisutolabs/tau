package tau

import (
	"encoding/json/v2"
	"errors"
	"io"
	"math"
	"strings"
	"testing"
)

type user struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func (u user) Validate() error {
	if u.Name == "" {
		return errors.New("name is required")
	}
	return nil
}

func TestParseJSON(t *testing.T) {
	u := ParseJSON[user]([]byte(`{"name":"Ada","age":36}`))
	if u.Name != "Ada" || u.Age != 36 {
		t.Fatalf("got %+v", u)
	}
	bad := map[string]string{
		"wrong type":    `{"name":"Ada","age":"old"}`,
		"unknown field": `{"name":"Ada","nmae":"x"}`,
		"trailing data": `{"name":"Ada"} {}`,
		"validation":    `{"age":3}`,
		"not json":      `nope`,
		"duplicate key": `{"name":"Ada","name":"Bob"}`,
	}
	for name, in := range bad {
		if _, err := TryValue(func() user { return ParseJSON[user]([]byte(in)) }); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

// failAfter yields its data and then a read error, like a dropped connection.
type failAfter struct {
	data string
	err  error
}

func (f *failAfter) Read(p []byte) (int, error) {
	if f.data == "" {
		return 0, f.err
	}
	n := copy(p, f.data)
	f.data = f.data[n:]
	return n, nil
}

func TestReadJSON(t *testing.T) {
	const body = `{"name":"Ada","age":36}` // 23 bytes
	if u := ReadJSON[user](strings.NewReader(body), 1024); u.Name != "Ada" {
		t.Fatalf("got %+v", u)
	}
	if u := ReadJSON[user](strings.NewReader(body), int64(len(body))); u.Age != 36 {
		t.Fatalf("a body exactly at the limit should be accepted, got %+v", u)
	}
	err := Try(func() { ReadJSON[user](strings.NewReader(body), int64(len(body))-1) })
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("one byte over the limit: got %v", err)
	}

	// It must not read an oversized body to the end.
	huge := &countingReader{remaining: 1 << 30}
	if err := Try(func() { ReadJSON[user](huge, 1000) }); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("got %v", err)
	}
	if huge.read > 1<<20 {
		t.Fatalf("read %d bytes of an oversized body with a 1000-byte limit", huge.read)
	}

	for name, in := range map[string]string{"unknown field": `{"name":"Ada","nmae":"x"}`, "validation": `{"age":3}`, "not json": `nope`} {
		if err := Try(func() { ReadJSON[user](strings.NewReader(in), 1024) }); err == nil || errors.Is(err, ErrTooLarge) {
			t.Errorf("%s: want a parse or validation error, got %v", name, err)
		}
	}
	if err := Try(func() { ReadJSON[user](&failAfter{`{"name":`, errBoom}, 1024) }); !errors.Is(err, errBoom) {
		t.Fatalf("a read error should be thrown as it is, got %v", err)
	}
	for _, limit := range []int64{0, -5} {
		if err := Try(func() { ReadJSON[user](strings.NewReader(body), limit) }); err == nil {
			t.Errorf("limit %d should be refused", limit)
		}
	}
	if u := ReadJSON[user](strings.NewReader(body), math.MaxInt64); u.Name != "Ada" {
		t.Fatal("the largest limit should not overflow")
	}
}

type countingReader struct{ remaining, read int }

func (c *countingReader) Read(p []byte) (int, error) {
	if c.remaining == 0 {
		return 0, io.EOF
	}
	n := min(len(p), c.remaining)
	for i := range p[:n] {
		p[i] = ' '
	}
	c.remaining -= n
	c.read += n
	return n, nil
}

func TestInputErrorKinds(t *testing.T) {
	cases := map[string]struct {
		body string
		kind InputKind
		path string
	}{
		"syntax error":   {`{"name":`, InputMalformed, ""},
		"not json":       {`nope`, InputMalformed, ""},
		"duplicate key":  {`{"name":"a","name":"b"}`, InputMalformed, "/name"},
		"trailing data":  {`{"name":"a"} {}`, InputMalformed, ""},
		"unknown field":  {`{"name":"a","nmae":"x"}`, InputUnknownField, "/nmae"},
		"wrong type":     {`{"name":"a","age":"old"}`, InputWrongType, "/age"},
		"fails Validate": {`{"age":3}`, InputInvalid, ""},
	}
	for name, c := range cases {
		err := Try(func() { ParseJSON[user]([]byte(c.body)) })
		in, ok := errors.AsType[*InputError](err)
		if !ok {
			t.Errorf("%s: want an *InputError, got %T: %v", name, err, err)
			continue
		}
		if in.Kind != c.kind {
			t.Errorf("%s: kind %q, want %q (%v)", name, in.Kind, c.kind, err)
		}
		if c.path != "" && in.Path != c.path {
			t.Errorf("%s: path %q, want %q", name, in.Path, c.path)
		}
		if in.Err == nil || !strings.HasPrefix(err.Error(), "tau: ") {
			t.Errorf("%s: the cause or message is missing: %v", name, err)
		}
	}
}

func TestInputErrorKeepsItsCause(t *testing.T) {
	// A tag failure is still reachable as a *ValidationError.
	err := Try(func() { ParseJSON[signup]([]byte(`{"email":"nope","age":30,"plan":"pro","address":{"city":"x"}}`)) })
	in, isInput := errors.AsType[*InputError](err)
	ve, isValidation := errors.AsType[*ValidationError](err)
	if !isInput || in.Kind != InputInvalid || !isValidation || ve.Fields[0].Field != "email" {
		t.Fatalf("got %v", err)
	}
	// So is the decoder's own error, and the size sentinel.
	err = Try(func() { ParseJSON[user]([]byte(`{"nmae":"x"}`)) })
	if !errors.Is(err, json.ErrUnknownName) {
		t.Fatalf("the decoder's error was lost: %v", err)
	}
	err = Try(func() { ReadJSON[user](strings.NewReader(`{"name":"Ada"}`), 3) })
	if in, ok := errors.AsType[*InputError](err); !ok || in.Kind != InputTooLarge || !errors.Is(err, ErrTooLarge) {
		t.Fatalf("got %v", err)
	}
	// A reader that fails is the reader's fault, not the input's.
	err = Try(func() { ReadJSON[user](&failAfter{`{"name":`, errBoom}, 1024) })
	if _, ok := errors.AsType[*InputError](err); ok || !errors.Is(err, errBoom) {
		t.Fatalf("a read failure must not be reported as bad input: %v", err)
	}
	// Validate called directly is not about input.
	err = Try(func() { Validate(signup{}) })
	if _, ok := errors.AsType[*InputError](err); ok {
		t.Fatal("Validate on its own should throw a plain ValidationError")
	}
}
