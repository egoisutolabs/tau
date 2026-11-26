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

func TestScopeMixedTypes(t *testing.T) {
	start := time.Now()
	got := WithScope(bg, func(s *Scope) string {
		n := s.Go(after(30*time.Millisecond, 42, nil))
		name := s.Go(after(30*time.Millisecond, "ada", nil))
		tags := s.Go(after(30*time.Millisecond, []string{"a", "b"}, nil))
		return name.Await() + strconv.Itoa(n.Await()) + strings.Join(tags.Await(), "")
	})
	if got != "ada42ab" {
		t.Fatalf("got %q", got)
	}
	if took := time.Since(start); took > 80*time.Millisecond {
		t.Fatalf("three 30ms tasks took %v; they should overlap", took)
	}
}

func TestScopeFailureCancelsSiblingsAndIsNoticedAtOnce(t *testing.T) {
	var stopped atomic.Int32
	start := time.Now()
	err := Try(func() {
		WithScope(bg, func(s *Scope) int {
			slowA := s.Go(after(time.Hour, 1, &stopped))
			s.Go(after(time.Hour, "never awaited", &stopped))
			s.Go(failing[int](5*time.Millisecond, errBoom))
			return slowA.Await() // blocked here when the failure happens
		})
	})
	took := time.Since(start)
	if !errors.Is(err, errBoom) {
		t.Fatalf("got %v", err)
	}
	if took > time.Second {
		t.Fatalf("the failure at 5ms was noticed after %v", took)
	}
	if stopped.Load() != 2 {
		t.Fatalf("WithScope returned with %d of 2 siblings stopped; it must cancel and wait for them", stopped.Load())
	}
	if !strings.Contains(StackOf(err), "scope_test.go") && !strings.Contains(StackOf(err), "async_test.go") {
		t.Fatalf("the failing task's stack was lost:\n%s", StackOf(err))
	}
}

func TestScopeBodyFailureCancelsTasks(t *testing.T) {
	var stopped atomic.Int32
	err := Try(func() {
		WithScope(bg, func(s *Scope) int {
			s.Go(after(time.Hour, 1, &stopped))
			s.Go(after(time.Hour, 2, &stopped))
			Throw(errBoom)
			return 0
		})
	})
	if !errors.Is(err, errBoom) || stopped.Load() != 2 {
		t.Fatalf("got %v with %d of 2 tasks stopped", err, stopped.Load())
	}
}

func TestScopeWaitsForUnawaitedTasks(t *testing.T) {
	var finished atomic.Int32
	got := WithScope(bg, func(s *Scope) string {
		for range 3 {
			s.Go(func(ctx context.Context) int {
				time.Sleep(20 * time.Millisecond)
				return int(finished.Add(1))
			})
		}
		return "body done first"
	})
	if got != "body done first" || finished.Load() != 3 {
		t.Fatalf("got %q with %d of 3 tasks finished; a normal return must wait, not cancel", got, finished.Load())
	}
	err := Try(func() {
		WithScope(bg, func(s *Scope) int {
			s.Go(failing[int](10*time.Millisecond, errBoom))
			return 1 // returns before the task fails
		})
	})
	if !errors.Is(err, errBoom) {
		t.Fatalf("a task that fails after the body returned must still fail the scope, got %v", err)
	}
}

func TestScopeCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(bg, 10*time.Millisecond)
	defer cancel()
	var stopped atomic.Int32
	err := Try(func() {
		WithScope(ctx, func(s *Scope) int {
			a, b := s.Go(after(time.Hour, 1, &stopped)), s.Go(after(time.Hour, 2, &stopped))
			return a.Await() + b.Await()
		})
	})
	if !errors.Is(err, context.DeadlineExceeded) || stopped.Load() != 2 {
		t.Fatalf("got %v with %d stopped", err, stopped.Load())
	}
}

func TestScopeContextStopsOtherWork(t *testing.T) {
	var stopped atomic.Int32
	start := time.Now()
	err := Try(func() {
		WithScope(bg, func(s *Scope) int {
			discount := s.Go(failing[int](5*time.Millisecond, errBoom))
			items := List[int]{1, 2, 3}.MapAsync(s.Context(), 3, func(ctx context.Context, n int) int {
				return after(time.Hour, n, &stopped)(ctx)
			})
			return len(items) * discount.Await()
		})
	})
	if !errors.Is(err, errBoom) || stopped.Load() != 3 || time.Since(start) > time.Second {
		t.Fatalf("got %v, %d of 3 items stopped, after %v", err, stopped.Load(), time.Since(start))
	}
}

func TestScopeResultNestingAndMisuse(t *testing.T) {
	var leaked *Scope
	total := WithScope(bg, func(s *Scope) int {
		leaked = s
		bad := s.Go(func(ctx context.Context) int {
			return WithScope(ctx, func(inner *Scope) int { // a scope inside a task
				return inner.Go(after(time.Millisecond, 20, nil)).Await() + 1
			})
		})
		v, err := bad.Result()
		if err != nil {
			t.Errorf("got %v", err)
		}
		return v * 2
	})
	if total != 42 {
		t.Fatalf("got %d", total)
	}
	if err := Try(func() { leaked.Go(after(0, 1, nil)) }); err == nil {
		t.Fatal("starting a task on a finished scope should throw")
	}
	if WithScope(bg, func(*Scope) int { return 7 }) != 7 {
		t.Fatal("a scope with no tasks should just return")
	}
}

func TestScopeOrdinaryPanicStillCleansUp(t *testing.T) {
	var stopped atomic.Int32
	defer func() {
		if recover() == nil {
			t.Fatal("WithScope swallowed an ordinary panic")
		}
		if stopped.Load() != 1 {
			t.Fatal("the task was not cancelled and waited for before the panic continued")
		}
	}()
	WithScope(bg, func(s *Scope) int {
		s.Go(after(time.Hour, 1, &stopped))
		var m map[string]int
		m["x"] = 1
		return 0
	})
}
