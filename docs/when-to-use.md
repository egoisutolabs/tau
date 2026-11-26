# When to use tau

## Where it fits

tau is for the edges of a program: HTTP handlers, calls to other services,
background jobs, command-line tools and scripts. That code is mostly I/O,
concurrency, parsing and error plumbing, which is what tau shortens.

It is a poor fit for core logic: rule engines, pricing, parsers, tight
loops. There, "no" is often a normal answer on a hot path, errors need
specific messages that tau cannot shorten, and plain Go is already about
as short as it gets.

The two combine well. Keep the core in plain Go, returning ordinary
errors, and call it from tau-style edges. Because tau code returns plain
`(T, error)` at its boundaries, the core never needs to know tau exists.

## What was measured

Two experiments were run while tau was being built:

| Code | Result |
| --- | --- |
| One endpoint: validated input, concurrent fetches with a limit, retries, per-call timeouts, error mapping | 160 lines in plain Go, 81 with tau; identical status and timing across ten scenarios |
| Two files of a rules engine with no concurrency | about 15% fewer lines; its whole test suite passed unchanged |

The saving comes almost entirely from concurrency (`MapAsync`, `Retry`,
`Timeout`) and from pass-through error checks (`Must`). Code that creates
its own errors with specific messages shrinks very little.

## What it costs

| | Cost |
| --- | --- |
| A thrown error | a panic and a stack capture, about 1 to 2 µs |
| An `Async` call | a goroutine, about 0.5 µs and 6 allocations |
| A `Convert` call | reflection: about 1 µs for a nested response of twenty fields, against 0.2 µs written by hand |
| A `Dig` lookup outside decoded JSON | reflection: about 90 ns for three steps, against 25 ns (see the `taubson` package for MongoDB) |
| Importing tau | about 6 MB of binary size, nearly all from the validator and the `golang.org/x` modules it needs |

In the rules-engine experiment the success path ran at the same speed, but
a refused rule went from 2.85 µs to 4.56 µs, because a refusal there is an
expected, frequent answer that had been turned into a throw. Hence the
rule: **throw for failures, not for expected answers.** Next to a network
call a microsecond is nothing; in a loop that says "no" a million times it
is the whole cost.

## What you give up

Go's explicit `if err != nil` shows every place a function can exit. With
tau, a call to `tau.Must` can leave the function and nothing at the call
site looks like a `return`. That is the objection that kept a built-in
`try` out of Go, and it is a real one. tau's answers are partial:

- thrown errors stop at package boundaries, so the effect stays local;
- a thrown error carries a stack, which a plain Go error does not;
- only errors thrown through tau are caught, so real bugs still crash.

Whether that trade is worth it depends on the team reading the code. For a
small team that knows its codebase, it usually is. For a large shared
codebase with many occasional readers, plain Go's explicitness may be
worth more than the brevity.
