package tau

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

var errBoom = errors.New("boom")

func parseAll(in []string) (out []int, err error) {
	defer Catch(&err)
	return List[string](in).Map(func(s string) int { return Must(strconv.Atoi(s)) }), nil
}

func TestMustAndCatch(t *testing.T) {
	got, err := parseAll([]string{"1", "2", "3"})
	if err != nil || len(got) != 3 || got[2] != 3 {
		t.Fatalf("got %v, %v", got, err)
	}
	if _, err := parseAll([]string{"1", "x"}); err == nil {
		t.Fatal("expected an error for a bad number")
	}
}

func TestTry(t *testing.T) {
	if err := Try(func() { Throw(errBoom) }); !errors.Is(err, errBoom) {
		t.Fatalf("got %v", err)
	}
	if err := Try(func() {}); err != nil {
		t.Fatalf("got %v", err)
	}
	v, err := TryValue(func() int { return 7 })
	if v != 7 || err != nil {
		t.Fatalf("got %v, %v", v, err)
	}
	err = Try(func() { Throwf("wrapped: %w", errBoom) })
	if !errors.Is(err, errBoom) {
		t.Fatalf("got %v", err)
	}
}

func TestOrdinaryPanicsPassThrough(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("a non-thrown panic was swallowed")
		}
	}()
	_ = Try(func() {
		var m map[string]int
		m["x"] = 1
	})
}

type notFound struct{ key string }

func (e *notFound) Error() string { return "not found: " + e.key }

func throwsNotFound() { Throw(&notFound{"k"}) }

func TestStackPointsAtThrowSite(t *testing.T) {
	err := Try(throwsNotFound)
	stack := StackOf(err)
	first, _, _ := strings.Cut(stack, "\n")
	if !strings.HasSuffix(first, "throwsNotFound") {
		t.Fatalf("innermost frame is %q, want throwsNotFound\n%s", first, stack)
	}
	if !strings.Contains(stack, "throw_test.go:") {
		t.Fatalf("stack has no file and line:\n%s", stack)
	}
	if err.Error() != "not found: k" {
		t.Fatalf("message changed: %q", err)
	}
	if StackOf(errors.New("plain")) != "" {
		t.Fatal("an error that was never thrown has no stack")
	}
}

func TestMustStackSkipsTau(t *testing.T) {
	err := Try(func() { Must(0, errBoom) })
	first, _, _ := strings.Cut(StackOf(err), "\n")
	if strings.Contains(first, "tau.Must") || strings.Contains(first, "tau.raise") {
		t.Fatalf("stack starts inside tau: %q", first)
	}
}

func TestRethrowKeepsOriginalStack(t *testing.T) {
	inner := Try(throwsNotFound)
	outer := Try(func() { Throw(inner) })
	if StackOf(inner) != StackOf(outer) {
		t.Fatal("re-throwing replaced the stack")
	}
	var nf *notFound
	if !errors.As(outer, &nf) || nf.key != "k" {
		t.Fatal("errors.As cannot see through a thrown error")
	}
}

func find(fail error) (got string) {
	defer Handle(func(e *notFound) { got = "fallback for " + e.key })
	Check(fail)
	return "found"
}

func TestHandle(t *testing.T) {
	if got := find(nil); got != "found" {
		t.Fatalf("got %q", got)
	}
	if got := find(&notFound{"a"}); got != "fallback for a" {
		t.Fatalf("got %q", got)
	}
	if err := Try(func() { find(errBoom) }); !errors.Is(err, errBoom) {
		t.Fatalf("an unrelated error should keep unwinding, got %v", err)
	}
}

func TestHandleIs(t *testing.T) {
	run := func(fail error) (handled bool) {
		defer HandleIs(errBoom, func(error) { handled = true })
		Check(fail)
		return false
	}
	if !run(errBoom) || run(nil) {
		t.Fatal("HandleIs wrong")
	}
	if err := Try(func() { run(&notFound{"x"}) }); err == nil {
		t.Fatal("an unrelated error should keep unwinding")
	}
}

