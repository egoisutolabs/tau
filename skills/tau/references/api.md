# tau API reference

`import "github.com/egoisutolabs/tau"`. Functions that "throw" raise an
error caught by `Catch`, `Try`, `TryValue`, `Handle`, `HandleIs` or `Wrapf`.

## Throwing and catching

```go
func Throw(err error)                    // nil does nothing
func Throwf(format string, args ...any)  // %w works
func Must[T any](v T, err error) T       // v, or throws err
func Check(err error)                    // throws err if not nil

func Catch(err *error)                             // defer; sets the named error result
func Try(fn func()) (err error)
func TryValue[T any](fn func() T) (v T, err error)
func Handle[E error](handler func(E))              // defer; catches one error type
func HandleIs(target error, handler func(error))   // defer; catches errors.Is(err, target)
func Wrapf(format string, args ...any)             // defer; prefixes whatever is thrown through

func StackOf(err error) string  // stack of the first throw, "" if never thrown
type Error struct{ ... }        // what is caught: wraps the original, carries the stack
func (e *Error) Error() string
func (e *Error) Unwrap() error
func (e *Error) Stack() string
func (e *Error) Format(s fmt.State, verb rune) // %+v prints message then stack
```

## Promises

```go
func Async[T any](ctx context.Context, fn func(ctx context.Context) T) *Promise[T]

func (p *Promise[T]) Await(ctx context.Context) T            // throws the error; stops waiting if ctx ends
func (p *Promise[T]) Result(ctx context.Context) (T, error)
func (p *Promise[T]) Cancel()
func (p *Promise[T]) Done() <-chan struct{}
```

## Task groups

```go
func WithScope[T any](ctx context.Context, fn func(s *Scope) T) T

func (s *Scope) Go[T any](fn func(ctx context.Context) T) *Task[T]
func (s *Scope) Context() context.Context

func (t *Task[T]) Await() T              // uses the scope's context
func (t *Task[T]) Result() (T, error)
```

A failing task, or `fn` throwing, cancels the rest at once. `WithScope`
returns only after every task has returned, then throws the first failure
or returns `fn`'s result. A task that is never awaited still runs to
completion.

## Combinators

```go
func All[T any](ctx context.Context, fns ...func(ctx context.Context) T) []T
func AllSettled[T any](ctx context.Context, fns ...func(ctx context.Context) T) []Settled[T]
func Race[T any](ctx context.Context, fns ...func(ctx context.Context) T) T
func Any[T any](ctx context.Context, fns ...func(ctx context.Context) T) T

type Settled[T any] struct {
	Value T
	Err   error
}
func (s Settled[T]) OK() bool

func Timeout[T any](ctx context.Context, d time.Duration, fn func(ctx context.Context) T) T
var ErrTimeout // also matches context.DeadlineExceeded

func Sleep(ctx context.Context, d time.Duration) // throws the context's error if it ends
```

| | Cancels | Returns |
| --- | --- | --- |
| `All`, `MapAsync`, `ForEachAsync`, `WithScope` | the rest, on the first failure | after everything it started has returned |
| `AllSettled`, `MapSettled` | nothing | after everything has returned |
| `Race`, `Any` | the rest, once it has its answer | at once |
| `Timeout` | the function, at the deadline | at once |

## Retries

```go
func Retry[T any](ctx context.Context, attempts int, delay time.Duration,
	fn func(ctx context.Context, attempt int) T) T // delay doubles each time
func RetryWith[T any](ctx context.Context, policy RetryPolicy,
	fn func(ctx context.Context, attempt int) T) T
func Permanent(err error) error // marks an error as never retried

type RetryPolicy struct {
	Attempts int              // total tries; less than 1 means 1
	Delay    time.Duration    // before the second try
	MaxDelay time.Duration    // cap; 0 means none
	Factor   float64          // 0 means 2; 1 keeps the wait constant
	Jitter   float64          // 0 to 1; a wait d becomes random in [d*(1-Jitter), d]
	RetryIf  func(err error) bool // nil retries everything except Permanent
}
```

`attempt` starts at 1.

## Lists

```go
type List[T any] []T

func (l List[T]) Map[U any](fn func(T) U) List[U]
func (l List[T]) FlatMap[U any](fn func(T) []U) List[U]
func (l List[T]) Filter(fn func(T) bool) List[T]
func (l List[T]) Reject(fn func(T) bool) List[T]
func (l List[T]) Reduce[A any](initial A, fn func(acc A, v T) A) A
func (l List[T]) Sort(less func(a, b T) bool) List[T] // stable; returns a copy
func (l List[T]) Reverse() List[T]
func (l List[T]) ForEach(fn func(T)) List[T]

func (l List[T]) GroupBy[K comparable](key func(T) K) map[K]List[T]
func (l List[T]) KeyBy[K comparable](key func(T) K) map[K]T
func (l List[T]) UniqBy[K comparable](key func(T) K) List[T]
func (l List[T]) Partition(fn func(T) bool) (matched, rest List[T])
func (l List[T]) Chunk(size int) []List[T] // throws if size < 1
func (l List[T]) Take(n int) List[T]
func (l List[T]) Drop(n int) List[T]

func (l List[T]) Find(fn func(T) bool) (T, bool)
func (l List[T]) First() (T, bool)
func (l List[T]) Last() (T, bool)
func (l List[T]) Some(fn func(T) bool) bool
func (l List[T]) Every(fn func(T) bool) bool
func (l List[T]) Count(fn func(T) bool) int

func (l List[T]) MapAsync[U any](ctx context.Context, limit int, fn func(ctx context.Context, v T) U) List[U]
func (l List[T]) MapSettled[U any](ctx context.Context, limit int, fn func(ctx context.Context, v T) U) List[Settled[U]]
func (l List[T]) ForEachAsync(ctx context.Context, limit int, fn func(ctx context.Context, v T))

func Uniq[T comparable](l []T) List[T]
func Keys[K comparable, V any](m map[K]V) List[K]   // no particular order
func Values[K comparable, V any](m map[K]V) List[V] // no particular order
```

