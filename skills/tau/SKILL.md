---
name: tau
description: Write and review Go code that uses the tau library (github.com/egoisutolabs/tau) - thrown errors with Must/Check/Catch, promises and async functions, task groups with WithScope, MapAsync, retries and timeouts, chainable lists, strict JSON parsing with validation, InputError, and Field for PATCH requests. Use when Go code imports tau, when asked to add tau to a Go service, handler, job or script, or when converting error-handling and concurrency boilerplate to tau.
---

# tau

tau is a Go library that brings Node.js conveniences to Go server code:
thrown errors, promises, chainable lists, and strict JSON with validation.
Import path `github.com/egoisutolabs/tau`, package `tau`. It needs Go 1.27
or newer.

The full API with signatures is in [references/api.md](references/api.md).
Read it before using a function you have not used in this session; do not
guess names or argument order.

## Decide first: does tau belong here?

Use tau at the edges of a program: HTTP handlers, calls to other services,
background jobs, CLIs and scripts. That code is I/O, concurrency, parsing
and error plumbing, which is what tau shortens.

Do not introduce tau into core logic: rule engines, pricing, parsers, tight
loops. A throw costs a panic plus a stack capture (about 1 to 2
microseconds). Where "no" is a normal, frequent answer, return a value. If
the surrounding package does not already use tau and the code is core
logic, write plain Go and say why.

Do not add tau to a module that targets a Go version below 1.27.

## The one rule: throw inside, return at the boundary

Unexported helpers may throw. Every exported function returns `(T, error)`
and catches, so callers never see a throw.

```go
func LoadConfig(path string) (cfg Config, err error) {
	defer tau.Catch(&err)
	defer tau.Wrapf("load config %s", path)
	return tau.ParseJSON[Config](tau.Must(os.ReadFile(path))), nil
}
```

- `tau.Must(v, err)` returns `v` or throws `err`. `tau.Check(err)` is the
  same for functions that return only an error.
- `defer tau.Catch(&err)` needs a *named* `err` result.
- If the function builds its result in steps, name the other results `_`
  (`(_ []Item, err error)`), so a throw part-way cannot return a half-built
  value.
- An uncaught throw crashes the program like any panic. Every goroutine
  entry point and every handler needs a catch: `tau.Try`, `tau.TryValue`,
  or `defer tau.Catch(&err)`.

In an HTTP handler:

```go
resp, err := tau.TryValue(func() Response { return serve(r) })
if err != nil {
	if bad, ok := errors.AsType[*tau.InputError](err); ok {
		http.Error(w, bad.Error(), http.StatusBadRequest)
		return
	}
	log.Printf("request failed: %+v", err) // %+v prints the stack
	http.Error(w, "internal error", http.StatusInternalServerError)
	return
}
```

## Errors

- Compare caught errors with `errors.Is` and `errors.As` (or
  `errors.AsType`). Never `err == ErrX` or `err.(*T)`: a caught error is a
  `*tau.Error` wrapper around the original.
- Add context with `defer tau.Wrapf("doing %s", thing)` at the top of a
  function, not with `fmt.Errorf` at every call.
- `tau.Must` takes exactly `(value, error)`. For a function returning two
  values and an error: `a, b, err := f(); tau.Check(err)`.
- To recover from one error type and let the rest continue:
  `defer tau.Handle(func(e *NotFoundError) { result = fallback })`. With
  more than one case, use `tau.TryValue` and an ordinary `switch` instead
  of stacking `Handle` calls, which run last-in-first-out.
- tau only catches what was thrown through tau. Never rely on it to catch
  a nil dereference or other ordinary panic, and never write code that
  recovers one to keep going.
- Do not throw for an expected outcome on a hot path (a cache miss, a rule
  that does not apply). Return `(T, bool)` or a value.

## Concurrency

Every function takes `ctx context.Context` first and hands the work a
derived context. **Pass that inner `ctx` on to whatever does the I/O.**
Cancellation is cooperative: work that ignores its context keeps running.

Pick by shape:

| Need | Use |
| --- | --- |
| A few results of different types | `tau.WithScope` and `s.Go` |
| Several results of the same type | `tau.All` |
| One result per list element, all needed | `list.MapAsync(ctx, limit, fn)` |
| One outcome per element, failures tolerated | `list.MapSettled(ctx, limit, fn)` |
| A side effect per element | `list.ForEachAsync(ctx, limit, fn)` |
| First to finish / first to succeed | `tau.Race` / `tau.Any` |
| A deadline | `tau.Timeout(ctx, d, fn)` |
| Retries | `tau.Retry`, or `tau.RetryWith` with a `tau.RetryPolicy` |
| A pause | `tau.Sleep(ctx, d)`, never `time.Sleep` in cancellable code |

```go
page := tau.WithScope(ctx, func(s *tau.Scope) Page {
	user := s.Go(func(ctx context.Context) User { return tau.Must(fetchUser(ctx, id)) })
	posts := s.Go(func(ctx context.Context) []Post { return tau.Must(fetchPosts(ctx, id)) })
	return Page{User: user.Await(), Posts: posts.Await()}
})
```

Rules that are easy to get wrong:

- **Prefer `WithScope` to several bare `tau.Async` calls.** With bare
  promises a failure is only noticed when that promise is awaited, and the
  other work runs on. In a scope a failure cancels the rest at once and
  nothing outlives the scope.
