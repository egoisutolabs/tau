# Coming from Node.js

## The mapping

| JavaScript | tau |
| --- | --- |
| `throw new Error("x")` | `tau.Throwf("x")` |
| `throw err` | `tau.Throw(err)` |
| `try { ... } catch (e) { ... }` | `err := tau.Try(func() { ... })` |
| `catch (e) { if (e instanceof NotFound) ... else throw e }` | `defer tau.Handle(func(e *NotFound) { ... })` |
| `finally { ... }` | `defer` (plain Go) |
| `e.stack` | `tau.StackOf(err)`, or `fmt.Sprintf("%+v", err)` |
| `async function f(x) { ... }` | `func f(ctx, x) *tau.Promise[T] { return tau.Async(ctx, func(ctx) T { ... }) }` |
| `await p` | `p.Await(ctx)` |
| `Promise.all([...])` | `tau.All(ctx, fns...)` |
| `Promise.allSettled`, `race`, `any` | `tau.AllSettled`, `tau.Race`, `tau.Any` |
| `p-map` with a concurrency limit | `list.MapAsync(ctx, limit, fn)` |
| `p-map` with `stopOnError: false` | `list.MapSettled(ctx, limit, fn)` |
| `Promise.all([fetchUser(), fetchPosts()])` with different types | `tau.WithScope(ctx, func(s *tau.Scope) T { ... s.Go(...) ... })` |
| `p-retry` with options | `tau.RetryWith(ctx, tau.RetryPolicy{...}, fn)` |
| `AbortController` | `context.Context` (plain Go) |
| `await setTimeout(ms)` from `timers/promises`, with a signal | `tau.Sleep(ctx, d)` |
| `'bio' in body` versus `body.bio === null` | `tau.Field[T]`: `IsOmitted()`, `IsNull()`, `Get()` |
| `arr.map(f).filter(g)` | `tau.List[T](arr).Map(f).Filter(g)` |
| `arr.reduce(f, init)` | `list.Reduce(init, f)` |
| `Object.groupBy(arr, f)` | `list.GroupBy(f)` |
| `[...new Set(arr)]` | `tau.Uniq(arr)` |
| `new Map()`, `new Set()` | `tau.NewMap[K, V]()`, `tau.NewSet[T]()` |
| `a?.b?.[0]?.c` | `tau.Dig[T](a, "b", 0, "c")` |
| `x ?? fallback` | `tau.Deref(ptr, fallback)`, `tau.DigOr(...)` |
| `cond ? a : b` | `tau.If(cond, a, b)` |
| `zod.parse(body)` | `tau.ParseJSON[T](body)` with `validate` tags |

## Where Go differs

**Every function is already async.** In JavaScript `await` exists because
there is one thread. In Go any function can block without stalling the
program, so you only reach for `Async` when you want two things to run at
the same time. Calling a function that does I/O and using its result needs
no promise at all.

**There is no lambda syntax.** A callback is written out in full:
`func(o Order) bool { return o.Paid }`. Passing a named function or a
method value (`strconv.Itoa`, `seen.Has`) avoids it where one exists.

**Cancellation is explicit.** A context is passed as the first argument
and handed on to whatever does the I/O. In exchange, tau cancels abandoned
work for you: when `All` fails or an async function returns, the calls it
started are told to stop. JavaScript promises cannot do that.

**A plain panic is not a rejection.** In JavaScript any exception inside
an async function rejects its promise. In tau only a *thrown* error does.
A nil dereference inside `Async` crashes the program, because it is a bug
and not a failure to handle.

**`If` evaluates both sides.** `tau.If(cond, a, b)` is a function call, so
`a` and `b` are both computed first. `tau.If(p != nil, p.Name, "")` will
crash on a nil `p`. Use an `if` statement, or `tau.Deref`.

**Types cannot be added by a method.** `list.Map` can change the element
type, but a method cannot demand that elements be comparable, so `Uniq` is
a function and not `list.Uniq()`.

**Exported functions return errors.** The convention in Go is
`(value, error)`. Throw inside your package, and catch on the way out with
`defer tau.Catch(&err)`. See [Getting started](getting-started.md).
