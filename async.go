package tau

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"sync"
	"time"
)

// Cancellation model
//
// Every function here takes the caller's context first and hands a derived
// context to the work it starts. Cancelling the caller's context cancels that
// work, and each function also cancels work it no longer needs:
//
//   - All, AllSettled, MapAsync, ForEachAsync, MapSettled and WithScope are
//     joins. They return only after every function they started has
//     returned, so no work outlives the call.
//   - Race, Any and Timeout are selects. They cancel the work they no longer
//     need and return at once, without waiting for it to stop.
//   - Async hands ownership to the caller: the promise's Cancel stops it.
//
// Cancellation is cooperative, as everywhere in Go: a function that ignores
// its context keeps running until it finishes.

// Promise is the eventual result of a function running in its own goroutine.
// Create one with Async.
type Promise[T any] struct {
	done   chan struct{}
	cancel context.CancelFunc
	val    T
	err    error
}

// Async runs fn in a new goroutine and returns a promise for its result. fn
// receives a context that is cancelled when ctx is, when the promise's Cancel
// is called, or when fn itself returns. An error thrown by fn rejects the
// promise. An ordinary panic in fn is not caught and crashes the program, as
// in any goroutine.
//
// An async function, in the JavaScript sense, is an ordinary function that
// returns Async. Awaiting inside it blocks only its own goroutine:
//
//	func loadPage(ctx context.Context, id string) *tau.Promise[Page] {
//		return tau.Async(ctx, func(ctx context.Context) Page {
//			user := fetchUser(ctx, id)   // both start now
//			posts := fetchPosts(ctx, id) // and run at the same time
//			return render(user.Await(ctx), posts.Await(ctx))
//		})
//	}
//
// Because fn's context is cancelled when fn returns or throws, work it
// started with that context and did not await is cancelled too: nothing an
// async function starts outlives it. For the same reason a promise started
// inside cannot be handed out as the result; await it and return the value.
func Async[T any](ctx context.Context, fn func(ctx context.Context) T) *Promise[T] {
	ctx, cancel := context.WithCancel(ctx)
	p := &Promise[T]{done: make(chan struct{}), cancel: cancel}
	go func() {
		defer close(p.done)
		defer cancel()
		defer Catch(&p.err)
		p.val = fn(ctx)
	}()
	return p
}

// Await waits for the promise and returns its value, or throws its error. If
// ctx ends first, Await throws the context's error and stops waiting; the
// promise's work carries on unless it is cancelled too.
func (p *Promise[T]) Await(ctx context.Context) T {
	select {
	case <-p.done:
	case <-ctx.Done():
		select {
		case <-p.done: // settled at the same moment: prefer the result
		default:
			Throw(context.Cause(ctx))
		}
	}
	Check(p.err)
	return p.val
}

// Result is Await that returns the error instead of throwing it.
func (p *Promise[T]) Result(ctx context.Context) (T, error) {
	return TryValue(func() T { return p.Await(ctx) })
}

// Cancel cancels the context of the promise's function. It does not wait for
// the function to return; Await does.
func (p *Promise[T]) Cancel() { p.cancel() }

// Done returns a channel that is closed when the promise settles, for use in
// a select.
func (p *Promise[T]) Done() <-chan struct{} { return p.done }

// All runs every function concurrently and returns their results in order.
// If one throws, the others' context is cancelled, All waits for them to
// return, and then throws the first error.
func All[T any](ctx context.Context, fns ...func(ctx context.Context) T) []T {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	out := make([]T, len(fns))
	var (
		wg    sync.WaitGroup
		once  sync.Once
		first error
	)
	for i, fn := range fns {
		wg.Go(func() {
			v, err := TryValue(func() T { return fn(ctx) })
			if err != nil {
				once.Do(func() { first = err; cancel(err) })
				return
			}
			out[i] = v
		})
	}
	wg.Wait()
	Check(first)
	return out
}

// Settled is the outcome of one function, as returned by AllSettled.
type Settled[T any] struct {
	Value T
	Err   error
}

// OK reports whether the function returned without throwing.
func (s Settled[T]) OK() bool { return s.Err == nil }

// AllSettled runs every function concurrently, waits for all of them, and
// returns each outcome in order. It never throws.
func AllSettled[T any](ctx context.Context, fns ...func(ctx context.Context) T) []Settled[T] {
	out := make([]Settled[T], len(fns))
	var wg sync.WaitGroup
	for i, fn := range fns {
		wg.Go(func() {
			out[i].Value, out[i].Err = TryValue(func() T { return fn(ctx) })
		})
	}
	wg.Wait()
	return out
}

