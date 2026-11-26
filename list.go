package tau

import (
	"sort"

	"github.com/samber/lo"
)

// List is a slice with chainable methods, in the style of JavaScript arrays.
// Map, FlatMap and Reduce are generic methods (Go 1.27), so a chain can
// change the element type as it goes:
//
//	tau.List[int]{1, 2, 3, 4}.
//		Filter(isEven).
//		Map(strconv.Itoa).
//		Reduce("", func(acc, s string) string { return acc + s })
//
// List is a plain slice, so len, indexing and range work on it directly.
// Most methods are thin wrappers over samber/lo.
type List[T any] []T

// Filter returns the elements for which fn is true.
func (l List[T]) Filter(fn func(T) bool) List[T] {
	out := make(List[T], 0, len(l))
	for _, v := range l {
		if fn(v) {
			out = append(out, v)
		}
	}
	return out
}

// Map returns fn applied to every element. The result may have a different
// element type.
func (l List[T]) Map[U any](fn func(T) U) List[U] {
	out := make(List[U], len(l))
	for i, v := range l {
		out[i] = fn(v)
	}
	return out
}

// FlatMap applies fn to every element and concatenates the results.
func (l List[T]) FlatMap[U any](fn func(T) []U) List[U] {
	out := List[U]{}
	for _, v := range l {
		out = append(out, fn(v)...)
	}
	return out
}

// Reduce folds the list into one value of any type, starting from initial.
func (l List[T]) Reduce[A any](initial A, fn func(acc A, v T) A) A {
	acc := initial
	for _, v := range l {
		acc = fn(acc, v)
	}
	return acc
}

// Find returns the first element for which fn is true.
func (l List[T]) Find(fn func(T) bool) (T, bool) {
	for _, v := range l {
		if fn(v) {
			return v, true
		}
	}
	var zero T
	return zero, false
}

// Some reports whether fn is true for at least one element.
func (l List[T]) Some(fn func(T) bool) bool {
	_, ok := l.Find(fn)
	return ok
}

// Every reports whether fn is true for all elements.
func (l List[T]) Every(fn func(T) bool) bool {
	for _, v := range l {
		if !fn(v) {
			return false
		}
	}
	return true
}

// ForEach calls fn for every element and returns the list for further chaining.
func (l List[T]) ForEach(fn func(T)) List[T] {
	for _, v := range l {
		fn(v)
	}
	return l
}

// Reverse returns a reversed copy.
func (l List[T]) Reverse() List[T] {
	out := make(List[T], len(l))
	for i, v := range l {
		out[len(l)-1-i] = v
	}
	return out
}

// Sort returns a copy sorted by less, which reports whether a goes before b.
func (l List[T]) Sort(less func(a, b T) bool) List[T] {
	out := append(List[T](nil), l...)
	sort.SliceStable(out, func(i, j int) bool { return less(out[i], out[j]) })
	return out
}

// Reject returns the elements for which fn is false.
func (l List[T]) Reject(fn func(T) bool) List[T] {
	return lo.Reject(l, func(v T, _ int) bool { return fn(v) })
}

// Partition splits the list into the elements for which fn is true and the
// rest, keeping their order.
func (l List[T]) Partition(fn func(T) bool) (matched, rest List[T]) {
	return lo.FilterReject(l, func(v T, _ int) bool { return fn(v) })
}

// Count returns how many elements fn is true for.
func (l List[T]) Count(fn func(T) bool) int { return lo.CountBy(l, fn) }

// UniqBy returns the first element for each distinct key.
func (l List[T]) UniqBy[K comparable](key func(T) K) List[T] { return lo.UniqBy(l, key) }

// GroupBy collects the elements under the key fn gives each one.
func (l List[T]) GroupBy[K comparable](key func(T) K) map[K]List[T] { return lo.GroupBy(l, key) }

// KeyBy indexes the elements by key. A later element replaces an earlier one
// with the same key.
func (l List[T]) KeyBy[K comparable](key func(T) K) map[K]T { return lo.KeyBy(l, key) }

// Chunk splits the list into pieces of at most size elements. It throws if
// size is less than 1.
func (l List[T]) Chunk(size int) []List[T] {
	if size < 1 {
		Throwf("tau: Chunk size must be at least 1, got %d", size)
	}
	return lo.Chunk(l, size)
}

// Take returns the first n elements, or all of them if there are fewer.
func (l List[T]) Take(n int) List[T] { return lo.Subset(l, 0, uint(max(n, 0))) }

// Drop returns the list without its first n elements.
func (l List[T]) Drop(n int) List[T] { return lo.Drop(l, max(n, 0)) }

// First returns the first element, if there is one.
func (l List[T]) First() (T, bool) { return lo.First(l) }

// Last returns the last element, if there is one.
func (l List[T]) Last() (T, bool) { return lo.Last(l) }

// Uniq returns the distinct values of l in the order they first appear. It
// is a function because a method cannot require comparable elements.
func Uniq[T comparable](l []T) List[T] { return lo.Uniq(l) }

// Keys returns the keys of m, in no particular order.
func Keys[K comparable, V any](m map[K]V) List[K] { return lo.Keys(m) }

// Values returns the values of m, in no particular order.
func Values[K comparable, V any](m map[K]V) List[V] { return lo.Values(m) }
