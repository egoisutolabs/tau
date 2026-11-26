# Ordered map and set

Go's built-in map iterates in a random order, and Go has no set type.
`OrderedMap` and `Set` behave like JavaScript's `Map` and `Set`: they
remember the order things were added in.

## OrderedMap

```go
m := tau.NewMap[string, int]().Set("zebra", 1).Set("apple", 2)
m.Set("zebra", 10) // keeps its place

for key, value := range m.All() { ... } // zebra, then apple
```

| | |
| --- | --- |
| `Set(k, v)` | store; returns the map for chaining. An existing key keeps its position |
| `Get(k)`, `GetOr(k, fallback)`, `Has(k)` | read |
| `Delete(k)` | remove; reports whether it was there. A key added again goes to the end |
| `Len()`, `Clear()` | |
| `Keys()`, `Values()` | as lists, in insertion order |
| `All()` | an iterator for `range` |

## Set

```go
seen := tau.NewSet("a", "b")
seen.Add("c", "a") // "a" is already there
seen.Values()      // [a b c]
```

| | |
| --- | --- |
| `Add(values...)` | returns the set for chaining |
| `Has(v)`, `Delete(v)`, `Len()` | |
| `Values()`, `All()` | in insertion order |
| `Union`, `Intersection`, `Difference` | each returns a new set |
| `IsSubsetOf(other)` | |

## Notes

- The zero value of each is ready to use: `var m tau.OrderedMap[string, int]`.
- Neither is safe for concurrent use. Guard them with a mutex if several
  goroutines write.
- They do not have a JSON form of their own.
