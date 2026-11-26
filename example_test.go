package tau_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/egoisutolabs/tau"
)

// A boundary function: errors thrown inside come back as its returned error.
func ExampleCatch() {
	parseAll := func(in []string) (out []int, err error) {
		defer tau.Catch(&err)
		return tau.List[string](in).Map(func(s string) int { return tau.Must(strconv.Atoi(s)) }), nil
	}
	fmt.Println(parseAll([]string{"1", "2", "3"}))
	_, err := parseAll([]string{"1", "x"})
	fmt.Println(err)
	// Output:
	// [1 2 3] <nil>
	// strconv.Atoi: parsing "x": invalid syntax
}

func ExampleWrapf() {
	readConfig := func(path string) []byte {
		defer tau.Wrapf("read config %s", path)
		return tau.Must(os.ReadFile(path))
	}
	err := tau.Try(func() { readConfig("/does/not/exist.json") })
	fmt.Println(err)
	fmt.Println(errors.Is(err, os.ErrNotExist))
	// Output:
	// read config /does/not/exist.json: open /does/not/exist.json: no such file or directory
	// true
}

type missingError struct{ key string }

func (e *missingError) Error() string { return "missing " + e.key }

// Handle catches one error type and lets every other error keep unwinding.
func ExampleHandle() {
	get := func(key string) string {
		tau.Throw(&missingError{key})
		return ""
	}
	getOr := func(key, fallback string) (value string) {
		defer tau.Handle(func(*missingError) { value = fallback })
		return get(key)
	}
	fmt.Println(getOr("colour", "blue"))
	// Output: blue
}

// An async function is an ordinary function that returns tau.Async.
func ExampleAsync() {
	double := func(ctx context.Context, n int) *tau.Promise[int] {
		return tau.Async(ctx, func(ctx context.Context) int {
			time.Sleep(10 * time.Millisecond)
			return n * 2
		})
	}
	ctx := context.Background()
	a, b := double(ctx, 1), double(ctx, 20) // both are running now
	fmt.Println(a.Await(ctx) + b.Await(ctx))
	// Output: 42
}

func ExampleAll() {
	results := tau.All(context.Background(),
		func(context.Context) string { return "a" },
		func(context.Context) string { return "b" },
	)
	fmt.Println(results)
	// Output: [a b]
}

func ExampleRetry() {
	result := tau.Retry(context.Background(), 5, time.Millisecond, func(_ context.Context, attempt int) string {
		if attempt < 3 {
			tau.Throwf("attempt %d failed", attempt)
		}
		return fmt.Sprintf("succeeded on attempt %d", attempt)
	})
	fmt.Println(result)
	// Output: succeeded on attempt 3
}

func ExampleTimeout() {
	err := tau.Try(func() {
		tau.Timeout(context.Background(), 10*time.Millisecond, func(ctx context.Context) int {
			<-ctx.Done() // cancelled at the deadline
			return 0
		})
	})
	fmt.Println(errors.Is(err, tau.ErrTimeout))
	// Output: true
}

func ExampleList_MapAsync() {
	lengths := tau.List[string]{"a", "bb", "ccc"}.MapAsync(context.Background(), 2,
		func(_ context.Context, s string) int { return len(s) })
	fmt.Println(lengths)
	// Output: [1 2 3]
}

func ExampleList_Map() {
	labels := tau.List[int]{1, 2, 3, 4}.
		Filter(func(n int) bool { return n%2 == 0 }).
		Map(strconv.Itoa) // int -> string
	fmt.Printf("%q\n", labels)
	// Output: ["2" "4"]
}

func ExampleList_GroupBy() {
	words := tau.List[string]{"apple", "avocado", "banana", "blueberry", "cherry"}
	groups := words.GroupBy(func(w string) string { return w[:1] })
	fmt.Println(groups["a"], groups["b"], len(groups))
	// Output: [apple avocado] [banana blueberry] 3
}

