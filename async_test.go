package tau

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var bg = context.Background()

// after returns a function that waits d, honouring cancellation, then
// returns v. stopped counts how many calls were cut short by cancellation.
func after[T any](d time.Duration, v T, stopped *atomic.Int32) func(context.Context) T {
	return func(ctx context.Context) T {
		select {
		case <-time.After(d):
			return v
		case <-ctx.Done():
			if stopped != nil {
				stopped.Add(1)
			}
			Throw(context.Cause(ctx))
			panic("unreachable")
		}
	}
}

func failing[T any](d time.Duration, err error) func(context.Context) T {
	return func(context.Context) T { time.Sleep(d); Throw(err); panic("unreachable") }
}

func TestAsyncAwait(t *testing.T) {
	a := Async(bg, after(5*time.Millisecond, 20, nil))
	b := Async(bg, after(time.Millisecond, "1", nil))
	if got := strconv.Itoa(a.Await(bg)) + b.Await(bg); got != "201" {
		t.Fatalf("got %q", got)
	}
	_, err := Async(bg, failing[int](0, errBoom)).Result(bg)
	if !errors.Is(err, errBoom) || !strings.Contains(StackOf(err), "async_test.go") {
		t.Fatalf("got %v with stack %q", err, StackOf(err))
	}
}

