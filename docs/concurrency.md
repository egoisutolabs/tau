# Concurrency

## Promises and async functions

`tau.Async` runs a function in its own goroutine and returns a promise:

```go
p := tau.Async(ctx, func(ctx context.Context) User { return fetchUser(ctx, id) })
user := p.Await(ctx) // waits; throws if the function threw
```

An *async function*, in the JavaScript sense, is an ordinary function that
returns `Async`. Calling it starts the work:

```go
func loadPage(ctx context.Context, id string) *tau.Promise[Page] {
	return tau.Async(ctx, func(ctx context.Context) Page {
		user := fetchUser(ctx, id)   // both start now
		posts := fetchPosts(ctx, id) // and run at the same time
		return render(user.Await(ctx), posts.Await(ctx))
	})
}

page := loadPage(ctx, "u1").Await(ctx)
```

Awaiting inside an async function blocks only that goroutine, which is
what `await` does to a JavaScript function.

| | |
| --- | --- |
| `p.Await(ctx)` | the value, or throws the error; stops waiting if `ctx` ends |
| `p.Result(ctx)` | the same, returning `(T, error)` |
| `p.Cancel()` | cancel the function's context |
| `p.Done()` | a channel closed when the promise settles, for `select` |

To start an existing `(T, error)` function, wrap it with `Must`:

```go
user := tau.Async(ctx, func(ctx context.Context) User { return tau.Must(client.FetchUser(ctx, id)) })
```

## Task groups

When one piece of work needs several results of different types, start
them in a scope:

```go
page := tau.WithScope(ctx, func(s *tau.Scope) Page {
	user := s.Go(func(ctx context.Context) User { return tau.Must(fetchUser(ctx, id)) })
	posts := s.Go(func(ctx context.Context) []Post { return tau.Must(fetchPosts(ctx, id)) })
	return Page{User: user.Await(), Posts: posts.Await()}
})
```

A scope gives two guarantees that separate `Async` calls do not:

- **A failure cancels everything else, at once.** If a task throws, or the
  function you passed throws, the scope's context is cancelled with that
  error. The other tasks stop, and an `Await` that was blocked throws it
  immediately. With plain promises a failure is only noticed when that
  promise is awaited, which may be much later.
- **Nothing outlives the scope.** `WithScope` returns only after every task
  has returned, whether it is returning a value or throwing.

`WithScope` then throws the first failure or returns your function's
result. A task you start and never await still runs to completion, and
still fails the scope if it throws.

To put other work under the same roof, pass it `s.Context()`:

```go
return tau.WithScope(ctx, func(s *tau.Scope) Response {
	discount := s.Go(fetchDiscount)
	items := tau.List[string](ids).MapAsync(s.Context(), 4, fetchItem) // stops if the discount fails
	return build(items, discount.Await())
})
```

| | |
| --- | --- |
| `s.Go(fn)` | start a task; returns a `*Task[T]` |
| `task.Await()` | the value, or throws; needs no context, it uses the scope's |
| `task.Result()` | the same, returning `(T, error)` |
| `s.Context()` | the scope's context |

Use a scope for a fixed handful of different things, `All` for several of
the same type, and `MapAsync` for a list.

## Helpers

| | |
| --- | --- |
| `All(ctx, fns...)` | run all, results in order; throws the first error |
| `AllSettled(ctx, fns...)` | run all, each outcome as `Settled{Value, Err}`; never throws |
| `Race(ctx, fns...)` | the first to finish, value or error |
| `Any(ctx, fns...)` | the first to succeed; throws all errors joined if none does |
| `list.MapAsync(ctx, limit, fn)` | map concurrently, at most `limit` at once, results in order; stops at the first failure |
| `list.MapSettled(ctx, limit, fn)` | the same, but every element gets its own outcome and nothing throws |
| `list.ForEachAsync(ctx, limit, fn)` | `MapAsync` for work done for its effect, with no results |
| `Sleep(ctx, d)` | wait, or throw the context's error as soon as it ends |
| `Timeout(ctx, d, fn)` | run with a deadline; throws `ErrTimeout` |
| `Retry(ctx, attempts, delay, fn)` | retry with a doubling delay |
| `RetryWith(ctx, policy, fn)` | retry under a `RetryPolicy` |