`limit <= 0` means no limit. `MapSettled` never throws; if `ctx` ends,
elements not yet started get the context's error as their outcome.

## JSON and validation

```go
func ParseJSON[T any](data []byte) T
func ReadJSON[T any](r io.Reader, maxBytes int64) T // maxBytes must be positive
func Validate(v any)                                // tags, then the Validate() error method
var ErrTooLarge

type InputError struct {
	Kind InputKind
	Path string // JSON Pointer such as "/user/age", when known
	Err  error
}
func (e *InputError) Error() string
func (e *InputError) Unwrap() error

const (
	InputMalformed    InputKind = "malformed"     // syntax, duplicate key, invalid UTF-8, trailing data
	InputUnknownField InputKind = "unknown_field"
	InputWrongType    InputKind = "wrong_type"
	InputInvalid      InputKind = "invalid"       // failed validation
	InputTooLarge     InputKind = "too_large"     // also matches ErrTooLarge
)

type ValidationError struct{ Fields []FieldError }
func (e *ValidationError) Error() string
type FieldError struct {
	Field string // JSON path: "address.city", "tags[2]"
	Rule  string // "required", "gte"
	Param string // "18"; empty if none
	Value any
}
func (f FieldError) String() string
```

`ParseJSON` and `ReadJSON` throw `*InputError`. `Validate` called directly
throws `*ValidationError`, or the `Validate()` method's own error. A reader
failure in `ReadJSON` is thrown as it is, not as an `InputError`.

## Field (PATCH requests)

```go
type Field[T any] struct{ ... } // zero value is "omitted"
func FieldOf[T any](v T) Field[T]
func NullField[T any]() Field[T]

func (f Field[T]) IsOmitted() bool
func (f Field[T]) IsNull() bool
func (f Field[T]) Get() (T, bool)       // value, and whether there is one
func (f Field[T]) OrElse(current T) T   // value; zero if null; current if omitted
func (f Field[T]) IsZero() bool         // for the json `omitzero` option
func (f *Field[T]) UnmarshalJSONFrom(dec *jsontext.Decoder) error
func (f Field[T]) MarshalJSONTo(enc *jsontext.Encoder) error
```

Tag as `json:"name,omitzero" validate:"omitnil,<rules>"` or
`validate:"required,<rules>"`.

## Untyped JSON

```go
func Dig[T any](data any, path ...any) (T, bool)      // string steps are keys, int steps are indexes
func DigOr[T any](data any, fallback T, path ...any) T
```

A whole `float64` converts to any integer type asked for.

## Ordered map and set

```go
func NewMap[K comparable, V any]() *OrderedMap[K, V]
func (m *OrderedMap[K, V]) Set(key K, value V) *OrderedMap[K, V]
func (m *OrderedMap[K, V]) Get(key K) (V, bool)
func (m *OrderedMap[K, V]) GetOr(key K, fallback V) V
func (m *OrderedMap[K, V]) Has(key K) bool
func (m *OrderedMap[K, V]) Delete(key K) bool
func (m *OrderedMap[K, V]) Len() int
func (m *OrderedMap[K, V]) Clear()
func (m *OrderedMap[K, V]) Keys() List[K]
func (m *OrderedMap[K, V]) Values() List[V]
func (m *OrderedMap[K, V]) All() iter.Seq2[K, V]

func NewSet[T comparable](values ...T) *Set[T]
func (s *Set[T]) Add(values ...T) *Set[T]
func (s *Set[T]) Has(v T) bool
func (s *Set[T]) Delete(v T) bool
func (s *Set[T]) Len() int
func (s *Set[T]) Values() List[T]
func (s *Set[T]) All() iter.Seq[T]
func (s *Set[T]) Union(other *Set[T]) *Set[T]
func (s *Set[T]) Intersection(other *Set[T]) *Set[T]
func (s *Set[T]) Difference(other *Set[T]) *Set[T]
func (s *Set[T]) IsSubsetOf(other *Set[T]) bool
```

Both keep insertion order; neither is safe for concurrent use.

## Small helpers

```go
func Ptr[T any](v T) *T
func Deref[T any](p *T, fallback T) T
func If[T any](cond bool, a, b T) T // evaluates both a and b
```