func TestAwaitStopsWhenCallerCancels(t *testing.T) {
	p := Async(bg, after(time.Hour, 1, nil))
	defer p.Cancel()
	ctx, cancel := context.WithTimeout(bg, 10*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := p.Result(ctx)
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > time.Second {
		t.Fatalf("got %v after %v", err, time.Since(start))
	}
	select {
	case <-p.Done():
		t.Fatal("abandoning an Await must not cancel the work")
	default:
	}
}

func TestCancelStopsTheWork(t *testing.T) {
	var stopped atomic.Int32
	p := Async(bg, after(time.Hour, 1, &stopped))
	p.Cancel()
	if _, err := p.Result(bg); !errors.Is(err, context.Canceled) || stopped.Load() != 1 {
		t.Fatalf("got %v, stopped %d", err, stopped.Load())
	}
	parent, cancel := context.WithCancelCause(bg)
	q := Async(parent, after(time.Hour, 1, &stopped))
	cancel(errBoom)
	if _, err := q.Result(bg); !errors.Is(err, errBoom) {
		t.Fatalf("cancelling the caller's context should stop the work with its cause, got %v", err)
	}
}

func TestAll(t *testing.T) {
	got := All(bg, after(20*time.Millisecond, "a", nil), after(time.Millisecond, "b", nil), after(0, "c", nil))
	if strings.Join(got, "") != "abc" {
		t.Fatalf("got %v", got)
	}
	if len(All[int](bg)) != 0 {
		t.Fatal("All of nothing should be empty")
	}
	var stopped atomic.Int32
	start := time.Now()
	err := Try(func() {
		All(bg, after(time.Hour, 1, &stopped), after(time.Hour, 2, &stopped), failing[int](time.Millisecond, errBoom))
	})
	if !errors.Is(err, errBoom) || time.Since(start) > time.Second {
		t.Fatalf("got %v after %v", err, time.Since(start))
	}
	if stopped.Load() != 2 {
		t.Fatalf("All returned with %d of 2 siblings stopped; it must cancel and wait for them", stopped.Load())
	}
}

func TestAllStopsWhenCallerCancels(t *testing.T) {
	ctx, cancel := context.WithTimeout(bg, 10*time.Millisecond)
	defer cancel()
	var stopped atomic.Int32
	err := Try(func() { All(ctx, after(time.Hour, 1, &stopped), after(time.Hour, 2, &stopped)) })
	if !errors.Is(err, context.DeadlineExceeded) || stopped.Load() != 2 {
		t.Fatalf("got %v, stopped %d", err, stopped.Load())
	}
}

func TestAllSettled(t *testing.T) {
	settled := AllSettled(bg, after(time.Millisecond, 1, nil), failing[int](0, errBoom))
	if !settled[0].OK() || settled[0].Value != 1 || settled[1].OK() || !errors.Is(settled[1].Err, errBoom) {
		t.Fatalf("got %+v", settled)
	}
}

func TestRace(t *testing.T) {
	var stopped atomic.Int32
	got := Race(bg, after(time.Hour, "slow", &stopped), after(time.Millisecond, "fast", nil))
	if got != "fast" {
		t.Fatalf("got %q", got)
	}
	waitFor(t, "the loser to be cancelled", func() bool { return stopped.Load() == 1 })
	if err := Try(func() { Race(bg, after(time.Hour, 1, nil), failing[int](0, errBoom)) }); !errors.Is(err, errBoom) {
		t.Fatalf("Race should throw the first failure, got %v", err)
	}
	ctx, cancel := context.WithTimeout(bg, 10*time.Millisecond)
	defer cancel()
	ignores := func(context.Context) int { time.Sleep(300 * time.Millisecond); return 1 }
	start := time.Now()
	if err := Try(func() { Race(ctx, ignores) }); !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 200*time.Millisecond {
		t.Fatalf("Race should return when the caller's context ends, got %v after %v", err, time.Since(start))
	}
	if Try(func() { Race[int](bg) }) == nil {
		t.Fatal("Race of nothing should throw")
	}
}

func TestAny(t *testing.T) {
	var stopped atomic.Int32
	got := Any(bg, failing[string](0, errBoom), after(5*time.Millisecond, "ok", nil), after(time.Hour, "late", &stopped))
	if got != "ok" {
		t.Fatalf("got %q", got)
	}
	waitFor(t, "the unneeded call to be cancelled", func() bool { return stopped.Load() == 1 })
	other := errors.New("other")
	err := Try(func() { Any(bg, failing[int](0, errBoom), failing[int](0, other)) })
	if !errors.Is(err, errBoom) || !errors.Is(err, other) {
		t.Fatalf("Any should join every error, got %v", err)
	}
	if Try(func() { Any[int](bg) }) == nil {
		t.Fatal("Any of nothing should throw")
	}
}

// untilStarted blocks until n calls have registered in started. Tests use it
// so that a deliberate failure happens only after its siblings are running;
// otherwise a sibling that has not started yet is, correctly, never run, and
// a count of cancelled siblings would depend on scheduling.
func untilStarted(started *atomic.Int32, n int32) {
	for started.Load() < n {
		time.Sleep(100 * time.Microsecond)
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestMapAsync(t *testing.T) {
	var running, peak atomic.Int32
	in := List[int]{1, 2, 3, 4, 5, 6, 7, 8}
	out := in.MapAsync(bg, 3, func(_ context.Context, n int) string {
		now := running.Add(1)
		for {
			old := peak.Load()
			if now <= old || peak.CompareAndSwap(old, now) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		running.Add(-1)
		return strconv.Itoa(n * n)
	})
	if strings.Join(out, ",") != "1,4,9,16,25,36,49,64" {
		t.Fatalf("got %v", out)
	}
	if peak.Load() > 3 || peak.Load() < 2 {
		t.Fatalf("ran %d at once, want at most 3", peak.Load())
	}
	var calls atomic.Int32
	err := Try(func() {
		in.MapAsync(bg, 1, func(_ context.Context, n int) int {
			calls.Add(1)
			if n == 2 {
				Throw(errBoom)
			}
			return n
		})
	})
	if !errors.Is(err, errBoom) || calls.Load() != 2 {
		t.Fatalf("got %v after %d calls, want boom after 2", err, calls.Load())
	}
	itoa := func(_ context.Context, n int) string { return strconv.Itoa(n) }
	if len(List[int]{}.MapAsync(bg, 4, itoa)) != 0 {
		t.Fatal("MapAsync of an empty list should be empty")
	}
	if got := in.MapAsync(bg, 0, itoa); len(got) != 8 || got[7] != "8" {
		t.Fatalf("unlimited MapAsync got %v", got)
	}
}

func TestMapAsyncCancellation(t *testing.T) {
	in := List[int]{1, 2, 3, 4, 5, 6}
	var started, stopped atomic.Int32
	err := Try(func() {
		in.MapAsync(bg, 6, func(ctx context.Context, n int) int {
			if n == 1 {
				untilStarted(&started, 5)
				Throw(errBoom)
			}
			started.Add(1)
			return after(time.Hour, n, &stopped)(ctx)
		})
	})
	if !errors.Is(err, errBoom) || stopped.Load() != 5 {
		t.Fatalf("got %v with %d of 5 siblings stopped", err, stopped.Load())
	}
	ctx, cancel := context.WithCancel(bg)
	var calls atomic.Int32
	err = Try(func() {
		in.MapAsync(ctx, 1, func(_ context.Context, n int) int {
			if calls.Add(1) == 2 {
				cancel() // the caller gives up part-way; this call still succeeds
			}
			return n
		})
	})
	if !errors.Is(err, context.Canceled) || calls.Load() != 2 {
		t.Fatalf("got %v after %d calls; a cancelled MapAsync must not return partial results", err, calls.Load())
	}
}

func TestTimeout(t *testing.T) {
	if got := Timeout(bg, time.Second, func(context.Context) int { return 5 }); got != 5 {
		t.Fatalf("got %d", got)
	}
	var stopped atomic.Int32
	err := Try(func() { Timeout(bg, 10*time.Millisecond, after(time.Hour, 0, &stopped)) })
	if !errors.Is(err, ErrTimeout) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v", err)
	}
	waitFor(t, "the function's context to be cancelled", func() bool { return stopped.Load() == 1 })

	ignores := func(context.Context) int { time.Sleep(300 * time.Millisecond); return 1 }
	start := time.Now()
	if err := Try(func() { Timeout(bg, 10*time.Millisecond, ignores) }); !errors.Is(err, ErrTimeout) || time.Since(start) > 200*time.Millisecond {
		t.Fatalf("Timeout should not wait for a function that ignores its context, got %v after %v", err, time.Since(start))
	}
	if err := Try(func() { Timeout(bg, time.Second, failing[int](0, errBoom)) }); !errors.Is(err, errBoom) {
		t.Fatalf("got %v", err)
	}
	parent, cancel := context.WithCancelCause(bg)
	cancel(errBoom)
	err = Try(func() { Timeout(parent, time.Hour, after(time.Hour, 0, nil)) })
	if !errors.Is(err, errBoom) || errors.Is(err, ErrTimeout) {
		t.Fatalf("a cancelled caller is not a timeout, got %v", err)
	}
}

func TestRetry(t *testing.T) {
	got := Retry(bg, 5, time.Millisecond, func(_ context.Context, attempt int) string {
		if attempt < 3 {
			Throw(errBoom)
		}
		return "ok on " + strconv.Itoa(attempt)
	})
	if got != "ok on 3" {
		t.Fatalf("got %q", got)
	}
	calls := 0
	always := func(context.Context, int) int { calls++; Throw(errBoom); return 0 }
	if err := Try(func() { Retry(bg, 3, time.Millisecond, always) }); !errors.Is(err, errBoom) || calls != 3 {
		t.Fatalf("got %v after %d calls", err, calls)
	}
	calls = 0
	_ = Try(func() { Retry(bg, 0, time.Millisecond, always) })
	if calls != 1 {
		t.Fatalf("Retry with 0 attempts should still try once, tried %d", calls)
	}
}

func TestRetryStopsWhenCallerCancels(t *testing.T) {
	ctx, cancel := context.WithTimeout(bg, 20*time.Millisecond)
	defer cancel()
	calls := 0
	start := time.Now()
	err := Try(func() {
		Retry(ctx, 10, time.Hour, func(context.Context, int) int { calls++; Throw(errBoom); return 0 })
	})
	if !errors.Is(err, context.DeadlineExceeded) || calls != 1 || time.Since(start) > time.Second {
		t.Fatalf("Retry should stop during its wait: got %v after %d calls and %v", err, calls, time.Since(start))
	}
	done, cancelDone := context.WithCancel(bg)
	cancelDone()
	calls = 0
	_ = Try(func() { Retry(done, 3, 0, func(context.Context, int) int { calls++; return 0 }) })
	if calls != 0 {
		t.Fatal("Retry should not start an attempt on a cancelled context")
	}
}

func TestRetryStopsOnPermanentError(t *testing.T) {
	calls := 0
	err := Try(func() {
		Retry(bg, 5, time.Millisecond, func(context.Context, int) int {
			calls++
			Throw(Permanent(&notFound{"k"}))
			return 0
		})
	})
	nf, ok := errors.AsType[*notFound](err)
	if calls != 1 || !ok || nf.key != "k" || err.Error() != "not found: k" {
		t.Fatalf("got %v after %d calls", err, calls)
	}
	if !strings.Contains(StackOf(err), "async_test.go") {
		t.Fatal("the original throw site was lost")
	}
	if Permanent(nil) != nil {
		t.Fatal("Permanent(nil) should be nil")
	}
}

// Async functions are ordinary functions that return Async.

func double(ctx context.Context, n int) *Promise[int] {
	return Async(ctx, func(ctx context.Context) int { return after(50*time.Millisecond, n*2, nil)(ctx) })
}

func labelled(ctx context.Context, prefix string, n int) *Promise[string] {
	return Async(ctx, func(ctx context.Context) string {
		a, b := double(ctx, n), double(ctx, n+1) // both start before either is awaited
		return prefix + strconv.Itoa(a.Await(ctx)+b.Await(ctx))
	})
}

func TestAsyncFunctionsRunConcurrentlyAndNest(t *testing.T) {
	both := Async(bg, func(ctx context.Context) string {
		x, y := labelled(ctx, "x", 1), labelled(ctx, "y", 10)
		return x.Await(ctx) + " " + y.Await(ctx)
	})
	start := time.Now()
	if got := both.Await(bg); got != "x6 y42" {
		t.Fatalf("got %q", got)
	}
	// Run one after another the four calls would take 200ms.
	if elapsed := time.Since(start); elapsed > 150*time.Millisecond {
		t.Fatalf("four 50ms calls took %v; they should overlap", elapsed)
	}
}

func TestAsyncFunctionCancelsAbandonedWork(t *testing.T) {
	var stopped atomic.Int32
	slow := func(ctx context.Context) *Promise[int] { return Async(ctx, after(time.Hour, 0, &stopped)) }
	broken := Async(bg, func(ctx context.Context) int {
		slow(ctx)
		slow(ctx)
		Throw(errBoom)
		return 0
	})
	if _, err := broken.Result(bg); !errors.Is(err, errBoom) {
		t.Fatalf("got %v", err)
	}
	waitFor(t, "abandoned work to be cancelled after a throw", func() bool { return stopped.Load() == 2 })

	early := Async(bg, func(ctx context.Context) int {
		slow(ctx)
		return 1 // returns without awaiting
	})
	if early.Await(bg) != 1 {
		t.Fatal("wrong value")
	}
	waitFor(t, "abandoned work to be cancelled after a return", func() bool { return stopped.Load() == 3 })
}

func TestCallerCancellationReachesNestedAsyncFunctions(t *testing.T) {
	var stopped atomic.Int32
	inner := func(ctx context.Context) *Promise[int] { return Async(ctx, after(time.Hour, 0, &stopped)) }
	ctx, cancel := context.WithTimeout(bg, 10*time.Millisecond)
	defer cancel()
	outer := Async(ctx, func(ctx context.Context) int { return inner(ctx).Await(ctx) + inner(ctx).Await(ctx) })
	if _, err := outer.Result(bg); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v", err)
	}
	// The outer function stops waiting as soon as its context ends, which can
	// be before the inner call has finished unwinding.
	waitFor(t, "the first inner call to be cancelled", func() bool { return stopped.Load() >= 1 })
	time.Sleep(20 * time.Millisecond)
	if stopped.Load() != 1 {
		t.Fatalf("stopped %d, want 1: the second call must not start after the first was cancelled", stopped.Load())
	}
}

func TestMapAsyncOnAnAlreadyCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(bg)
	cancel()
	for _, limit := range []int{1, 3, 0} {
		calls := 0
		got, err := TryValue(func() List[int] {
			return List[int]{1, 2, 3}.MapAsync(ctx, limit, func(context.Context, int) int { calls++; return 9 })
		})
		if !errors.Is(err, context.Canceled) || calls != 0 {
			t.Errorf("limit %d: got %v, %v after %d calls; skipped work must not be reported as success", limit, got, err, calls)
		}
	}
	if got := (List[int]{}).MapAsync(ctx, 1, func(context.Context, int) int { return 0 }); len(got) != 0 {
		t.Error("an empty list has nothing to skip and should succeed")
	}
}

func TestMapSettled(t *testing.T) {
	var running, peak atomic.Int32
	out := List[int]{1, 2, 3, 4, 5, 6}.MapSettled(bg, 2, func(_ context.Context, n int) string {
		peak.Store(max(peak.Load(), running.Add(1)))
		defer running.Add(-1)
		time.Sleep(3 * time.Millisecond)
		if n%3 == 0 {
			Throwf("bad %d", n)
		}
		return strconv.Itoa(n * 10)
	})
	if len(out) != 6 || out[0].Value != "10" || !out[0].OK() || out[2].OK() || out[2].Err.Error() != "bad 3" || out[5].OK() || out[4].Value != "50" {
		t.Fatalf("got %+v", out)
	}
	if peak.Load() > 2 {
		t.Fatalf("ran %d at once, over the limit of 2", peak.Load())
	}
	failures := out.Count(func(s Settled[string]) bool { return !s.OK() })
	if failures != 2 {
		t.Fatalf("one failure must not stop the others: %d failed of 6", failures)
	}
	if len(List[int]{}.MapSettled(bg, 3, func(context.Context, int) int { return 0 })) != 0 {
		t.Fatal("empty list")
	}
}

func TestMapSettledCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(bg)
	var calls atomic.Int32
	out := List[int]{1, 2, 3, 4}.MapSettled(ctx, 1, func(_ context.Context, n int) int {
		if calls.Add(1) == 2 {
			cancel()
		}
		return n
	})
	if calls.Load() != 2 || !out[0].OK() || !out[1].OK() {
		t.Fatalf("the two started calls should have their own outcomes: %+v after %d calls", out, calls.Load())
	}
	for i := 2; i < 4; i++ {
		if !errors.Is(out[i].Err, context.Canceled) {
			t.Fatalf("element %d was never started and should carry the context's error, got %+v", i, out[i])
		}
	}
}

