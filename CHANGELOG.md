# Changelog

All notable changes to tau are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project
uses [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- `Dig` and `DigOr` now read MongoDB documents (`bson.M`, `bson.A`,
  `bson.D`, from either driver version), any map with string keys, any
  slice or array, and structs. A struct field is named by its `json` tag,
  else its `bson` tag, else its Go name.
- `Dig` converts between numeric types when nothing is lost, so `Dig[int]`
  accepts the `int32` and `int64` values MongoDB returns.
- The `taubson` sub-package. Importing it for its side effect
  (`import _ "github.com/egoisutolabs/tau/taubson"`) makes `Dig` on the v2
  driver's types as fast as on decoded JSON. Only programs that import it
  compile in the driver's `bson` package.

- `Convert`, which builds a value of one struct type from another by
  matching fields by Go name or json name, with `tau:"Path.To.Field"` and
  `tau:"-"` tags for the rest. A destination field with no source is
  refused, not left empty. It throws `*ConvertError`, which names the
  field.

### Changed

- `Dig` used to report false for every one of the types above; it now
  finds the value. Lookups on decoded JSON behave as before, with one
  addition: an integer value can be asked for as a float.

## [0.1.0] - 2026-11-26

The first release.

### Added

- Thrown errors: `Throw`, `Throwf`, `Must` and `Check` to raise; `Catch`,
  `Try`, `TryValue`, `Handle` and `HandleIs` to recover. Only errors
  thrown through tau are caught.
- `*Error`, carrying the stack of the first throw, with `StackOf` and the
  `%+v` verb to read it; `Wrapf` to add context to errors thrown through a
  function.
- Promises: `Async`, with `Await`, `Result`, `Cancel` and `Done`.
- Concurrency helpers that take the caller's context and cancel work they
  no longer need: `All`, `AllSettled`, `Race`, `Any`, `Timeout`, and
  `List.MapAsync` with a concurrency limit.
- Task groups: `WithScope`, `Scope.Go`, `Scope.Context` and `Task`, for
  tasks of different result types. A failure cancels the rest at once and
  the scope returns only after every task has finished.
- Retries: `Retry`, `RetryWith` with a `RetryPolicy` (attempts, delay,
  maximum delay, factor, jitter, a predicate for retryable errors), and
  `Permanent`.
- `List.MapSettled`, which maps concurrently with a limit and returns every
  element's outcome instead of stopping at the first failure.
- `List.ForEachAsync`, `MapAsync` for work done for its effect.
- `Sleep`, which waits or throws the context's error when it ends.
- `List[T]`, a slice with chainable methods: `Map`, `FlatMap`, `Filter`,
  `Reject`, `Reduce`, `Sort`, `Reverse`, `GroupBy`, `KeyBy`, `UniqBy`,
  `Partition`, `Chunk`, `Take`, `Drop`, `Find`, `Some`, `Every`, `Count`,
  `First`, `Last`, `ForEach`; and the functions `Uniq`, `Keys`, `Values`.
- `ParseJSON`, which decodes strictly and validates; `ReadJSON`, the same
  from a reader with a byte limit, throwing `ErrTooLarge`; `Validate`;
  `*InputError`, the single type both throw for bad input, with a `Kind`
  (malformed, unknown field, wrong type, invalid, too large) and a JSON
  Pointer `Path`;
  `*ValidationError` and `FieldError` reporting each failed field by JSON
  path.
- `Field[T]`, a JSON field that distinguishes omitted, null and a value,
  for PATCH requests, with `FieldOf`, `NullField` and validation support.
- `Dig` and `DigOr` for reading untyped JSON.
- `OrderedMap` and `Set`, which keep insertion order.
- `Ptr`, `Deref` and `If`.
- Guides under `docs/`, a contributing guide, this changelog, five
  example programs, and runnable examples on pkg.go.dev.
- An agent skill under `skills/tau/`, installable with
  `npx skills add egoisutolabs/tau`.
