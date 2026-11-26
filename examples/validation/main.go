// Strict JSON parsing, validation tags, and reading loose JSON.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/egoisutolabs/tau"
)

type Address struct {
	City string `json:"city" validate:"required"`
}

type Signup struct {
	Email   string   `json:"email" validate:"required,email"`
	Age     int      `json:"age" validate:"gte=18,lte=120"`
	Plan    string   `json:"plan" validate:"oneof=free pro"`
	Tags    []string `json:"tags" validate:"max=3"`
	Address Address  `json:"address"`
	Coupon  *string  `json:"coupon"`
}

// Validate holds a rule that tags cannot express. It runs after the tags.
func (s Signup) Validate() error {
	if s.Plan == "free" && s.Coupon != nil {
		return errors.New("coupons need a paid plan")
	}
	return nil
}

// parse is a boundary: it returns a plain error.
func parse(body string) (Signup, error) {
	return tau.TryValue(func() Signup { return tau.ParseJSON[Signup]([]byte(body)) })
}

func main() {
	good := `{"email":"ada@example.com","age":36,"plan":"pro","tags":["go"],"address":{"city":"London"}}`
	s, err := parse(good)
	check(err == nil && s.Address.City == "London" && tau.Deref(s.Coupon, "none") == "none", "good: %+v, %v", s, err)

	// Every broken rule is reported, by JSON path.
	_, err = parse(`{"email":"nope","age":12,"plan":"gold","tags":[],"address":{}}`)
	invalid, ok := errors.AsType[*tau.ValidationError](err)
	check(ok && len(invalid.Fields) == 4, "want 4 failed fields, got %v", err)
	failed := tau.List[tau.FieldError](invalid.Fields).Map(func(f tau.FieldError) string { return f.Field + ":" + f.Rule })
	check(strings.Join(failed, " ") == "email:email age:gte plan:oneof address.city:required", "failed %v", failed)

	// Strict decoding: a misspelt field is an error, not silently dropped.
	_, err = parse(strings.Replace(good, `"plan"`, `"paln"`, 1))
	check(err != nil && strings.Contains(err.Error(), "paln"), "unknown field: %v", err)

	// The Validate method runs after the tags pass.
	_, err = parse(strings.Replace(good, `"plan":"pro"`, `"plan":"free","coupon":"X"`, 1))
	check(err != nil && strings.Contains(err.Error(), "coupons need a paid plan"), "custom rule: %v", err)

	// Dig reads untyped JSON without a chain of type assertions.
	var payload any
	check(json.Unmarshal([]byte(`{"data":{"users":[{"name":"Ada","age":36}]}}`), &payload) == nil, "decode")
	name := tau.DigOr(payload, "?", "data", "users", 0, "name")
	age, found := tau.Dig[int](payload, "data", "users", 0, "age")
	missing := tau.DigOr(payload, "unknown", "data", "users", 5, "name")
	check(name == "Ada" && found && age == 36 && missing == "unknown", "Dig: %q %d %q", name, age, missing)

	fmt.Println("valid signup from", s.Address.City)
	for _, f := range invalid.Fields {
		fmt.Println("invalid:", f)
	}
}

func check(ok bool, format string, args ...any) {
	if !ok {
		fmt.Fprintf(os.Stderr, "FAIL: "+format+"\n", args...)
		os.Exit(1)
	}
}