// Race runs every function concurrently and returns the result of the first
// to finish, or throws its error. The others' context is cancelled; Race does
// not wait for them.
func Race[T any](ctx context.Context, fns ...func(ctx context.Context) T) T {
	if len(fns) == 0 {
		Throw(errors.New("tau: Race needs at least one function"))
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := launch(ctx, fns)
	select {
	case r := <-results:
		Check(r.Err)
		return r.Value
	case <-ctx.Done():
		Throw(context.Cause(ctx))
		panic("unreachable")
	}
}

// Any runs every function concurrently and returns the first result that is
// not an error, cancelling the rest without waiting for them. If every
// function throws, Any throws all of their errors joined together.
func Any[T any](ctx context.Context, fns ...func(ctx context.Context) T) T {
	if len(fns) == 0 {
		Throw(errors.New("tau: Any needs at least one function"))
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := launch(ctx, fns)
	var errs []error
	for range fns {
		select {
		case r := <-results:
			if r.OK() {
				return r.Value
			}
			errs = append(errs, r.Err)
		case <-ctx.Done():
			Throw(context.Cause(ctx))
		}
	}
	Throw(errors.Join(errs...))
	panic("unreachable")
}

// launch runs fns concurrently and delivers each outcome on a channel with
// room for all of them, so no goroutine blocks if the receiver has left.
func launch[T any](ctx context.Context, fns []func(ctx context.Context) T) <-chan Settled[T] {
	results := make(chan Settled[T], len(fns))
	for _, fn := range fns {
		go func() {
			v, err := TryValue(func() T { return fn(ctx) })
			results <- Settled[T]{v, err}
		}()
	}
	return results
}

// MapAsync applies fn to every element concurrently, running at most limit
// calls at a time (limit <= 0 means no limit), and returns the results in
// order. If a call throws, the others' context is cancelled, no further calls
// start, and the first error is thrown once the running calls have returned.
func (l List[T]) MapAsync[U any](ctx context.Context, limit int, fn func(ctx context.Context, v T) U) List[U] {
	if limit <= 0 || limit > len(l) {
		limit = len(l)
	}
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	out := make(List[U], len(l))
	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		next  int
		first error
	)
	for range limit {
		wg.Go(func() {
			for {
				// Claim the next element only if it will actually run, so
				// next counts started elements and nothing else.
				mu.Lock()
				if first != nil || next >= len(l) || ctx.Err() != nil {
					mu.Unlock()
					return
				}
				i := next
				next++
				mu.Unlock()
				v, err := TryValue(func() U { return fn(ctx, l[i]) })
				if err != nil {
					mu.Lock()
					if first == nil {
						first = err
						cancel(err)
					}
					mu.Unlock()
					return
				}
				out[i] = v
			}
		})
	}
	wg.Wait()
	Check(first)
	// The caller's context ended before every element was started.
	if next < len(l) {
		Throw(context.Cause(ctx))
	}
	return out
}

// MapSettled applies fn to every element concurrently, running at most limit
// calls at a time (limit <= 0 means no limit), and returns every outcome in
// order. Unlike MapAsync, one element failing does not stop or cancel the
// others, and MapSettled never throws: it is for batches where each element
// succeeds or fails on its own.
//
// If ctx ends, the calls already running are cancelled through their context
// and the elements not yet started are not run; each of those gets the
// context's error as its outcome. MapSettled returns after every call it
// started has returned.
func (l List[T]) MapSettled[U any](ctx context.Context, limit int, fn func(ctx context.Context, v T) U) List[Settled[U]] {
	if limit <= 0 || limit > len(l) {
		limit = len(l)
	}
	out := make(List[Settled[U]], len(l))
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		next int
	)
	for range limit {
		wg.Go(func() {
			for {
				mu.Lock()
				i := next
				next++
				mu.Unlock()
				if i >= len(l) {
					return
				}
				if err := context.Cause(ctx); err != nil {
					out[i].Err = err
					continue
				}
				out[i].Value, out[i].Err = TryValue(func() U { return fn(ctx, l[i]) })
			}
		})
	}
	wg.Wait()
	return out
}

// ForEachAsync calls fn for every element concurrently, running at most
// limit calls at a time (limit <= 0 means no limit). It is MapAsync for work
// done for its effect, such as sending or uploading, and follows the same
// rules: if a call throws, the others' context is cancelled, no further
// calls start, and the first error is thrown once the running calls have
// returned.
func (l List[T]) ForEachAsync(ctx context.Context, limit int, fn func(ctx context.Context, v T)) {
	l.MapAsync(ctx, limit, func(ctx context.Context, v T) struct{} {
		fn(ctx, v)
		return struct{}{}
	})
}

// Sleep waits for d, or throws the context's error as soon as ctx ends. It
// throws at once if ctx has already ended, even for a zero d.
func Sleep(ctx context.Context, d time.Duration) {
	Check(context.Cause(ctx))
	if d <= 0 {
		return
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		Throw(context.Cause(ctx))
	}
}

// ErrTimeout is thrown by Timeout when the time runs out. It also matches
// context.DeadlineExceeded.
var ErrTimeout = fmt.Errorf("tau: timed out: %w", context.DeadlineExceeded)

