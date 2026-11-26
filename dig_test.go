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

// The shapes of MongoDB's bson.M, bson.A, bson.E and bson.D, declared here so
// the root package is tested without importing the driver. The driver's own
// types are tested in the taubson package.
type (
	digDoc   map[string]any
	digList  []any
	digEntry struct {
		Key   string
		Value any
	}
	digPairList []digEntry
)

type digAddress struct {
	City string  `json:"city" bson:"town"`
	Zip  *string `json:"zip,omitempty"`
}

type digAudit struct {
	Version int32 `bson:"v"`
}

type digUser struct {
	ID        string         `json:"id" bson:"_id"`
	Name      string         `json:"name"`
	Addresses []digAddress   `json:"addresses"`
	Meta      map[string]any `json:"meta"`
	Manager   *digUser       `json:"manager"`
	Hidden    string         `json:"-"`
	Nickname  string
	digAudit
	*digExtra
	secret string
}

type digExtra struct{ Note string }

// expectDig checks one lookup; want is ignored when the lookup should miss.
func expectDig[T comparable](t *testing.T, label string, data any, want T, wantOK bool, path ...any) {
	t.Helper()
	got, ok := Dig[T](data, path...)
	if ok != wantOK || (ok && got != want) {
		t.Errorf("%s: got (%v, %v), want (%v, %v)", label, got, ok, want, wantOK)
	}
}

func TestDigNamedDocumentTypes(t *testing.T) {
	data := digDoc{
		"name": "Ada",
		"age":  int32(36),
		"addresses": digList{
			digPairList{{"city", "London"}},
			digPairList{{"city", "Turin"}, {"floor", int64(3)}},
		},
		"manager": nil,
	}
	expectDig(t, "named map", data, "Ada", true, "name")
	expectDig(t, "named list, pairs", data, "Turin", true, "addresses", -1, "city")
	expectDig(t, "pairs, later key", data, 3, true, "addresses", 1, "floor")
	expectDig(t, "pairs, missing key", data, "", false, "addresses", 0, "floor")
	expectDig(t, "index into pairs is the pair", data, digEntry{"city", "London"}, true, "addresses", 0, 0)
	expectDig(t, "nil value", data, "", false, "manager", "name")
	expectDig(t, "missing key", data, "", false, "email")
	expectDig(t, "index out of range", data, "", false, "addresses", 2, "city")
	expectDig(t, "key on a list", data, "", false, "addresses", "city")
	expectDig(t, "index on a map", data, "", false, 0)

	top := digPairList{{"user", data}}
	expectDig(t, "pairs at the top", top, "Ada", true, "user", "name")
	expectDig(t, "pointer to a document", &data, "Ada", true, "name")
	expectDig(t, "the document itself", data, int32(36), true, "age")
}

func TestDigTypedMapsAndSlices(t *testing.T) {
	type label string
	expectDig(t, "map[string]string", map[string]string{"a": "b"}, "b", true, "a")
	expectDig(t, "map[string][]int", map[string][]int{"n": {1, 2, 3}}, 3, true, "n", -1)
	expectDig(t, "named key type", map[label]int{"a": 1}, 1, true, "a")
	expectDig(t, "array", [2]string{"x", "y"}, "y", true, 1)
	expectDig(t, "int-keyed map", map[int]string{1: "b"}, "", false, "1")
	expectDig(t, "nil map", map[string]string(nil), "", false, "a")
	expectDig(t, "nil data", nil, "", false, "a")
	expectDig(t, "slice of non-pairs", []digAddress{{City: "x"}}, "", false, "city")
	var none *digUser
	expectDig(t, "nil pointer", none, "", false, "name")
}

