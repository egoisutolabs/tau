package tau

import (
	"strings"
	"testing"
)

func TestOrderedMap(t *testing.T) {
	m := NewMap[string, int]().Set("c", 3).Set("a", 1).Set("b", 2)
	m.Set("c", 30) // keeps its place
	if got := strings.Join(m.Keys(), ""); got != "cab" {
		t.Fatalf("keys %q, want insertion order", got)
	}
	if v, ok := m.Get("c"); !ok || v != 30 || m.GetOr("z", -1) != -1 || !m.Has("a") || m.Has("z") {
		t.Fatal("Get/GetOr/Has wrong")
	}
	if !m.Delete("a") || m.Delete("a") || m.Len() != 2 {
		t.Fatal("Delete wrong")
	}
	m.Set("a", 100) // re-added keys go to the end
	var order []string
	sum := 0
	for k, v := range m.All() {
		order = append(order, k)
		sum += v
	}
	if strings.Join(order, "") != "cba" || sum != 132 {
		t.Fatalf("got %v, %d", order, sum)
	}
	for k := range m.All() {
		if k == "c" {
			break // stopping early must not panic
		}
	}
	m.Delete("c")
	m.Delete("a")
	if got := m.Values(); len(got) != 1 || got[0] != 2 {
		t.Fatalf("got %v", got)
	}
	m.Delete("b")
	m.Set("x", 1)
	if m.Len() != 1 || m.Keys()[0] != "x" {
		t.Fatal("map broken after emptying it")
	}
	m.Clear()
	if m.Len() != 0 || len(m.Keys()) != 0 {
		t.Fatal("Clear wrong")
	}
	var zero OrderedMap[int, string]
	if zero.Has(1) || zero.Len() != 0 || zero.Delete(1) {
		t.Fatal("zero value should behave as empty")
	}
	if zero.Set(1, "a").GetOr(1, "") != "a" {
		t.Fatal("zero value should be usable")
	}
}

func TestSet(t *testing.T) {
	s := NewSet(3, 1, 3, 2, 1)
	if got := s.Values(); len(got) != 3 || got[0] != 3 || got[1] != 1 || got[2] != 2 {
		t.Fatalf("got %v, want distinct values in insertion order", got)
	}
	if !s.Has(2) || s.Has(9) || !s.Delete(1) || s.Delete(1) || s.Len() != 2 {
		t.Fatal("Has/Delete/Len wrong")
	}
	a, b := NewSet(1, 2, 3), NewSet(3, 4)
	check := func(name string, got *Set[int], want ...int) {
		t.Helper()
		vals := got.Values()
		if len(vals) != len(want) {
			t.Fatalf("%s: got %v, want %v", name, vals, want)
		}
		for i := range want {
			if vals[i] != want[i] {
				t.Fatalf("%s: got %v, want %v", name, vals, want)
			}
		}
	}
	check("union", a.Union(b), 1, 2, 3, 4)
	check("intersection", a.Intersection(b), 3)
	check("difference", a.Difference(b), 1, 2)
	check("inputs untouched", a, 1, 2, 3)
	if !NewSet(1, 3).IsSubsetOf(a) || b.IsSubsetOf(a) || !NewSet[int]().IsSubsetOf(a) {
		t.Fatal("IsSubsetOf wrong")
	}
	total := 0
	for v := range a.All() {
		total += v
	}
	var zero Set[string]
	if total != 6 || zero.Has("x") || zero.Add("x").Len() != 1 {
		t.Fatal("All or zero value wrong")
	}
}
