package tau

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
)

// thrown wraps an error raised by Throw so that Try, Catch and Handle can
// tell it apart from ordinary panics (nil dereferences, index out of range,
// ...), which they never swallow.
type thrown struct{ err error }

// Error is a thrown error together with the stack at the point it was first
// thrown. It reads and unwraps exactly like the error it carries, so
// errors.Is and errors.As see through it.
type Error struct {
	err error
	pcs []uintptr
}

func (e *Error) Error() string { return e.err.Error() }
func (e *Error) Unwrap() error { return e.err }

// Format prints the message for %v and %s, and the message followed by the
// stack for %+v, so one log line shows where an error was thrown:
//
//	log.Printf("request failed: %+v", err)
func (e *Error) Format(s fmt.State, verb rune) {
	switch {
	case verb == 'v' && s.Flag('+'):
		fmt.Fprintf(s, "%s\n%s", e.err.Error(), e.Stack())
	case verb == 'q':
		fmt.Fprintf(s, "%q", e.err.Error())
	default:
		fmt.Fprint(s, e.err.Error())
	}
}

// Stack returns the call stack at the throw, one "function\n\tfile:line" pair
// per frame, innermost first.
func (e *Error) Stack() string {
	var b strings.Builder
	frames := runtime.CallersFrames(e.pcs)
	for {
		f, more := frames.Next()
		if f.Function != "" {
			fmt.Fprintf(&b, "%s\n\t%s:%d\n", f.Function, f.File, f.Line)
		}
		if !more {
			break
		}
	}
	return b.String()
}

// StackOf returns the stack recorded when err was thrown, or "" if err was
// never thrown.
func StackOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Stack()
	}
	return ""
}

// raise panics with err, recording the stack of whoever called the exported
// function that called raise. The thrown value is always an *Error, so %+v
// and StackOf work on whatever is caught. An error that already carries a
// stack, itself or anywhere in its chain, keeps that stack, so re-throwing
// and wrapping preserve the original throw site.
func raise(err error) {
	if _, ok := err.(*Error); ok {
		panic(thrown{err})
	}
	if inner, ok := errors.AsType[*Error](err); ok {
		panic(thrown{&Error{err, inner.pcs}})
	}
	pcs := make([]uintptr, 48)
	panic(thrown{&Error{err, pcs[:runtime.Callers(3, pcs)]}})
}

// Throw aborts the current function and unwinds to the nearest Try, Catch or
// Handle. A nil error does nothing.
func Throw(err error) {
	if err != nil {
		raise(err)
	}
}

// Throwf throws a formatted error. It supports %w like fmt.Errorf.
func Throwf(format string, args ...any) {
	raise(fmt.Errorf(format, args...))
}

// Must returns v, or throws err if it is not nil. It turns
//
//	v, err := f()
//	if err != nil { return err }
//
// into v := tau.Must(f()).
func Must[T any](v T, err error) T {
	if err != nil {
		raise(err)
	}
	return v
}

// Check throws err if it is not nil. It is Must for functions that return
// only an error.
func Check(err error) {
	if err != nil {
		raise(err)
	}
}

// Catch turns a thrown error back into a returned one. Use it with defer
// and a named error result:
//
//	func load(path string) (cfg Config, err error) {
//		defer tau.Catch(&err)
//		data := tau.Must(os.ReadFile(path))
//		...
//	}
func Catch(err *error) {
	if r := recover(); r != nil {
		t, ok := r.(thrown)
		if !ok {
			panic(r)
		}
		*err = t.err
	}
}

// Handle catches thrown errors of one type and lets everything else keep
// unwinding, like `catch (e) { if (e instanceof NotFound) ... else throw e }`.
// Use it with defer; the handler usually sets a named result:
//
//	func find(id string) (u *User) {
//		defer tau.Handle(func(e *NotFoundError) { u = nil })
//		return load(id)
//	}
func Handle[E error](handler func(E)) {
	r := recover()
	if r == nil {
		return
	}
	t, ok := r.(thrown)
	if !ok {
		panic(r)
	}
	var target E
	if !errors.As(t.err, &target) {
		panic(r)
	}
	handler(target)
}

// HandleIs is Handle for sentinel errors: it catches thrown errors for which
// errors.Is(err, target) is true.
func HandleIs(target error, handler func(error)) {
	r := recover()
	if r == nil {
		return
	}
	t, ok := r.(thrown)
	if !ok || !errors.Is(t.err, target) {
		panic(r)
	}
	handler(t.err)
}

// Wrapf adds context to any error thrown through the function it is deferred
// in, like wrapping with fmt.Errorf("context: %w", err) at every exit:
//
//	func loadConfig(path string) Config {
//		defer tau.Wrapf("load config %s", path)
//		data := tau.Must(os.ReadFile(path))
//		return tau.ParseJSON[Config](data)
//	}
//
// The error keeps the stack of its original throw and still matches
// errors.Is and errors.As.
func Wrapf(format string, args ...any) {
	r := recover()
	if r == nil {
		return
	}
	t, ok := r.(thrown)
	if !ok {
		panic(r)
	}
	// Wrap the whole incoming error, not just the part that carries the
	// stack: it may have gained context or been joined with other errors
	// since it was first thrown.
	inner, _ := errors.AsType[*Error](t.err)
	wrapped := fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), t.err)
	panic(thrown{&Error{wrapped, inner.pcs}})
}

// Try runs fn and returns whatever it threw, or nil.
func Try(fn func()) (err error) {
	defer Catch(&err)
	fn()
	return nil
}

// TryValue runs fn and returns its result, or the error it threw.
func TryValue[T any](fn func() T) (v T, err error) {
	defer Catch(&err)
	return fn(), nil
}
