package tau

import (
	"encoding/json"
	"testing"
)

func TestDig(t *testing.T) {
	var data any
	if err := json.Unmarshal([]byte(`{
		"user": {"name": "Ada", "age": 36, "ratio": 1.5, "big": 300, "neg": -2,
			"addresses": [{"city": "London"}, {"city": "Turin"}], "nick": null}
	}`), &data); err != nil {
		t.Fatal(err)
	}
	if v, ok := Dig[string](data, "user", "name"); !ok || v != "Ada" {
		t.Fatalf("got %q, %v", v, ok)
	}
	if v, ok := Dig[string](data, "user", "addresses", -1, "city"); !ok || v != "Turin" {
		t.Fatalf("got %q, %v", v, ok)
	}
	if v, ok := Dig[int](data, "user", "age"); !ok || v != 36 {
		t.Fatalf("got %d, %v", v, ok)
	}
	if v, ok := Dig[float64](data, "user", "ratio"); !ok || v != 1.5 {
		t.Fatalf("got %v, %v", v, ok)
	}
	if list, ok := Dig[[]any](data, "user", "addresses"); !ok || len(list) != 2 {
		t.Fatal("could not dig out a list")
	}
	misses := map[string]bool{}
	_, misses["fractional as int"] = Dig[int](data, "user", "ratio")
	_, misses["overflowing int8"] = Dig[int8](data, "user", "big")
	_, misses["negative as uint"] = Dig[uint](data, "user", "neg")
	_, misses["huge number as int"] = Dig[int](map[string]any{"n": 1e30}, "n")
	_, misses["missing key"] = Dig[string](data, "user", "email")
	_, misses["index out of range"] = Dig[string](data, "user", "addresses", 5, "city")
	_, misses["key on a list"] = Dig[string](data, "user", "addresses", "city")
	_, misses["index on a map"] = Dig[string](data, "user", 0)
	_, misses["wrong type"] = Dig[string](data, "user", "age")
	_, misses["null"] = Dig[string](data, "user", "nick")
	_, misses["path through a scalar"] = Dig[string](data, "user", "name", "first")
	_, misses["bad step type"] = Dig[string](data, 1.5)
	for name, found := range misses {
		if found {
			t.Errorf("%s: expected a miss", name)
		}
	}
	if DigOr(data, "nowhere", "user", "addresses", 9, "city") != "nowhere" || DigOr(data, "", "user", "name") != "Ada" {
		t.Fatal("DigOr wrong")
	}
	if v, ok := Dig[int8](data, "user", "neg"); !ok || v != -2 {
		t.Fatalf("got %d, %v", v, ok)
	}
	if v, ok := Dig[uint16](data, "user", "big"); !ok || v != 300 {
		t.Fatalf("got %d, %v", v, ok)
	}
	type userID int64
	if v, ok := Dig[userID](data, "user", "age"); !ok || v != 36 {
		t.Fatalf("got %d, %v", v, ok)
	}
}
