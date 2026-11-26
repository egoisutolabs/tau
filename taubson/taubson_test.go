package taubson

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/egoisutolabs/tau"
	"github.com/egoisutolabs/tau/internal/dighook"
)

type address struct {
	City string `bson:"city"`
}

type user struct {
	ID        bson.ObjectID  `bson:"_id"`
	Name      string         `bson:"name"`
	Age       int            `bson:"age"`
	Addresses []address      `bson:"addresses"`
	Meta      map[string]any `bson:"meta"`
	Manager   *user          `bson:"manager"`
}

// decoded returns one document as the driver decodes it into a bson.M and
// into a bson.D. Driver v2 nests bson.D and bson.A inside both.
func decoded(t testing.TB) (bson.ObjectID, bson.M, bson.D) {
	t.Helper()
	id := bson.NewObjectID()
	raw, err := bson.Marshal(user{
		ID: id, Name: "Ada", Age: 36,
		Addresses: []address{{City: "London"}, {City: "Turin"}},
		Meta:      map[string]any{"logins": int64(3_000_000_000), "score": 9.5, "tags": []string{"a", "b"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var m bson.M
	var d bson.D
	for _, into := range []any{&m, &d} {
		if err := bson.Unmarshal(raw, into); err != nil {
			t.Fatal(err)
		}
	}
	return id, m, d
}

// withoutFastPath runs fn with the hooks removed, so Dig uses reflection.
func withoutFastPath(fn func()) {
	key, index := dighook.Key, dighook.Index
	dighook.Key, dighook.Index = nil, nil
	defer func() { dighook.Key, dighook.Index = key, index }()
	fn()
}

func expect[T comparable](t *testing.T, label string, data any, want T, wantOK bool, path ...any) {
	t.Helper()
	got, ok := tau.Dig[T](data, path...)
	if ok != wantOK || (ok && got != want) {
		t.Errorf("%s: got (%v, %v), want (%v, %v)", label, got, ok, want, wantOK)
	}
}

// The same lookups must give the same answers through the registered fast
// path and through tau's reflection.
func TestDigOnDriverDocuments(t *testing.T) {
	id, m, d := decoded(t)
	if _, nested := m["addresses"].(bson.A)[0].(bson.D); !nested {
		t.Fatalf("expected the driver to nest bson.D, got %T", m["addresses"].(bson.A)[0])
	}
	lookups := func(t *testing.T) {
		for name, doc := range map[string]any{"bson.M": m, "bson.D": d, "*bson.M": &m} {
			expect(t, name+" name", doc, "Ada", true, "name")
			expect(t, name+" addresses[-1].city", doc, "Turin", true, "addresses", -1, "city")
			expect(t, name+" addresses[0].city", doc, "London", true, "addresses", 0, "city")
			expect(t, name+" meta.tags[1]", doc, "b", true, "meta", "tags", 1)
			expect(t, name+" age as int", doc, 36, true, "age")
			expect(t, name+" age as float64", doc, 36.0, true, "age")
			expect(t, name+" logins as int", doc, 3_000_000_000, true, "meta", "logins")
			expect(t, name+" logins as int32", doc, int32(0), false, "meta", "logins")
			expect(t, name+" score as int", doc, 0, false, "meta", "score")
			expect(t, name+" _id", doc, id, true, "_id")
			expect(t, name+" null manager", doc, "", false, "manager", "name")
			expect(t, name+" missing key", doc, "", false, "email")
			expect(t, name+" index out of range", doc, "", false, "addresses", 2, "city")
			expect(t, name+" key on a list", doc, "", false, "addresses", "city")
			expect(t, name+" index on a document's key", doc, "", false, "meta", 0)
		}
		// A driver document inside plain JSON data and inside a struct.
		expect(t, "inside map[string]any", map[string]any{"doc": m}, "Ada", true, "doc", "name")
		expect(t, "inside []any", []any{d}, "Ada", true, 0, "name")
		expect(t, "inside a struct", struct{ Doc bson.M }{m}, "Turin", true, "Doc", "addresses", 1, "city")
		expect(t, "plain JSON still works", map[string]any{"a": []any{"x"}}, "x", true, "a", 0)
	}
	t.Run("fast path", lookups)
	t.Run("reflection", func(t *testing.T) { withoutFastPath(func() { lookups(t) }) })
}

func TestImportRegistersTheFastPath(t *testing.T) {
	if dighook.Key == nil || dighook.Index == nil {
		t.Fatal("importing taubson did not register its hooks")
	}
	if _, _, handled := dighook.Key(map[string]string{}, "a"); handled {
		t.Error("the key hook claimed a type that is not the driver's")
	}
	if _, _, handled := dighook.Index([]string{}, 0); handled {
		t.Error("the index hook claimed a type that is not the driver's")
	}
}

func BenchmarkDigBsonM(b *testing.B) {
	_, m, _ := decoded(b)
	b.Run("fast path", func(b *testing.B) {
		for b.Loop() {
			if _, ok := tau.Dig[string](m, "addresses", 1, "city"); !ok {
				b.Fatal("miss")
			}
		}
	})
	b.Run("reflection", func(b *testing.B) {
		withoutFastPath(func() {
			for b.Loop() {
				if _, ok := tau.Dig[string](m, "addresses", 1, "city"); !ok {
					b.Fatal("miss")
				}
			}
		})
	})
}