func TestDigStructs(t *testing.T) {
	zip := "10121"
	user := digUser{
		ID: "u1", Name: "Ada", Nickname: "countess", Hidden: "h", secret: "s",
		Addresses: []digAddress{{City: "London"}, {City: "Turin", Zip: &zip}},
		Meta:      map[string]any{"score": 9.5, "tags": []string{"a", "b"}},
		digAudit:  digAudit{Version: 7},
	}
	expectDig(t, "json tag", user, "Ada", true, "name")
	expectDig(t, "json tag wins", user, "u1", true, "id")
	expectDig(t, "bson tag", user, "u1", true, "_id")
	expectDig(t, "bson tag on a nested struct", user, "London", true, "addresses", 0, "town")
	expectDig(t, "Go name", user, "countess", true, "Nickname")
	expectDig(t, "Go name of a tagged field", user, "Ada", true, "Name")
	expectDig(t, "Go name of a json:\"-\" field", user, "h", true, "Hidden")
	expectDig(t, "pointer to struct", &user, "Ada", true, "name")
	expectDig(t, "typed slice", user, "Turin", true, "addresses", -1, "city")
	expectDig(t, "pointer field as its value", user, "10121", true, "addresses", 1, "zip")
	expectDig(t, "pointer field as a pointer", user, &zip, true, "addresses", 1, "zip")
	expectDig(t, "nil pointer field", user, "", false, "addresses", 0, "zip")
	expectDig(t, "through a nil pointer", user, "", false, "manager", "name")
	expectDig(t, "embedded by bson tag", user, 7, true, "v")
	expectDig(t, "embedded by Go name", user, int32(7), true, "Version")
	expectDig(t, "through a nil embedded pointer", user, "", false, "Note")
	expectDig(t, "map inside a struct", user, 9.5, true, "meta", "score")
	expectDig(t, "typed slice inside a map", user, "b", true, "meta", "tags", 1)
	expectDig(t, "unexported field", user, "", false, "secret")
	expectDig(t, "unknown field", user, "", false, "email")
	expectDig(t, "index on a struct", user, "", false, 0)
	expectDig(t, "a whole struct", user, digAddress{City: "London"}, true, "addresses", 0)

	user.digExtra = &digExtra{Note: "n"}
	expectDig(t, "through an embedded pointer", user, "n", true, "Note")
	if DigOr(user, "none", "manager", "name") != "none" {
		t.Error("DigOr on a struct ignored the fallback")
	}
}

func TestDigNumbersConvertOnlyWhenExact(t *testing.T) {
	data := digDoc{
		"i32": int32(36), "i64": int64(3_000_000_000), "neg": int64(-2), "u8": uint8(200),
		"u64": uint64(1 << 63), "f": 9.5, "f32": float32(0.5), "whole": 4.0,
		"huge": int64(1<<53 + 1), "tenth": 0.1, "str": "7", "yes": true,
	}
	type userID int64
	expectDig(t, "int32 as int", data, 36, true, "i32")
	expectDig(t, "int32 as a named int", data, userID(36), true, "i32")
	expectDig(t, "int64 as int", data, 3_000_000_000, true, "i64")
	expectDig(t, "int64 too big for int32", data, int32(0), false, "i64")
	expectDig(t, "negative as uint", data, uint(0), false, "neg")
	expectDig(t, "negative as int8", data, int8(-2), true, "neg")
	expectDig(t, "uint8 as int", data, 200, true, "u8")
	expectDig(t, "uint8 too big for int8", data, int8(0), false, "u8")
	expectDig(t, "uint64 too big for int64", data, int64(0), false, "u64")
	expectDig(t, "uint64 as uint64", data, uint64(1<<63), true, "u64")
	expectDig(t, "int as float64", data, 36.0, true, "i32")
	expectDig(t, "uint as float32", data, float32(200), true, "u8")
	expectDig(t, "int too big to be an exact float64", data, 0.0, false, "huge")
	expectDig(t, "fraction as int", data, 0, false, "f")
	expectDig(t, "whole float as int", data, 4, true, "whole")
	expectDig(t, "float32 as float64", data, 0.5, true, "f32")
	expectDig(t, "float64 as float32, exact", data, float32(9.5), true, "f")
	expectDig(t, "float64 as float32, inexact", data, float32(0), false, "tenth")
	expectDig(t, "string as int", data, 0, false, "str")
	expectDig(t, "bool as int", data, 0, false, "yes")
	expectDig(t, "int as string", data, "", false, "i32")
}