- To put a `MapAsync` under a scope, pass it `s.Context()`, not the outer
  `ctx`.
- Always give `MapAsync`, `MapSettled` and `ForEachAsync` a real limit for
  I/O (for example 4 to 16). `0` means unlimited.
- An async function is an ordinary function returning
  `tau.Async(ctx, func(ctx context.Context) T { ... })`. Do not return a
  promise that was started *inside* an async function: its context is
  cancelled when the function returns. Await it and return the value.
- Work cancelled because a sibling failed throws the sibling's error, not
  `context.Canceled`. Do not test for `context.Canceled` to detect it.
- An ordinary panic inside any of these crashes the program. That is
  intended.
- `tau.Retry` retries every thrown error. Mark one as final with
  `tau.Throw(tau.Permanent(err))`, or give `RetryWith` a `RetryIf`.
- Do not use `tau.Async` for small CPU-only work in a loop; each call
  costs a goroutine.

## Lists

`tau.List[T]` is a slice with chainable methods. Convert with
`tau.List[T](slice)`; `len`, indexing and `range` still work.

```go
names := tau.List[Order](orders).
	Filter(func(o Order) bool { return o.Paid }).
	Map(func(o Order) string { return o.Item }) // the type may change
```

- Methods do not modify the receiver.
- `Uniq`, `Keys` and `Values` are functions (`tau.Uniq(slice)`), because a
  method cannot require comparable elements.
- For a loop that returns early on an error, keep a `for` loop, or throw
  from inside `Map`.
- Anything not on `List` comes from `samber/lo` or the standard `slices`
  package; a `List` is accepted wherever they take a slice.

## JSON and validation

```go
type Signup struct {
	Email string `json:"email" validate:"required,email"`
	Age   int    `json:"age" validate:"gte=18"`
	Items []Item `json:"items" validate:"required,min=1,dive"`
}

req := tau.ReadJSON[Signup](r.Body, 1<<20) // request bodies: always a limit
cfg := tau.ParseJSON[Config](data)         // bytes already in hand
```

- Both decode strictly (unknown fields, duplicate keys and trailing data
  are errors), then apply `validate` tags
  ([go-playground/validator](https://pkg.go.dev/github.com/go-playground/validator/v10)
  rules), then call the type's `Validate() error` method if it has one.
- Both throw `*tau.InputError` for anything that is the input's fault. Map
  that to a 400 with one `errors.AsType` check; do not wrap parse errors in
  your own sentinel. Its `Kind` and `Path` say what and where.
- A tag failure carries a `*tau.ValidationError` whose `Fields` list every
  failed field by JSON path.
- **Put `dive` on a slice or map of structs**, or its elements are not
  validated.
- Encode with the standard library: `tau.Must(json.Marshal(v))`.
- For PATCH requests use `tau.Field[T]`, which tells omitted from `null`
  from a value. Tag it `json:"bio,omitzero" validate:"omitnil,max=200"`:
  `omitzero` so an omitted field is left out when encoding, and `omitnil`
  (or `required`) **first** in `validate`, since a bare rule fails for an
  omitted field. Apply it with `profile.Bio = req.Bio.OrElse(profile.Bio)`.
- `tau.Dig[T](data, "user", "addresses", 0, "city")` reads untyped JSON,
  MongoDB documents (`bson.M`, `bson.D`, `bson.A`) and structs (fields by
  `json` tag, then `bson` tag, then Go name). Use it for a few values from
  data whose shape you do not own; in a loop over many documents decode
  into a struct instead. In a program that reads MongoDB documents with
  `Dig`, add `import _ "github.com/egoisutolabs/tau/taubson"` once.

## Converting structs

- To build a response from a row (or any struct from another), use
  `tau.Convert[Response](row)` instead of a field-by-field mapper, then set
  computed fields yourself. It works on slices: `tau.Convert[[]Response](rows)`.
- Every destination field must match a source field by Go name or json
  name. For the rest, tag the destination field: `tau:"Customer.Email"` to
  read a nested or renamed source field, `tau:"-"` for a field you compute.
  A missing match throws `*tau.ConvertError` naming the field; it does not
  leave the field empty.
- The mapping is checked when the code runs, not by the compiler. **Add a
  test that calls each conversion once**; an empty value is enough, since
  the check depends only on the types.
- Do not use it in a loop over thousands of rows (it costs about 1 µs per
  response by reflection); write that mapper by hand.

## Small helpers

- `tau.Ptr(v)`, `tau.Deref(p, fallback)` for optional fields.
- `tau.If(cond, a, b)` evaluates **both** `a` and `b`. Never write
  `tau.If(p != nil, p.Name, "")`; use an `if` statement or `tau.Deref`.
- `tau.OrderedMap` and `tau.Set` keep insertion order. Neither is safe for
  concurrent use.
- tau has no switch-expression or pipe helper; use a `switch` statement
  and ordinary function calls.

## Before finishing

1. Every exported function you touched returns `(T, error)` and has
   `defer tau.Catch(&err)`, or does not throw.
2. Every goroutine entry point and handler catches.
3. Every callback that does I/O uses the `ctx` it was given, not an outer
   one.
4. No `err ==` or `err.(*T)` on an error that may have been thrown.
5. `gofmt`, `go vet ./...` and `go test -race ./...` pass. Concurrency
   added with tau should have a test that cancels the caller's context.
