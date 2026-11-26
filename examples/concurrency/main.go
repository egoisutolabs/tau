// Promises, async functions and the helpers built on them.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/egoisutolabs/tau"
)

var cancelled atomic.Int32

// wait sleeps for d, or throws as soon as ctx is cancelled, counting the
// cancellations so the checks below can see that work was stopped. What is
// thrown is the reason for the cancellation: a sibling's error if that is
// what stopped it.
func wait(ctx context.Context, d time.Duration) {
	if err := tau.Try(func() { tau.Sleep(ctx, d) }); err != nil {
		cancelled.Add(1)
		tau.Throw(err)
	}
}

// fetchUser and fetchPosts are async functions: ordinary functions that
// return tau.Async. Calling one starts it and returns a promise.
func fetchUser(ctx context.Context, id string) *tau.Promise[string] {
	return tau.Async(ctx, func(ctx context.Context) string {
		wait(ctx, 30*time.Millisecond)
		return "user:" + id
	})
}

func fetchPosts(ctx context.Context, id string) *tau.Promise[int] {
	return tau.Async(ctx, func(ctx context.Context) int {
		wait(ctx, 30*time.Millisecond)
		return 3
	})
}

func main() {
	ctx := context.Background()

	// Two calls started before either is awaited run at the same time.
	start := time.Now()
	user, posts := fetchUser(ctx, "u1"), fetchPosts(ctx, "u1")
	summary := fmt.Sprintf("%s has %d posts", user.Await(ctx), posts.Await(ctx))
	check(summary == "user:u1 has 3 posts", "got %q", summary)
	check(time.Since(start) < 55*time.Millisecond, "the two 30ms calls should overlap, took %v", time.Since(start))

	// MapAsync: at most 2 at a time, results in order.
	var running, peak atomic.Int32
	squares := tau.List[int]{1, 2, 3, 4, 5}.MapAsync(ctx, 2, func(ctx context.Context, n int) int {
		peak.Store(max(peak.Load(), running.Add(1)))
		defer running.Add(-1)
		wait(ctx, 10*time.Millisecond)
		return n * n
	})
	check(fmt.Sprint(squares) == "[1 4 9 16 25]" && peak.Load() == 2, "squares %v, peak %d", squares, peak.Load())

	// ForEachAsync: the same, for work done for its effect.
	var sent atomic.Int32
	tau.List[string]{"a@x.co", "b@x.co", "c@x.co"}.ForEachAsync(ctx, 2, func(ctx context.Context, to string) {
		wait(ctx, 5*time.Millisecond)
		sent.Add(1)
	})
	check(sent.Load() == 3, "ForEachAsync sent %d of 3", sent.Load())

	// MapSettled: every element gets its own outcome; one failure stops nothing.
	outcomes := tau.List[int]{1, 2, 3, 4}.MapSettled(ctx, 2, func(_ context.Context, n int) int {
		if n == 2 {
			tau.Throwf("job %d failed", n)
		}
		return n * 10
	})
	failed := outcomes.Count(func(o tau.Settled[int]) bool { return !o.OK() })
	check(failed == 1 && outcomes[3].Value == 40, "MapSettled: %d failed, %+v", failed, outcomes)

	// WithScope: results of different types; a failure cancels the rest at once.
	type page struct {
		user  string
		posts int
	}
	p := tau.WithScope(ctx, func(s *tau.Scope) page {
		u := s.Go(func(ctx context.Context) string { wait(ctx, 10*time.Millisecond); return "ada" })
		n := s.Go(func(ctx context.Context) int { wait(ctx, 10*time.Millisecond); return 3 })
		return page{u.Await(), n.Await()}
	})
	check(p == page{"ada", 3}, "WithScope: %+v", p)

	// All: one failure cancels the others, and All waits for them to stop.
	boom := errors.New("boom")
	cancelled.Store(0)
	err := tau.Try(func() {
		tau.All(ctx,
			func(ctx context.Context) int { wait(ctx, time.Hour); return 1 },
			func(ctx context.Context) int { wait(ctx, time.Hour); return 2 },
			func(ctx context.Context) int { tau.Throw(boom); return 3 },
		)
	})
	check(errors.Is(err, boom) && cancelled.Load() == 2, "All: err %v, %d siblings cancelled", err, cancelled.Load())

	// Retry: fails twice, then succeeds. A Permanent error is not retried.
	attempts := 0
	value := tau.Retry(ctx, 5, time.Millisecond, func(ctx context.Context, attempt int) string {
		attempts = attempt
		if attempt < 3 {
			tau.Throw(boom)
		}
		return "ok"
	})
	check(value == "ok" && attempts == 3, "Retry: %q after %d attempts", value, attempts)
	tries := 0
	err = tau.Try(func() {
		tau.Retry(ctx, 5, time.Millisecond, func(context.Context, int) int {
			tries++
			tau.Throw(tau.Permanent(boom))
			return 0
		})
	})
	check(errors.Is(err, boom) && tries == 1, "Permanent: err %v after %d tries", err, tries)

	// Timeout: the function's context is cancelled at the deadline.
	err = tau.Try(func() {
		tau.Timeout(ctx, 20*time.Millisecond, func(ctx context.Context) int { wait(ctx, time.Hour); return 0 })
	})
	check(errors.Is(err, tau.ErrTimeout), "Timeout: err %v", err)

	// Race: first to finish wins, the loser is cancelled.
	winner := tau.Race(ctx,
		func(ctx context.Context) string { wait(ctx, time.Hour); return "slow" },
		func(ctx context.Context) string { wait(ctx, 5*time.Millisecond); return "fast" },
	)
	check(winner == "fast", "Race: %q", winner)

	fmt.Println(summary)
	fmt.Println("squares:", squares)
	fmt.Println("all checks passed")
}

func check(ok bool, format string, args ...any) {
	if !ok {
		fmt.Fprintf(os.Stderr, "FAIL: "+format+"\n", args...)
		os.Exit(1)
	}
}