func TestRetryPolicyWaits(t *testing.T) {
	ms := time.Millisecond
	p := RetryPolicy{Delay: 100 * ms}
	for failures, want := range map[int]time.Duration{1: 100 * ms, 2: 200 * ms, 3: 400 * ms, 4: 800 * ms} {
		if got := p.wait(failures, 0.9); got != want {
			t.Errorf("default factor, failure %d: got %v, want %v", failures, got, want)
		}
	}
	capped := RetryPolicy{Delay: 100 * ms, MaxDelay: 250 * ms}
	if capped.wait(2, 0) != 200*ms || capped.wait(3, 0) != 250*ms || capped.wait(60, 0) != 250*ms {
		t.Errorf("MaxDelay: got %v, %v, %v", capped.wait(2, 0), capped.wait(3, 0), capped.wait(60, 0))
	}
	if constant := (RetryPolicy{Delay: 100 * ms, Factor: 1}); constant.wait(5, 0) != 100*ms {
		t.Errorf("Factor 1 should give a constant wait, got %v", constant.wait(5, 0))
	}
	jitter := RetryPolicy{Delay: 100 * ms, Jitter: 0.5}
	if jitter.wait(1, 0) != 100*ms || jitter.wait(1, 0.5) != 75*ms || jitter.wait(1, 0.999) < 50*ms {
		t.Errorf("Jitter 0.5 should give 50ms..100ms, got %v, %v, %v", jitter.wait(1, 0), jitter.wait(1, 0.5), jitter.wait(1, 0.999))
	}
	if wild := (RetryPolicy{Delay: 100 * ms, Jitter: 7}); wild.wait(1, 0.999) < 0 {
		t.Error("Jitter above 1 must be clamped, not go negative")
	}
	if (RetryPolicy{}).wait(3, 0.5) != 0 {
		t.Error("the zero policy waits zero")
	}
}

