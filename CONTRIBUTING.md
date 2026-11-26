# Contributing

Thanks for taking the time. This page explains how to get a working
checkout, what the checks expect, and the conventions the library keeps.

## Development setup

You need Go 1.27 or newer. Then:

```bash
git clone https://github.com/egoisutolabs/tau.git
cd tau
make verify
```

`make verify` is the full gate, the same checks CI runs: `gofmt`,
`go vet`, the tests under the race detector, and every example. It takes
a few seconds.

## Before opening a pull request

```bash
make verify
```

Keep commits focused and their messages plain: a short summary line, and a
body when the why is not obvious from the diff. The project does not use
commit trailers.

## Conventions

**Only catch what tau threw.** `Catch`, `Try`, `Handle`, `Wrapf` and
everything built on them recover a thrown error and re-panic anything
else. An ordinary panic is a bug and must keep propagating, including out
of a goroutine started by `Async`. `Catch`, `Try`, `TryValue`, `Handle`,
`HandleIs` and `Wrapf` each have a test that a nil-map write passes
through them.

**Context first, and cancel what you no longer need.** Anything that
starts or waits on work takes `ctx context.Context` as its first argument
and gives the work a derived context. A function is either a join, which
returns only after everything it started has returned (`All`, `MapAsync`),
or a select, which cancels the rest and returns at once (`Race`, `Any`,
`Timeout`). A new helper picks one side, documents it, and tests the
cancellation.

**Keep the stack.** A thrown error records where it was first thrown.
Re-throwing, `Wrapf`, `Retry` and crossing a goroutine must not replace
that stack.

**No numbered names.** Go has no variadic type parameters, so a helper
that needs one copy per argument count (`Foo1`, `Foo2`) is not added. Take
a closure instead.

**Delegate, and stay curated.** Where `samber/lo` or the standard library
already does the job, tau's function is a thin wrapper over it. tau is
not a mirror of `lo`: a function is added when people commonly need it
from tau-style code, not because `lo` has it.

**Methods for what chains.** Operations on `List[T]` are generic methods.
One that needs more of the element type than `any`, such as comparable
elements, has to be a function, because a method cannot narrow `T`.

**Dependencies are `lo` and `validator`.** Adding another needs a reason
that outweighs its cost to every importer; validator alone adds about
5.5 MB to a binary. The MongoDB driver's `bson` package is imported only by
the `taubson` sub-package, never by the root package, so that programs that
do not use MongoDB do not compile it in. A sub-package reaches the root
through a hook in `internal/` (see `internal/dighook`), set once from
`init`.

## Adding to the API

1. Check that it does not already exist: `go doc github.com/samber/lo.<Name>`
   and the standard library.
2. Show it earning its place on real code: the same function written with
   and without the helper. A helper that only shortens a contrived example
   is not added.
3. Add it with a doc comment that states what it throws, and for anything
   concurrent, what it cancels and when it returns.
4. Add tests in the matching `_test.go` file (`async.go` and
   `async_test.go`). Cover the happy path, the error path, and for
   concurrency, cancellation by the caller.
5. Add an `Example` in `example_test.go` if the function is one people
   will look up, and a line in `CHANGELOG.md` under Unreleased.
6. Update the guide under `docs/` and the method tables in the README.
7. Add it to `skills/tau/references/api.md`, and to `skills/tau/SKILL.md`
   if agents need to be told when to use it or what to avoid.
   `TestSkillReferenceMatchesTheAPI` fails until the reference lists it.

## Reporting a bug

Include the Go version (`go version`), the tau version, a small program
that shows the problem, and what you expected. For an error, the output of
`fmt.Printf("%+v", err)` includes the stack and is the most useful thing
to attach.