func TestErrorFormatting(t *testing.T) {
	err := Try(throwsNotFound)
	if got := fmt.Sprintf("%v|%s|%q", err, err, err); got != `not found: k|not found: k|"not found: k"` {
		t.Fatalf("got %s", got)
	}
	verbose := fmt.Sprintf("%+v", err)
	if !strings.HasPrefix(verbose, "not found: k\n") || !strings.Contains(verbose, "throwsNotFound") {
		t.Fatalf("%%+v should print the message then the stack, got:\n%s", verbose)
	}
	wrapped := fmt.Errorf("handling request: %w", err)
	if !strings.Contains(StackOf(wrapped), "throwsNotFound") {
		t.Fatal("the stack should survive wrapping with %w")
	}
}

func loadThing(name string, fail error) int {
	defer Wrapf("load thing %q", name)
	Check(fail)
	return 7
}

func TestWrapf(t *testing.T) {
	if got := loadThing("a", nil); got != 7 {
		t.Fatalf("got %d", got)
	}
	err := Try(func() { loadThing("a", &notFound{"k"}) })
	if err == nil || err.Error() != `load thing "a": not found: k` {
		t.Fatalf("got %v", err)
	}
	if nf, ok := errors.AsType[*notFound](err); !ok || nf.key != "k" {
		t.Fatal("errors.As cannot see through the added context")
	}
	first, _, _ := strings.Cut(StackOf(err), "\n")
	if !strings.HasSuffix(first, "loadThing") {
		t.Fatalf("the stack should still start at the original throw, got %q", first)
	}
	nested := Try(func() {
		defer Wrapf("outer")
		loadThing("b", errBoom)
	})
	if nested.Error() != `outer: load thing "b": boom` || !errors.Is(nested, errBoom) {
		t.Fatalf("got %v", nested)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("Wrapf swallowed an ordinary panic")
		}
	}()
	func() {
		defer Wrapf("ctx")
		var m map[string]int
		m["x"] = 1
	}()
}

func TestHandlersLetOrdinaryPanicsThrough(t *testing.T) {
	nilMapWrite := func() {
		var m map[string]int
		m["x"] = 1
	}
	cases := map[string]func(){
		"Handle":   func() { defer Handle(func(*notFound) {}); nilMapWrite() },
		"HandleIs": func() { defer HandleIs(errBoom, func(error) {}); nilMapWrite() },
		"Catch":    func() { var err error; defer Catch(&err); nilMapWrite() },
		"TryValue": func() { _, _ = TryValue(func() int { nilMapWrite(); return 0 }) },
	}
	for name, run := range cases {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s swallowed an ordinary panic", name)
				}
			}()
			run()
		}()
	}
}

// A thrown error may be wrapped or joined by ordinary Go code before it is
// thrown again. Nothing added along the way may be lost.
func TestRethrowingAWrappedErrorKeepsEverything(t *testing.T) {
	other := errors.New("other")
	inner := Try(throwsNotFound)
	err := Try(func() {
		defer Wrapf("outer")
		Throw(fmt.Errorf("service: %w", errors.Join(inner, other)))
	})
	if err.Error() != "outer: service: not found: k\nother" {
		t.Fatalf("message lost context: %q", err)
	}
	if _, ok := errors.AsType[*notFound](err); !ok || !errors.Is(err, other) {
		t.Fatalf("lost an error from the chain: %v", err)
	}
	first, _, _ := strings.Cut(StackOf(err), "\n")
	if !strings.HasSuffix(first, "throwsNotFound") {
		t.Fatalf("stack should still start at the first throw, got %q", first)
	}
	// Without Wrapf: the caught error must still be printable with its stack.
	plain := Try(func() { Throw(fmt.Errorf("service: %w", inner)) })
	if verbose := fmt.Sprintf("%+v", plain); !strings.HasPrefix(verbose, "service: not found: k\n") || !strings.Contains(verbose, "throwsNotFound") {
		t.Fatalf("%%+v lost the stack of a re-thrown wrapped error:\n%s", verbose)
	}
}