func TestDigAsInterfaceType(t *testing.T) {
	user := digUser{Name: "Ada", Addresses: []digAddress{{City: "London"}}}
	if v, ok := Dig[any](user, "addresses", 0, "city"); !ok || v != "London" {
		t.Errorf("Dig[any] on a struct field: got (%v, %v)", v, ok)
	}
	if _, ok := Dig[error](user, "name"); ok {
		t.Error("a string was returned as an error")
	}
	if _, ok := Dig[any](digDoc{"a": nil}, "a"); ok {
		t.Error("a nil was reported as found")
	}
}

func TestDigConcurrentFirstUseOfAType(t *testing.T) {
	type fresh struct {
		A struct{ B []digEntry }
	}
	data := fresh{}
	data.A.B = []digEntry{{"k", 1}}
	done := make(chan bool)
	for range 8 {
		go func() {
			v, ok := Dig[int](data, "A", "B", "k")
			done <- ok && v == 1
		}()
	}
	for range 8 {
		if !<-done {
			t.Error("concurrent Dig missed")
		}
	}
}

func BenchmarkDigJSON(b *testing.B) {
	var data any
	if err := json.Unmarshal([]byte(`{"addresses":[{"city":"London"},{"city":"Turin"}]}`), &data); err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		if _, ok := Dig[string](data, "addresses", 1, "city"); !ok {
			b.Fatal("miss")
		}
	}
}

func BenchmarkDigStruct(b *testing.B) {
	data := &digUser{Addresses: []digAddress{{City: "London"}, {City: "Turin"}}}
	for b.Loop() {
		if _, ok := Dig[string](data, "addresses", 1, "city"); !ok {
			b.Fatal("miss")
		}
	}
}

type digDeepName struct {
	Name string `json:"name" bson:"n"`
}

// The embedded field is declared first, so a rule of "first seen wins"
// would pick it over the direct field.
type digShadowed struct {
	digDeepName
	Direct string `json:"name"`
	Short  string `bson:"n"`
}

func TestDigShallowestFieldWinsATagName(t *testing.T) {
	v := digShadowed{digDeepName{"nested"}, "direct", "short"}
	expectDig(t, "json name", v, "direct", true, "name")
	expectDig(t, "bson name", v, "short", true, "n")
	expectDig(t, "the embedded field by its Go name", v, "nested", true, "Name")
}

type digPtrErr struct{ msg string }

func (e *digPtrErr) Error() string { return e.msg }

// An interface satisfied only by the pointer must be found the same way in
// a struct field as in a map[string]any.
func TestDigInterfaceImplementedByAPointer(t *testing.T) {
	e := &digPtrErr{"boom"}
	type holder struct {
		Err   *digPtrErr
		Typed map[string]*digPtrErr
		None  *digPtrErr
	}
	h := holder{Err: e, Typed: map[string]*digPtrErr{"k": e}}
	for label, got := range map[string]func() (error, bool){
		"map[string]any": func() (error, bool) { return Dig[error](map[string]any{"Err": e}, "Err") },
		"struct field":   func() (error, bool) { return Dig[error](h, "Err") },
		"typed map":      func() (error, bool) { return Dig[error](h, "Typed", "k") },
		"named document": func() (error, bool) { return Dig[error](digDoc{"Err": e}, "Err") },
	} {
		if err, ok := got(); !ok || err != error(e) {
			t.Errorf("%s: got (%v, %v), want the pointer itself", label, err, ok)
		}
	}
	if _, ok := Dig[error](h, "None"); ok {
		t.Error("a nil pointer was returned as an error")
	}
	if v, ok := Dig[any](h, "Err"); !ok || v != any(e) {
		t.Errorf("Dig[any] on a pointer field: got (%v, %v), want the pointer", v, ok)
	}
	if v, ok := Dig[digPtrErr](h, "Err"); !ok || v.msg != "boom" {
		t.Errorf("the value behind the pointer: got (%v, %v)", v, ok)
	}
}
