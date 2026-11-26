package tau

import (
	"context"
	"errors"
	"sync"
)

// Scope owns a group of tasks that may have different result types. Create
// one with WithScope.
type Scope struct {
	ctx    context.Context
	cancel context.CancelCauseFunc
	wg     sync.WaitGroup

	mu    sync.Mutex
	first error // the first failure, from a task or from the body
	ended bool
}

// Task is the eventual result of a function started with Scope.Go.
type Task[T any] struct {
	p *Promise[T]
	s *Scope
}

var errScopeEnded = errors.New("tau: Scope.Go called after the scope ended")

// WithScope runs fn with a Scope for starting tasks, and returns fn's result
// once every task has finished:
//
//	page := tau.WithScope(ctx, func(s *tau.Scope) Page {
//		user := s.Go(func(ctx context.Context) User { return tau.Must(fetchUser(ctx, id)) })
//		posts := s.Go(func(ctx context.Context) []Post { return tau.Must(fetchPosts(ctx, id)) })
//		return Page{User: user.Await(), Posts: posts.Await()}
//	})
//
// It gives two guarantees:
//
//   - A failure cancels everything else. If a task throws, or fn throws, the
//     scope's context is cancelled with that error, so the other tasks stop
//     and an Await blocked in fn throws it at once.
//   - Nothing outlives the scope. WithScope returns only after every task it
//     started has returned, whether fn returned or threw. It is a join.
//
// WithScope then throws the first failure, or returns fn's result. A task that
// is started and never awaited still runs to completion, and still fails the
// scope if it throws. Cancelling ctx cancels every task.
func WithScope[T any](ctx context.Context, fn func(s *Scope) T) T {
	sctx, cancel := context.WithCancelCause(ctx)
	s := &Scope{ctx: sctx, cancel: cancel}
	// Also runs if fn panics with something tau does not catch: the tasks are
	// cancelled and waited for before the panic continues.
	defer func() {
		cancel(errScopeEnded)
		s.wg.Wait()
		s.mu.Lock()
		s.ended = true
		s.mu.Unlock()
	}()

	v, err := TryValue(func() T { return fn(s) })
	if err != nil {
		s.fail(err)
	}
	s.wg.Wait()
	s.mu.Lock()
	first := s.first
	s.mu.Unlock()
	Check(first)
	return v
}

// fail records the first failure and cancels the scope with it.
func (s *Scope) fail(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.first == nil {
		s.first = err
		s.cancel(err)
	}
}

// Context returns the scope's context, which is cancelled when any task or
// the scope's function fails, or when the caller's context ends. Pass it to
// other work that should stop with the scope, such as List.MapAsync.
func (s *Scope) Context() context.Context { return s.ctx }

// Go starts fn as a task of the scope and returns a handle to its result.
// fn's context is the scope's. An ordinary panic in fn is not caught and
// crashes the program, as in any goroutine.
func (s *Scope) Go[T any](fn func(ctx context.Context) T) *Task[T] {
	s.mu.Lock()
	if s.ended {
		s.mu.Unlock()
		Throw(errScopeEnded)
	}
	s.wg.Add(1)
	s.mu.Unlock()

	ctx, cancel := context.WithCancel(s.ctx)
	p := &Promise[T]{done: make(chan struct{}), cancel: cancel}
	go func() {
		defer s.wg.Done()
		defer close(p.done)
		defer cancel()
		func() {
			defer Catch(&p.err)
			p.val = fn(ctx)
		}()
		if p.err != nil {
			s.fail(p.err)
		}
	}()
	return &Task[T]{p, s}
}

// Await waits for the task and returns its value, or throws its error. If
// the scope fails first, Await throws that failure without waiting further.
func (t *Task[T]) Await() T { return t.p.Await(t.s.ctx) }

// Result is Await that returns the error instead of throwing it.
func (t *Task[T]) Result() (T, error) { return TryValue(t.Await) }