`ErrTimeout` also matches `context.DeadlineExceeded`.

```go
sizes := tau.List[string](urls).MapAsync(ctx, 8, func(ctx context.Context, url string) int {
	return tau.Retry(ctx, 3, 100*time.Millisecond, func(ctx context.Context, _ int) int {
		return tau.Timeout(ctx, 2*time.Second, func(ctx context.Context) int {
			return tau.Must(fetchSize(ctx, url))
		})
	})
})
```

## Retry policies

`Retry` covers the simple case. For anything more, describe the policy
once as a value and share it:

```go
var paymentsRetry = tau.RetryPolicy{
	Attempts: 5,                      // total tries, including the first
	Delay:    100 * time.Millisecond, // before the second try
	MaxDelay: 2 * time.Second,        // cap on the wait
	Jitter:   0.5,                    // each wait is a random 50% to 100% of its length
	RetryIf:  isTemporary,            // which errors are worth another try
}

receipt := tau.RetryWith(ctx, paymentsRetry, func(ctx context.Context, _ int) Receipt {
	return tau.Must(charge(ctx, order))
})
```

The wait doubles after each try unless `Factor` says otherwise (`1` keeps
it constant). With `RetryIf` unset every error is retried. An error marked
`tau.Permanent(err)` is never retried, whatever the policy says, which is
the way to give up from inside the work itself. The zero policy tries
once. Cancelling the context stops a retry, including in the middle of a
wait.

## Batches

`MapAsync` stops at the first failure, which is right when you need every
result. For imports, notifications and other jobs where each element
stands alone, `MapSettled` keeps going and reports each one:

```go
results := jobs.MapSettled(ctx, 8, processJob) // List[Settled[Result]]

failed := results.Filter(func(r tau.Settled[Result]) bool { return !r.OK() })
```

It never throws. If the context ends part-way, the calls already running
are cancelled through their context, and each element that was never
started gets the context's error as its outcome, so the result always has
one entry per element.

## Cancellation

Every function takes the caller's context first and hands the work a
derived one. Cancelling the caller's context cancels the work. Beyond
that, each function cancels work it no longer needs:

| | Cancels | Returns |
| --- | --- | --- |
| `All`, `MapAsync`, `ForEachAsync`, `WithScope` | the rest, on the first failure | after everything it started has returned |
| `AllSettled`, `MapSettled` | nothing | after everything has returned |
| `Race`, `Any` | the rest, once it has its answer | at once |
| `Timeout` | the function, at the deadline | at once |
| `Retry` | stops between and during waits | at once |
| `Async` | when the promise's `Cancel` is called, or its function returns | `Await` stops waiting when its `ctx` ends |

So `All`, `MapAsync`, `ForEachAsync`, `MapSettled` and `WithScope` are joins: nothing they started is still running
when they return. `Race`, `Any` and `Timeout` are selects: they answer
promptly and leave the cancelled work to wind down.

Work that is cancelled because a sibling failed is told why: its context's
cause is the sibling's error, and that is what `Sleep`, `Await` and the
other functions throw. So a cancelled task usually throws the original
failure, not `context.Canceled`.

Two consequences worth knowing:

- **Cancellation is cooperative.** A function that ignores its context
  keeps running. Pass `ctx` on to whatever does the I/O.
- **Work started inside an async function does not outlive it.** The
  context an async function receives is cancelled when it returns or
  throws, so a call it started and never awaited is cancelled too. For the
  same reason, a promise started inside cannot be handed out as the
  result: await it and return the value.

## Panics

An error thrown inside any of these rejects the promise or is thrown to
the caller. An ordinary panic (a nil dereference, say) is not caught and
crashes the program, as it would in any goroutine.

## Cost

Each `Async` call costs a goroutine and a few allocations, about half a
microsecond. That is invisible next to I/O and wasteful for small
computations in a loop.