func ExampleParseJSON() {
	type Signup struct {
		Email string `json:"email" validate:"required,email"`
		Age   int    `json:"age" validate:"gte=18"`
	}
	s := tau.ParseJSON[Signup]([]byte(`{"email":"ada@example.com","age":36}`))
	fmt.Println(s.Email)

	err := tau.Try(func() { tau.ParseJSON[Signup]([]byte(`{"email":"ada@example.com","age":12}`)) })
	fmt.Println(err)
	// Output:
	// ada@example.com
	// tau: invalid tau_test.Signup: age must satisfy gte=18 (got 12)
}

func ExampleDig() {
	data := map[string]any{
		"user": map[string]any{"addresses": []any{map[string]any{"city": "London"}}},
	}
	city, ok := tau.Dig[string](data, "user", "addresses", 0, "city")
	fmt.Println(city, ok)
	fmt.Println(tau.DigOr(data, "unknown", "user", "phone"))
	// Output:
	// London true
	// unknown
}

func ExampleDig_struct() {
	type Address struct {
		City string `json:"city"`
	}
	type User struct {
		Name      string    `json:"name"`
		Addresses []Address `json:"addresses"`
		Manager   *User     `json:"manager"`
	}
	user := User{Name: "Ada", Addresses: []Address{{City: "London"}, {City: "Turin"}}}

	// Fields are named by their json tag; a nil pointer on the way is a miss.
	city, _ := tau.Dig[string](user, "addresses", -1, "city")
	fmt.Println(city)
	fmt.Println(tau.DigOr(user, "nobody", "manager", "name"))
	// Output:
	// Turin
	// nobody
}

func ExampleConvert() {
	type CustomerRow struct {
		Email        string
		PasswordHash string
	}
	type OrderRow struct {
		ID       int32
		Customer CustomerRow
		PlacedAt time.Time
	}
	type OrderResponse struct {
		ID       int    `json:"id"`
		Email    string `json:"email" tau:"Customer.Email"`
		PlacedAt string `json:"placed_at"`
		Summary  string `json:"summary" tau:"-"`
	}
	row := OrderRow{ID: 7, Customer: CustomerRow{"ada@example.com", "secret"}, PlacedAt: time.Date(2026, 11, 18, 14, 0, 0, 0, time.UTC)}

	resp := tau.Convert[OrderResponse](row)
	resp.Summary = fmt.Sprintf("order %d", resp.ID)
	fmt.Printf("%+v\n", resp)

	// A field with nothing to match is refused, not left empty.
	type Renamed struct{ Mail string }
	err := tau.Try(func() { tau.Convert[Renamed](row.Customer) })
	if ce, ok := errors.AsType[*tau.ConvertError](err); ok {
		fmt.Println("refused:", ce.Field)
	}
	// Output:
	// {ID:7 Email:ada@example.com PlacedAt:2026-11-18T14:00:00Z Summary:order 7}
	// refused: Mail
}

func ExampleOrderedMap() {
	m := tau.NewMap[string, int]().Set("zebra", 1).Set("apple", 2).Set("mango", 3)
	for key, value := range m.All() {
		fmt.Println(key, value)
	}
	// Output:
	// zebra 1
	// apple 2
	// mango 3
}

func ExampleSet() {
	a, b := tau.NewSet(1, 2, 3), tau.NewSet(3, 4)
	fmt.Println(a.Union(b).Values(), a.Intersection(b).Values(), a.Difference(b).Values())
	// Output: [1 2 3 4] [3] [1 2]
}

func ExampleStackOf() {
	fail := func() { tau.Throwf("something broke") }
	err := tau.Try(fail)
	fmt.Println(err)
	fmt.Println(tau.StackOf(err) != "")
	// Output:
	// something broke
	// true
}

