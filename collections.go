package tau

import "iter"

type entry[K comparable, V any] struct {
	key        K
	value      V
	prev, next *entry[K, V]
}

// OrderedMap is a map that remembers insertion order, like a JavaScript Map.
// Go's built-in map iterates in random order. The zero value is ready to use;
// an OrderedMap is not safe for concurrent use.
type OrderedMap[K comparable, V any] struct {
	index      map[K]*entry[K, V]
	head, tail *entry[K, V]
}

// NewMap returns an empty ordered map.
func NewMap[K comparable, V any]() *OrderedMap[K, V] { return &OrderedMap[K, V]{} }

// Set stores value under key and returns the map for chaining. Setting an
// existing key keeps its original position.
func (m *OrderedMap[K, V]) Set(key K, value V) *OrderedMap[K, V] {
	if e, ok := m.index[key]; ok {
		e.value = value
		return m
	}
	if m.index == nil {
		m.index = map[K]*entry[K, V]{}
	}
	e := &entry[K, V]{key: key, value: value, prev: m.tail}
	if m.tail == nil {
		m.head = e
	} else {
		m.tail.next = e
	}
	m.tail = e
	m.index[key] = e
	return m
}

// Get returns the value stored under key.
func (m *OrderedMap[K, V]) Get(key K) (V, bool) {
	if e, ok := m.index[key]; ok {
		return e.value, true
	}
	var zero V
	return zero, false
}

// GetOr returns the value stored under key, or fallback if there is none.
func (m *OrderedMap[K, V]) GetOr(key K, fallback V) V {
	if e, ok := m.index[key]; ok {
		return e.value
	}
	return fallback
}

// Has reports whether key is present.
func (m *OrderedMap[K, V]) Has(key K) bool {
	_, ok := m.index[key]
	return ok
}

// Delete removes key and reports whether it was present.
func (m *OrderedMap[K, V]) Delete(key K) bool {
	e, ok := m.index[key]
	if !ok {
		return false
	}
	if e.prev == nil {
		m.head = e.next
	} else {
		e.prev.next = e.next
	}
	if e.next == nil {
		m.tail = e.prev
	} else {
		e.next.prev = e.prev
	}
	delete(m.index, key)
	return true
}

// Len returns the number of entries.
func (m *OrderedMap[K, V]) Len() int { return len(m.index) }

// Clear removes every entry.
func (m *OrderedMap[K, V]) Clear() { *m = OrderedMap[K, V]{} }

// All iterates over the entries in insertion order:
//
//	for key, value := range m.All() { ... }
func (m *OrderedMap[K, V]) All() iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		for e := m.head; e != nil; e = e.next {
			if !yield(e.key, e.value) {
				return
			}
		}
	}
}

// Keys returns the keys in insertion order.
func (m *OrderedMap[K, V]) Keys() List[K] {
	out := make(List[K], 0, m.Len())
	for e := m.head; e != nil; e = e.next {
		out = append(out, e.key)
	}
	return out
}

// Values returns the values in insertion order.
func (m *OrderedMap[K, V]) Values() List[V] {
	out := make(List[V], 0, m.Len())
	for e := m.head; e != nil; e = e.next {
		out = append(out, e.value)
	}
	return out
}

// Set is a collection of distinct values that remembers insertion order,
// like a JavaScript Set. The zero value is ready to use; a Set is not safe
// for concurrent use.
type Set[T comparable] struct{ m OrderedMap[T, struct{}] }

// NewSet returns a set holding values.
func NewSet[T comparable](values ...T) *Set[T] { return new(Set[T]).Add(values...) }

// Add inserts values and returns the set for chaining.
func (s *Set[T]) Add(values ...T) *Set[T] {
	for _, v := range values {
		s.m.Set(v, struct{}{})
	}
	return s
}

// Has reports whether v is in the set.
func (s *Set[T]) Has(v T) bool { return s.m.Has(v) }

// Delete removes v and reports whether it was present.
func (s *Set[T]) Delete(v T) bool { return s.m.Delete(v) }

// Len returns the number of values.
func (s *Set[T]) Len() int { return s.m.Len() }

// Values returns the values in insertion order.
func (s *Set[T]) Values() List[T] { return s.m.Keys() }

// All iterates over the values in insertion order.
func (s *Set[T]) All() iter.Seq[T] {
	return func(yield func(T) bool) {
		for v := range s.m.All() {
			if !yield(v) {
				return
			}
		}
	}
}

// Union returns a new set with the values of both sets.
func (s *Set[T]) Union(other *Set[T]) *Set[T] {
	return NewSet(s.Values()...).Add(other.Values()...)
}

// Intersection returns a new set with the values present in both sets.
func (s *Set[T]) Intersection(other *Set[T]) *Set[T] {
	return NewSet(s.Values().Filter(other.Has)...)
}

// Difference returns a new set with the values of s that are not in other.
func (s *Set[T]) Difference(other *Set[T]) *Set[T] {
	return NewSet(s.Values().Filter(func(v T) bool { return !other.Has(v) })...)
}

// IsSubsetOf reports whether every value of s is in other.
func (s *Set[T]) IsSubsetOf(other *Set[T]) bool { return s.Values().Every(other.Has) }
