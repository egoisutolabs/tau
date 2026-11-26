# tau

[![Go Reference](https://pkg.go.dev/badge/github.com/egoisutolabs/tau.svg)](https://pkg.go.dev/github.com/egoisutolabs/tau)
[![CI](https://github.com/egoisutolabs/tau/actions/workflows/ci.yml/badge.svg)](https://github.com/egoisutolabs/tau/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/egoisutolabs/tau)](https://goreportcard.com/report/github.com/egoisutolabs/tau)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Node.js conveniences for Go server code, behind one import: thrown errors
with stacks, promises and async functions with cancellation, chainable
lists, and strict JSON parsing with validation.

```go
type Request struct {
	UserID  string   `json:"userId" validate:"required"`
	ItemIDs []string `json:"itemIds" validate:"required,min=1"`
}

func Summary(ctx context.Context, body []byte) (total int, err error) {
	defer tau.Catch(&err) // thrown errors become the returned error

	req := tau.ParseJSON[Request](body) // strict, validated
	user := tau.Must(fetchUser(ctx, req.UserID))

	items := tau.List[string](req.ItemIDs).MapAsync(ctx, 4, // at most 4 at once
		func(ctx context.Context, id string) Item {
			return tau.Retry(ctx, 3, 50*time.Millisecond, func(ctx context.Context, _ int) Item {
				return tau.Must(fetchItem(ctx, id))
			})
		})

	total = items.Reduce(0, func(sum int, i Item) int { return sum + i.Cents })
	return total - total*user.DiscountPercent/100, nil
}
```

## Features

- **Thrown errors.** `Must`, `Check` and `Throw` replace pass-through
  `if err != nil` blocks; `Catch` turns a thrown error back into a returned
  one at the package boundary. A thrown error records the stack of its
  first throw, and `Wrapf` adds context. Only errors thrown through tau are
  caught, so a nil dereference or any other real bug still crashes.
- **Promises and async functions.** `Async` and `Await`, with `All`,
  `AllSettled`, `Race`, `Any`, `Timeout`, a cancellable `Sleep`, and the
  concurrency-limited `MapAsync`, `MapSettled` and `ForEachAsync`.
  Everything takes a `context.Context`, and work that is no longer needed
  is cancelled for you.
- **Task groups.** `WithScope` runs tasks of different result types
  together. A failure in one cancels the others at once, and nothing is
  still running when the scope returns.
- **Retries.** `Retry` for the simple case; `RetryWith` and a reusable
  `RetryPolicy` for a delay cap, jitter and a rule for which errors to
  retry.
- **Chainable lists.** `Map`, `Filter`, `Reduce`, `GroupBy`, `Partition`,
  `Chunk` and more as methods, where a chain can change the element type.
  A `List` is a plain slice underneath.
- **Strict JSON with validation.** `ParseJSON` rejects unknown fields and
  duplicate keys, applies `validate` struct tags, and reports every failed
  field by its JSON path. `ReadJSON` does the same from a reader with a
  size limit. Every input failure is one `InputError` type, so a handler
  recognises a bad request with a single check. `Field[T]` tells an omitted
  key from an explicit `null`, for PATCH requests. `Dig` reads untyped
  JSON, MongoDB documents and structs without a chain of type assertions.
- **Ordered map and set.** `OrderedMap` and `Set` keep insertion order,
  like their JavaScript namesakes.
- **One import.** tau wraps [`samber/lo`](https://github.com/samber/lo) and
  [`go-playground/validator`](https://github.com/go-playground/validator),
  so everyday work needs nothing else. The optional `tau/taubson`
  sub-package speeds up `Dig` on MongoDB documents; only programs that
  import it carry the driver's `bson` package.

## Installation

```bash
go get github.com/egoisutolabs/tau
```

Go 1.27 or newer is required, for generic methods and `encoding/json/v2`.

## Where it fits

tau is for the edges of a program: handlers, calls to other services,
jobs and scripts. On one concurrency-heavy endpoint it halved the code
(160 lines to 81) with identical behaviour. It is a poor fit for core
logic such as rule engines and hot loops, where it saved about 15% and
made an expected "no" answer slower, since a throw costs a panic and a
stack capture. [When to use tau](docs/when-to-use.md) has the
measurements and the trade-offs.

## Documentation

The [documentation](docs/README.md) covers each area; the
[API reference](https://pkg.go.dev/github.com/egoisutolabs/tau) documents
every function.

| Guide | Covers |
| --- | --- |
| [Getting started](docs/getting-started.md) | Install, a first function, the boundary rule |
| [When to use tau](docs/when-to-use.md) | Where it pays off, what it costs, what you give up |
| [Errors](docs/errors.md) | Throwing, catching, context, stacks, meeting ordinary errors |
| [Concurrency](docs/concurrency.md) | Promises, async functions, task groups, retry policies, batches, the cancellation model |
| [Lists](docs/lists.md) | Chaining, grouping, what stays a function |
| [JSON and validation](docs/json-and-validation.md) | Strict parsing, size-limited reading, the input error type, validation tags, PATCH fields, untyped JSON, MongoDB documents |
| [Ordered map and set](docs/collections.md) | `OrderedMap` and `Set` |
| [Coming from Node.js](docs/coming-from-node.md) | The mapping from JavaScript idioms, and where Go differs |

## Examples

Five complete programs live under [`examples/`](examples/README.md), each
checking its own result: errors, concurrency, lists, validation, and a
whole endpoint run against eleven scenarios. Run one from its directory with
`go run .`.

## For coding agents

tau ships a skill that teaches coding agents (Claude Code, Cursor, Codex and
others) how to write Go with it: where it belongs, the boundary rule, which
concurrency helper fits which shape, and the mistakes to avoid. Install it
with the [`skills`](https://github.com/vercel-labs/skills) CLI:

```bash
npx skills add egoisutolabs/tau
```

The skill lives in [`skills/tau/`](skills/tau/SKILL.md). A test keeps its
API reference in step with the package.

## Repository layout

| Path | Contents |
| --- | --- |
| `*.go` | The library, a single Go package at the module root |
| `examples/` | Example programs |
| `docs/` | The guides |
| `skills/tau/` | The agent skill: `SKILL.md` and an API reference |

## Status

tau is pre-1.0 and has not been used in production. The API may still
change; see the [changelog](CHANGELOG.md).

## Contributing

Bug reports, questions and pull requests are welcome. The
[contributing guide](CONTRIBUTING.md) explains the checks and the
conventions the library keeps.

## License

MIT, see [LICENSE](LICENSE).