// Tasks in a scope may have different result types. A failure in one cancels
// the others, and WithScope returns only after all of them have finished.
func ExampleWithScope() {
	type Page struct {
		User  string
		Posts []string
	}
	page := tau.WithScope(context.Background(), func(s *tau.Scope) Page {
		user := s.Go(func(context.Context) string { return "ada" })
		posts := s.Go(func(context.Context) []string { return []string{"hello", "world"} })
		return Page{User: user.Await(), Posts: posts.Await()}
	})
	fmt.Println(page.User, len(page.Posts))
	// Output: ada 2
}

func ExampleRetryWith() {
	temporary := errors.New("temporary")
	policy := tau.RetryPolicy{
		Attempts: 5,
		Delay:    time.Millisecond,
		MaxDelay: 10 * time.Millisecond,
		Jitter:   0.5,
		RetryIf:  func(err error) bool { return errors.Is(err, temporary) },
	}
	result := tau.RetryWith(context.Background(), policy, func(_ context.Context, attempt int) string {
		if attempt < 3 {
			tau.Throw(temporary)
		}
		return fmt.Sprintf("ok on attempt %d", attempt)
	})
	fmt.Println(result)
	// Output: ok on attempt 3
}

// Every element gets its own outcome; one failure does not stop the rest.
func ExampleList_MapSettled() {
	results := tau.List[string]{"1", "x", "3"}.MapSettled(context.Background(), 2,
		func(_ context.Context, s string) int { return tau.Must(strconv.Atoi(s)) })
	for _, r := range results {
		fmt.Println(r.Value, r.OK())
	}
	// Output:
	// 1 true
	// 0 false
	// 3 true
}

func ExampleReadJSON() {
	type Request struct {
		Name string `json:"name" validate:"required"`
	}
	req := tau.ReadJSON[Request](strings.NewReader(`{"name":"ada"}`), 1<<20)
	fmt.Println(req.Name)

	err := tau.Try(func() { tau.ReadJSON[Request](strings.NewReader(`{"name":"ada"}`), 5) })
	fmt.Println(errors.Is(err, tau.ErrTooLarge))
	// Output:
	// ada
	// true
}

// A Field tells apart a key that was left out, one sent as null, and a value.
func ExampleField() {
	type UpdateProfile struct {
		Bio tau.Field[string] `json:"bio,omitzero" validate:"omitnil,max=200"`
	}
	current := "old bio"
	for _, body := range []string{`{}`, `{"bio":null}`, `{"bio":"new bio"}`} {
		req := tau.ParseJSON[UpdateProfile]([]byte(body))
		fmt.Printf("%-18s -> %q\n", body, req.Bio.OrElse(current))
	}
	// Output:
	// {}                 -> "old bio"
	// {"bio":null}       -> ""
	// {"bio":"new bio"}  -> "new bio"
}

// One check recognises every failure that is the input's fault.
func ExampleInputError() {
	type Request struct {
		Name string `json:"name" validate:"required"`
		Age  int    `json:"age"`
	}
	for _, body := range []string{`{"name":"ada","age":"old"}`, `{"name":"ada","colour":"blue"}`, `{"age":3}`, `{`} {
		err := tau.Try(func() { tau.ParseJSON[Request]([]byte(body)) })
		if bad, ok := errors.AsType[*tau.InputError](err); ok {
			fmt.Printf("%s at %q\n", bad.Kind, bad.Path)
		}
	}
	// Output:
	// wrong_type at "/age"
	// unknown_field at "/colour"
	// invalid at ""
	// malformed at ""
}

func ExampleList_ForEachAsync() {
	var sent atomic.Int32
	tau.List[string]{"a@example.com", "b@example.com", "c@example.com"}.
		ForEachAsync(context.Background(), 2, func(ctx context.Context, to string) {
			sent.Add(1) // send the notification
		})
	fmt.Println(sent.Load())
	// Output: 3
}

func ExampleSleep() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	err := tau.Try(func() { tau.Sleep(ctx, time.Hour) })
	fmt.Println(errors.Is(err, context.DeadlineExceeded))
	// Output: true
}
