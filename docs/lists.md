# Lists

`tau.List[T]` is a slice with methods, so operations read as a chain:

```go
names := tau.List[Order](orders).
	Filter(func(o Order) bool { return o.Paid }).
	Sort(func(a, b Order) bool { return a.Cents > b.Cents }).
	Take(3).
	Map(func(o Order) string { return o.Item }) // List[Order] -> List[string]
```

It is a plain slice underneath. `len`, indexing, `range`, the standard
`slices` package and `samber/lo` all work on it, and any `[]T` converts
with `tau.List[T](s)`.

## Methods

| | |
| --- | --- |
| Transform | `Map`, `FlatMap`, `Filter`, `Reject`, `Reduce`, `Sort`, `Reverse` |
| Group and split | `GroupBy`, `KeyBy`, `UniqBy`, `Partition`, `Chunk`, `Take`, `Drop` |
| Query | `Find`, `Some`, `Every`, `Count`, `First`, `Last` |
| Other | `ForEach`, `MapAsync` |

No method changes the list it is called on.

`Map`, `FlatMap` and `Reduce` can produce a different type, and `GroupBy`,
`KeyBy` and `UniqBy` take a key of any comparable type. `GroupBy` returns
`map[K]List[T]`, so each group chains as well:

```go
for customer, theirs := range orders.GroupBy(func(o Order) string { return o.Customer }) {
	total := theirs.Reduce(0, func(sum int, o Order) int { return sum + o.Cents })
	...
}
```

`MapAsync` is covered in [Concurrency](concurrency.md).

## What stays a function

A method cannot require more of the element type than the list declares,
so operations that need comparable elements are functions:

| | |
| --- | --- |
| `tau.Uniq(slice)` | distinct values, in first-seen order |
| `tau.Keys(m)`, `tau.Values(m)` | a map's keys or values as a list, in no particular order |

For anything else (`Zip`, `Sum`, `Intersect`, and several hundred more),
import `samber/lo` and pass the list directly.

## Loops that stop early

A chain suits code that transforms every element. A loop that returns an
error part-way reads better as a `for` loop, or with a throwing function
inside `Map`:

```go
nums := tau.List[string](in).Map(func(s string) int { return tau.Must(strconv.Atoi(s)) })
```
