package tau

import (
	"encoding/json/v2"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type profileUpdate struct {
	Bio  Field[string]   `json:"bio,omitzero" validate:"omitnil,max=10"`
	Age  Field[int]      `json:"age,omitzero" validate:"omitnil,gte=18"`
	Tags Field[[]string] `json:"tags,omitzero" validate:"omitnil,max=2,dive,min=2"`
}

func TestFieldThreeStates(t *testing.T) {
	u := ParseJSON[profileUpdate]([]byte(`{"bio":null,"age":30}`))
	if !u.Bio.IsNull() || u.Bio.IsOmitted() {
		t.Fatal("bio was sent as null")
	}
	if v, ok := u.Bio.Get(); ok || v != "" {
		t.Fatalf("a null field has no value, got %q, %v", v, ok)
	}
	if v, ok := u.Age.Get(); !ok || v != 30 || u.Age.IsNull() || u.Age.IsOmitted() {
		t.Fatalf("age was sent as 30, got %d, %v", v, ok)
	}
	if !u.Tags.IsOmitted() || u.Tags.IsNull() {
		t.Fatal("tags was not sent")
	}
	// A value that happens to be the zero value is still a value.
	empty := ParseJSON[profileUpdate]([]byte(`{"bio":"","tags":[]}`))
	if v, ok := empty.Bio.Get(); !ok || v != "" || empty.Bio.IsNull() || empty.Bio.IsOmitted() {
		t.Fatal(`"" is a value, not null or omitted`)
	}
	if v, ok := empty.Tags.Get(); !ok || v == nil || len(v) != 0 {
		t.Fatalf("[] is a value, got %v, %v", v, ok)
	}
}

func TestFieldOrElseAppliesAPatch(t *testing.T) {
	const current = "old bio"
	for body, want := range map[string]string{`{}`: "old bio", `{"bio":null}`: "", `{"bio":"new"}`: "new", `{"bio":""}`: ""} {
		if got := ParseJSON[profileUpdate]([]byte(body)).Bio.OrElse(current); got != want {
			t.Errorf("%s: got %q, want %q", body, got, want)
		}
	}
	if FieldOf(5).OrElse(1) != 5 || NullField[int]().OrElse(1) != 0 || (Field[int]{}).OrElse(1) != 1 {
		t.Fatal("constructors wrong")
	}
}

func TestFieldRoundTrips(t *testing.T) {
	for _, body := range []string{`{}`, `{"bio":null}`, `{"bio":"hi","age":30}`, `{"bio":"","tags":[]}`, `{"tags":["go","ts"]}`} {
		out := Must(json.Marshal(ParseJSON[profileUpdate]([]byte(body))))
		if string(out) != body {
			t.Errorf("%s encoded back as %s", body, out)
		}
	}
	// Without omitzero an omitted field cannot be left out, so it is null.
	type noOmit struct {
		Bio Field[string] `json:"bio"`
	}
	if out := Must(json.Marshal(noOmit{})); string(out) != `{"bio":null}` {
		t.Fatalf("got %s", out)
	}
	if out := Must(json.Marshal(noOmit{FieldOf("x")})); string(out) != `{"bio":"x"}` {
		t.Fatalf("got %s", out)
	}
}

func TestFieldDecodingIsStillStrict(t *testing.T) {
	err := Try(func() { ParseJSON[profileUpdate]([]byte(`{"age":"thirty"}`)) })
	if in, ok := errors.AsType[*InputError](err); !ok || in.Kind != InputWrongType || in.Path != "/age" {
		t.Fatalf("a wrong type inside a Field should be reported with its path, got %+v (%v)", in, err)
	}
	// Decoding into a Field that already holds something replaces it entirely.
	u := profileUpdate{Bio: FieldOf("stale")}
	Check(json.Unmarshal([]byte(`{"bio":null}`), &u))
	if v, ok := u.Bio.Get(); ok || v != "" || !u.Bio.IsNull() {
		t.Fatal("a reused Field kept its old value")
	}
}

func TestFieldValidation(t *testing.T) {
	failed := func(body string) string {
		err := Try(func() { ParseJSON[profileUpdate]([]byte(body)) })
		if err == nil {
			return ""
		}
		ve, ok := errors.AsType[*ValidationError](err)
		if !ok {
			t.Fatalf("%s: want a ValidationError, got %v", body, err)
		}
		return strings.Join(List[FieldError](ve.Fields).Map(func(f FieldError) string { return f.Field + ":" + f.Rule }), " ")
	}
	for body, want := range map[string]string{
		`{}`:                         "",
		`{"bio":null,"age":null}`:    "",
		`{"bio":"short","age":18}`:   "",
		`{"bio":""}`:                 "", // a present empty string passes max=10
		`{"bio":"far too long bio"}`: "bio:max",
		`{"age":12}`:                 "age:gte",
		`{"tags":["go","ts","rs"]}`:  "tags:max",
		`{"tags":["go","x"]}`:        "tags[1]:min",
	} {
		if got := failed(body); got != want {
			t.Errorf("%s: failed %q, want %q", body, got, want)
		}
	}

	type mustSend struct {
		Name Field[string] `json:"name,omitzero" validate:"required,min=2"`
	}
	for body, wantRule := range map[string]string{`{}`: "required", `{"name":null}`: "required", `{"name":"a"}`: "min", `{"name":"ab"}`: ""} {
		err := Try(func() { ParseJSON[mustSend]([]byte(body)) })
		ve, _ := errors.AsType[*ValidationError](err)
		got := ""
		if ve != nil {
			got = ve.Fields[0].Rule
		}
		if got != wantRule {
			t.Errorf("required: %s failed %q, want %q (%v)", body, got, wantRule, err)
		}
	}
}

type nestedUpdate struct {
	Profile profileUpdate   `json:"profile"`
	Others  []profileUpdate `json:"others" validate:"dive"` // the validator only enters a slice when told to
}

func TestFieldValidationWhenNested(t *testing.T) {
	err := Try(func() {
		ParseJSON[nestedUpdate]([]byte(`{"profile":{"age":3},"others":[{},{"bio":"far too long bio"}]}`))
	})
	ve, ok := errors.AsType[*ValidationError](err)
	if !ok || len(ve.Fields) != 2 || ve.Fields[0].Field != "profile.age" || ve.Fields[1].Field != "others[1].bio" {
		t.Fatalf("got %v", err)
	}
}

// Each of these types is validated for the first time from several goroutines
// at once; run with -race.
func TestFieldTypesAreRegisteredSafely(t *testing.T) {
	type a struct {
		X Field[int8] `validate:"omitnil,gte=1"`
	}
	type b struct {
		Y Field[uint16] `validate:"omitnil,gte=1"`
	}
	type c struct {
		Z Field[float32] `validate:"omitnil,gte=1"`
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for _, v := range []any{a{FieldOf[int8](0)}, b{FieldOf[uint16](0)}, c{FieldOf[float32](0)}} {
				if err := Try(func() { Validate(v) }); err == nil {
					t.Errorf("%T: the rule inside the Field was not applied", v)
				}
			}
			if err := Try(func() { Validate(a{}) }); err != nil {
				t.Errorf("an omitted field should pass: %v", err)
			}
		})
	}
	wg.Wait()
}

type lazyOuter struct{ Inner lazyInner }

type lazyInner struct {
	N Field[int64] `validate:"omitnil,gte=1"`
}

// A Validate method that itself validates a type not seen before.
func (o lazyOuter) Validate() error { return Try(func() { Validate(struct{ V lazyInner }{o.Inner}) }) }

func TestValidateMethodMayValidateNewTypes(t *testing.T) {
	done := make(chan error, 1)
	go func() { done <- Try(func() { Validate(lazyOuter{lazyInner{FieldOf[int64](0)}}) }) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected the inner rule to fail")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Validate deadlocked when a Validate method validated a new type")
	}
}