// Timeout runs fn and returns its result, or throws ErrTimeout if it takes
// longer than d. fn's context is cancelled at the deadline or when ctx is;
// Timeout then returns at once, so a fn that ignores its context keeps
// running in the background. If ctx ends first, its error is thrown instead.
func Timeout[T any](ctx context.Context, d time.Duration, fn func(ctx context.Context) T) T {
	ctx, cancel := context.WithTimeoutCause(ctx, d, ErrTimeout)
	defer cancel()
	p := Async(ctx, fn)
	select {
	case <-p.done:
		// A fn that honoured the deadline reports it as its own error; give
		// the caller the same ErrTimeout either way.
		if p.err != nil && context.Cause(ctx) == ErrTimeout && errors.Is(p.err, context.DeadlineExceeded) {
			Throw(ErrTimeout)
		}
		Check(p.err)
		return p.val
	case <-ctx.Done():
		Throw(context.Cause(ctx))
		panic("unreachable")
	}
}

type permanent struct{ err error }

func (p *permanent) Error() string { return p.err.Error() }
func (p *permanent) Unwrap() error { return p.err }

// Permanent marks err as not worth retrying: Retry and RetryWith rethrow it
// at once instead of trying again. It reads and unwraps like err.
//
//	if errors.As(err, &notFound) {
//		tau.Throw(tau.Permanent(err))
//	}
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return &permanent{err}
}

// RetryPolicy says how RetryWith retries. The zero value tries once.
type RetryPolicy struct {
	// Attempts is the total number of tries, including the first. Less than
	// one means one.
	Attempts int
	// Delay is the wait before the second try. Each later wait is the
	// previous one multiplied by Factor.
	Delay time.Duration
	// MaxDelay caps the wait between tries. Zero means no cap.
	MaxDelay time.Duration
	// Factor multiplies the wait after each try. Zero means 2; use 1 for a
	// constant wait.
	Factor float64
	// Jitter randomises each wait so that many clients do not retry in step.
	// A wait of d becomes a random time between d*(1-Jitter) and d. It is
	// clamped to the range 0 to 1; zero means no jitter.
	Jitter float64
	// RetryIf decides whether an error is worth another try. Nil means every
	// error is, except one marked with Permanent, which never is.
	RetryIf func(err error) bool
}

// wait returns the pause after the given number of failed tries (1 for the
// first). rnd is a random number in [0, 1).
func (p RetryPolicy) wait(failures int, rnd float64) time.Duration {
	factor := p.Factor
	if factor <= 0 {
		factor = 2
	}
	d := float64(p.Delay)
	for range failures - 1 {
		d *= factor
		if p.MaxDelay > 0 && d >= float64(p.MaxDelay) {
			break
		}
	}
	if p.MaxDelay > 0 {
		d = min(d, float64(p.MaxDelay))
	}
	d -= d * min(max(p.Jitter, 0), 1) * rnd
	return time.Duration(d)
}

// RetryWith calls fn until it returns without throwing, as allowed by
// policy, and returns its result. fn receives the attempt number, from 1.
// The last error is thrown when the attempts run out, when policy.RetryIf
// rejects an error, or when an error is marked with Permanent. If ctx ends,
// RetryWith stops, including in the middle of a wait, and throws the
// context's error.
//
// A policy is a plain value, so one can be declared once and shared:
//
//	var paymentsRetry = tau.RetryPolicy{
//		Attempts: 5, Delay: 100 * time.Millisecond, MaxDelay: 2 * time.Second,
//		Jitter: 0.5, RetryIf: isTemporary,
//	}
//	receipt := tau.RetryWith(ctx, paymentsRetry, charge)
func RetryWith[T any](ctx context.Context, policy RetryPolicy, fn func(ctx context.Context, attempt int) T) T {
	attempts := max(policy.Attempts, 1)
	var last error
	for attempt := 1; attempt <= attempts; attempt++ {
		Check(context.Cause(ctx))
		v, err := TryValue(func() T { return fn(ctx, attempt) })
		if err == nil {
			return v
		}
		if _, stop := errors.AsType[*permanent](err); stop {
			Throw(err)
		}
		if policy.RetryIf != nil && !policy.RetryIf(err) {
			Throw(err)
		}
		last = err
		if attempt < attempts {
			Sleep(ctx, policy.wait(attempt, rand.Float64()))
		}
	}
	Throw(last)
	panic("unreachable")
}

// Retry is RetryWith for the common case: at most attempts tries, waiting
// delay before the second and doubling the wait each time after. Every error
// is retried except one marked with Permanent.
func Retry[T any](ctx context.Context, attempts int, delay time.Duration, fn func(ctx context.Context, attempt int) T) T {
	return RetryWith(ctx, RetryPolicy{Attempts: attempts, Delay: delay}, fn)
}
