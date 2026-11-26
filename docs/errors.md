# Errors

tau adds thrown errors to Go. They are built on `panic` and `recover`, with
one important restriction: tau only ever catches what was thrown through
tau. A nil dereference, an index out of range, or any other ordinary panic
passes straight through every function on this page.

## Throwing

| | |
| --- | --- |
| `Throw(err)` | throw an error; a nil error does nothing |
| `Throwf(format, args...)` | throw a formatted error; `%w` works as in `fmt.Errorf` |
| `Must(v, err)` | return `v`, or throw `err` if it is not nil |
| `Check(err)` | throw `err` if it is not nil, for functions returning only an error |

`Must` is how existing Go functions join in:

```go
data := tau.Must(os.ReadFile(path))
tau.Check(os.Remove(tmp))
```

`Must` takes a value and an error. For a function that returns two values
and an error, assign and use `Check`:

```go
out, details, err := program.Eval(vars)
tau.Check(err)
```

## Catching

| | |
| --- | --- |
| `defer Catch(&err)` | thrown error becomes the function's named `err` result |
| `Try(fn)` | run `fn`, return what it threw or nil |
| `TryValue(fn)` | run `fn`, return its result and what it threw |
| `defer Handle(func(e *MyError) {...})` | catch one error type |
| `defer HandleIs(ErrX, func(error) {...})` | catch a sentinel error |

`Catch` is the usual one, on exported functions:

```go
func Load(id string) (u User, err error) {
	defer tau.Catch(&err)
	return load(id), nil
}
```

Use blank names for the other results (`(_ User, err error)`) when the
function builds its result in steps, so a throw part-way never returns a
half-built value.

`Handle` is for recovering from one kind of failure and letting the rest
continue:

```go
func settingOr(key string, fallback int) (n int) {
	defer tau.Handle(func(*NotFoundError) { n = fallback })
	return setting(key)
}
```

Several deferred `Handle` calls run last-in-first-out, so a catch-all would
have to be written first. When there is more than one case, it reads
better to convert to a value with `TryValue` and use an ordinary `switch`
with `errors.Is` and `errors.As`.

## Adding context

`defer tau.Wrapf` prefixes whatever is thrown through a function, the way
`fmt.Errorf("...: %w", err)` does at each return:

```go
func evalMoney(ruleID, key string) Num {
	defer tau.Wrapf("eval %s %s", ruleID, key)
	out := tau.Must(run(ruleID + "/" + key))
	return tau.Must(round(tau.Must(toNumber(out))))
}
```

All three failures come out as `eval <rule> <key>: <cause>`.

## Stacks

A thrown error remembers where it was first thrown.

```go
log.Printf("request failed: %+v", err) // message, then the stack
stack := tau.StackOf(err)              // "" if err was never thrown
```

Re-throwing keeps the original stack, and so does `Wrapf`. The stack ends
at the goroutine that threw: for an error from inside `MapAsync` it shows
the function you passed, not the code that called `MapAsync`.

## Thrown errors and ordinary errors

What `Catch` and `Try` give you is a `*tau.Error` wrapping the original.
It prints the same message and unwraps to the original, so `errors.Is` and
`errors.As` behave as usual:

```go
err := tau.Try(func() { tau.Throw(&NotFoundError{"k"}) })
errors.As(err, &notFound) // true
err.Error()               // "not found: k"
```

Two things do **not** work on it, because it is a wrapper: comparing with
`==` (`err == ErrNotFound`) and a direct type assertion
(`err.(*NotFoundError)`). Use `errors.Is` and `errors.As`.

## Cost

A throw is a panic plus a recorded stack: roughly one to two microseconds,
against a few nanoseconds for a returned error. See
[When to use tau](when-to-use.md).
