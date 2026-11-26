package tau

import (
	"errors"
	"strings"
	"testing"
)

type address struct {
	City string `json:"city" validate:"required"`
}

type signup struct {
	Email   string   `json:"email" validate:"required,email"`
	Age     int      `json:"age" validate:"gte=18,lte=120"`
	Tags    []string `json:"tags" validate:"max=3,dive,min=2"`
	Plan    string   `json:"plan" validate:"oneof=free pro"`
	Address address  `json:"address"`
	Coupon  string   `json:"coupon"`
}

// A rule that tags cannot express.
func (s signup) Validate() error {
	if s.Plan == "free" && s.Coupon != "" {
		return errors.New("coupons need a paid plan")
	}
	return nil
}

const goodSignup = `{"email":"ada@example.com","age":36,"tags":["go","ts"],"plan":"pro","address":{"city":"London"}}`

func TestParseJSONAppliesValidateTags(t *testing.T) {
	if s := ParseJSON[signup]([]byte(goodSignup)); s.Email != "ada@example.com" || s.Address.City != "London" {
		t.Fatalf("got %+v", s)
	}
	_, err := TryValue(func() signup {
		return ParseJSON[signup]([]byte(`{"email":"nope","age":12,"tags":["go","x"],"plan":"gold","address":{}}`))
	})
	ve, ok := errors.AsType[*ValidationError](err)
	if !ok {
		t.Fatalf("want a *ValidationError, got %T: %v", err, err)
	}
	got := map[string]string{}
	for _, f := range ve.Fields {
		got[f.Field] = f.Rule + "=" + f.Param
	}
	want := map[string]string{"email": "email=", "age": "gte=18", "tags[1]": "min=2", "plan": "oneof=free pro", "address.city": "required="}
	for field, rule := range want {
		if got[field] != rule {
			t.Errorf("field %q: got rule %q, want %q (all: %v)", field, got[field], rule, got)
		}
	}
	if len(got) != len(want) {
		t.Errorf("got %d failures, want %d: %v", len(got), len(want), got)
	}
	if msg := err.Error(); !strings.Contains(msg, "age must satisfy gte=18 (got 12)") || !strings.HasPrefix(msg, "tau: invalid tau.signup: ") {
		t.Fatalf("unhelpful message: %s", msg)
	}
}

func TestValidateMethodRunsAfterTags(t *testing.T) {
	body := strings.Replace(goodSignup, `"plan":"pro"`, `"plan":"free","coupon":"X"`, 1)
	_, err := TryValue(func() signup { return ParseJSON[signup]([]byte(body)) })
	if err == nil || !strings.Contains(err.Error(), "coupons need a paid plan") {
		t.Fatalf("got %v", err)
	}
	if _, isTagFailure := errors.AsType[*ValidationError](err); isTagFailure {
		t.Fatal("a Validate() method's error is not a tag failure")
	}
}

func TestValidateDirectly(t *testing.T) {
	good := signup{Email: "a@b.co", Age: 30, Plan: "pro", Address: address{"Rome"}}
	if err := Try(func() { Validate(good) }); err != nil {
		t.Fatalf("got %v", err)
	}
	if err := Try(func() { Validate(&good) }); err != nil {
		t.Fatalf("pointer: got %v", err)
	}
	bad := good
	bad.Age = 3
	for name, v := range map[string]any{"value": bad, "pointer": &bad, "slice": []signup{good, bad}, "map": map[string]signup{"x": bad}} {
		err := Try(func() { Validate(v) })
		ve, ok := errors.AsType[*ValidationError](err)
		if !ok || len(ve.Fields) != 1 || ve.Fields[0].Rule != "gte" {
			t.Errorf("%s: got %v", name, err)
		}
	}
	if err := Try(func() { Validate([]signup{good, bad}) }); !strings.Contains(err.Error(), "[1].age") {
		t.Errorf("a failure inside a slice should say which element: %v", err)
	}
	for name, v := range map[string]any{"int": 3, "string": "x", "nil": nil, "nil pointer": (*address)(nil), "untagged struct": struct{ A int }{}} {
		if err := Try(func() { Validate(v) }); err != nil {
			t.Errorf("%s should pass, got %v", name, err)
		}
	}
}

type ptrConfig struct {
	Name string `json:"name" validate:"max=5"`
}

var errNameRequired = errors.New("name is required")

// Declared on the pointer, unlike signup's.
func (c *ptrConfig) Validate() error {
	if c.Name == "" {
		return errNameRequired
	}
	return nil
}

func TestParseJSONIntoAPointer(t *testing.T) {
	if c := ParseJSON[*ptrConfig]([]byte(`{"name":"ok"}`)); c == nil || c.Name != "ok" {
		t.Fatalf("got %+v", c)
	}
	for name, parse := range map[string]func(string) error{
		"value":   func(in string) error { return Try(func() { ParseJSON[ptrConfig]([]byte(in)) }) },
		"pointer": func(in string) error { return Try(func() { ParseJSON[*ptrConfig]([]byte(in)) }) },
	} {
		if err := parse(`{}`); !errors.Is(err, errNameRequired) {
			t.Errorf("%s target: the Validate method did not run: %v", name, err)
		}
		if _, ok := errors.AsType[*ValidationError](parse(`{"name":"too long"}`)); !ok {
			t.Errorf("%s target: the tags were not checked", name)
		}
	}
	if c := ParseJSON[*ptrConfig]([]byte(`null`)); c != nil {
		t.Fatalf("null should decode to a nil pointer, got %+v", c)
	}
	deep := &ptrConfig{}
	if err := Try(func() { Validate(&deep) }); !errors.Is(err, errNameRequired) {
		t.Fatalf("a pointer to a pointer should still be validated: %v", err)
	}
}