func TestRetryWith(t *testing.T) {
	temporary := errors.New("temporary")
	policy := RetryPolicy{Attempts: 5, Delay: time.Millisecond, MaxDelay: 2 * time.Millisecond, Jitter: 0.5,
		RetryIf: func(err error) bool { return errors.Is(err, temporary) }}

	calls := 0
	got := RetryWith(bg, policy, func(_ context.Context, attempt int) string {
		calls++
		if attempt < 3 {
			Throw(temporary)
		}
		return "ok"
	})
	if got != "ok" || calls != 3 {
		t.Fatalf("got %q after %d calls", got, calls)
	}

	calls = 0
	err := Try(func() { RetryWith(bg, policy, func(context.Context, int) int { calls++; Throw(errBoom); return 0 }) })
	if !errors.Is(err, errBoom) || calls != 1 {
		t.Fatalf("an error RetryIf rejects must not be retried: %v after %d calls", err, calls)
	}

	calls = 0
	err = Try(func() {
		RetryWith(bg, policy, func(context.Context, int) int { calls++; Throw(Permanent(temporary)); return 0 })
	})
	if !errors.Is(err, temporary) || calls != 1 {
		t.Fatalf("Permanent wins over RetryIf: %v after %d calls", err, calls)
	}

	calls = 0
	err = Try(func() { RetryWith(bg, policy, func(context.Context, int) int { calls++; Throw(temporary); return 0 }) })
	if !errors.Is(err, temporary) || calls != 5 {
		t.Fatalf("got %v after %d calls, want 5", err, calls)
	}

	calls = 0
	_ = Try(func() {
		RetryWith(bg, RetryPolicy{}, func(context.Context, int) int { calls++; Throw(errBoom); return 0 })
	})
	if calls != 1 {
		t.Fatalf("the zero policy tries once, tried %d", calls)
	}

	ctx, cancel := context.WithTimeout(bg, 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	err = Try(func() {
		RetryWith(ctx, RetryPolicy{Attempts: 9, Delay: time.Hour}, func(context.Context, int) int { Throw(temporary); return 0 })
	})
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > time.Second {
		t.Fatalf("RetryWith should stop during its wait: %v after %v", err, time.Since(start))
	}
}

func TestForEachAsync(t *testing.T) {
	var sum, running, peak atomic.Int32
	List[int]{1, 2, 3, 4, 5, 6}.ForEachAsync(bg, 2, func(_ context.Context, n int) {
		peak.Store(max(peak.Load(), running.Add(1)))
		defer running.Add(-1)
		time.Sleep(3 * time.Millisecond)
		sum.Add(int32(n))
	})
	if sum.Load() != 21 || peak.Load() > 2 {
		t.Fatalf("sum %d, peak %d", sum.Load(), peak.Load())
	}
	var started, stopped atomic.Int32
	err := Try(func() {
		List[int]{1, 2, 3}.ForEachAsync(bg, 3, func(ctx context.Context, n int) {
			if n == 1 {
				untilStarted(&started, 2)
				Throw(errBoom)
			}
			started.Add(1)
			after(time.Hour, n, &stopped)(ctx)
		})
	})
	if !errors.Is(err, errBoom) || stopped.Load() != 2 {
		t.Fatalf("got %v with %d of 2 siblings stopped", err, stopped.Load())
	}
	ctx, cancel := context.WithCancel(bg)
	cancel()
	calls := 0
	if err := Try(func() { List[int]{1}.ForEachAsync(ctx, 1, func(context.Context, int) { calls++ }) }); !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatalf("an already cancelled ForEachAsync must throw, got %v after %d calls", err, calls)
	}
	List[int]{}.ForEachAsync(bg, 4, func(context.Context, int) { t.Error("called for an empty list") })
}

func TestSleep(t *testing.T) {
	start := time.Now()
	Sleep(bg, 20*time.Millisecond)
	if took := time.Since(start); took < 20*time.Millisecond || took > 200*time.Millisecond {
		t.Fatalf("slept %v, want about 20ms", took)
	}
	Sleep(bg, 0)
	Sleep(bg, -time.Second)

	ctx, cancel := context.WithCancelCause(bg)
	time.AfterFunc(10*time.Millisecond, func() { cancel(errBoom) })
	start = time.Now()
	err := Try(func() { Sleep(ctx, time.Hour) })
	if !errors.Is(err, errBoom) || time.Since(start) > time.Second {
		t.Fatalf("Sleep should throw the cancellation cause promptly, got %v after %v", err, time.Since(start))
	}
	if err := Try(func() { Sleep(ctx, 0) }); !errors.Is(err, errBoom) {
		t.Fatalf("Sleep on an ended context throws even for a zero wait, got %v", err)
	}
}
