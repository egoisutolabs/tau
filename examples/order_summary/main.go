// A whole endpoint: parse and validate a request, fetch from three services
// concurrently with a limit, retries and per-call timeouts, and map failures
// to HTTP statuses. The backend is faked in this file so the example runs
// on its own.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/egoisutolabs/tau"
)

// --- the endpoint -----------------------------------------------------------

type Request struct {
	UserID  string   `json:"userId" validate:"required"`
	ItemIDs []string `json:"itemIds" validate:"required,min=1"`
	Coupon  *string  `json:"coupon"`
}

type Response struct {
	Status   int
	Error    string
	Customer string
	Lines    []string
	Total    int
}

// Summary is the boundary. Nothing thrown inside escapes it: the error comes
// back as a value and is mapped with an ordinary switch.
func Summary(ctx context.Context, b *Backend, body []byte) Response {
	resp, err := tau.TryValue(func() Response { return summary(ctx, b, body) })
	if err == nil {
		return resp
	}
	status := 500
	switch _, missing := errors.AsType[*NotFoundError](err); {
	case missing:
		status = 404
	case isBadInput(err):
		status = 400
	case errors.Is(err, context.DeadlineExceeded):
		status = 504
	}
	return Response{Status: status, Error: err.Error()}
}

// isBadInput reports whether err is the request's fault: malformed JSON, an
// unknown field, a wrong type, or a failed validation rule.
func isBadInput(err error) bool {
	_, bad := errors.AsType[*tau.InputError](err)
	return bad
}

func summary(ctx context.Context, b *Backend, body []byte) Response {
	req := tau.ParseJSON[Request](body)
	user := tau.Must(b.FetchUser(ctx, req.UserID))

	// The discount and the items are fetched at the same time, in one scope:
	// if either fails, the other is cancelled at once, and nothing is still
	// running when the scope returns.
	return tau.WithScope(ctx, func(s *tau.Scope) Response {
		discount := s.Go(func(ctx context.Context) int {
			return tau.Must(b.FetchDiscountPercent(ctx, user.Tier))
		})
		items := tau.List[string](req.ItemIDs).MapAsync(s.Context(), 4, func(ctx context.Context, id string) Item {
			return fetchItem(ctx, b, id)
		})

		lines := items.
			Filter(func(i Item) bool { return i.Cents >= 100 }).
			Map(func(i Item) string { return fmt.Sprintf("%s $%d.%02d", i.ID, i.Cents/100, i.Cents%100) })
		total := items.Reduce(0, func(sum int, i Item) int { return sum + i.Cents })
		total -= total * discount.Await() / 100
		if tau.Deref(req.Coupon, "") == "FIVEOFF" {
			total -= 500
		}
		return Response{Status: 200, Customer: user.Name, Lines: lines, Total: max(total, 0)}
	})
}

// fetchItem tries three times, 30ms each, but gives up at once on a missing item.
func fetchItem(ctx context.Context, b *Backend, id string) Item {
	return tau.Retry(ctx, 3, time.Millisecond, func(ctx context.Context, _ int) Item {
		return tau.Timeout(ctx, 30*time.Millisecond, func(ctx context.Context) Item {
			item, err := b.FetchItem(ctx, id)
			if _, missing := errors.AsType[*NotFoundError](err); missing {
				err = tau.Permanent(err)
			}
			return tau.Must(item, err)
		})
	})
}

// --- a fake backend, written as ordinary Go ---------------------------------

type User struct{ ID, Name, Tier string }

type Item struct {
	ID    string
	Cents int
}

type NotFoundError struct{ Kind, ID string }

func (e *NotFoundError) Error() string { return e.Kind + " not found: " + e.ID }

var errFlaky = errors.New("backend: connection reset")

type Backend struct {
	mu         sync.Mutex
	attempts   map[string]int
	Flaky      map[string]int           // item ID -> failures before it works
	Slow       map[string]time.Duration // item ID -> extra latency
	NoDiscount bool                     // the discount service is down
}

// sleep is the backend's latency. The backend returns errors, so the thrown
// cancellation is turned back into one.
func sleep(ctx context.Context, d time.Duration) error {
	return tau.Try(func() { tau.Sleep(ctx, d) })
}

func (b *Backend) FetchUser(ctx context.Context, id string) (User, error) {
	if err := sleep(ctx, 2*time.Millisecond); err != nil {
		return User{}, err
	}
	users := map[string]User{"u1": {"u1", "Ada", "pro"}, "u2": {"u2", "Linus", "free"}}
	if u, ok := users[id]; ok {
		return u, nil
	}
	return User{}, &NotFoundError{"user", id}
}

func (b *Backend) FetchDiscountPercent(ctx context.Context, tier string) (int, error) {
	if err := sleep(ctx, 2*time.Millisecond); err != nil {
		return 0, err
	}
	if b.NoDiscount {
		return 0, errors.New("discount service unavailable")
	}
	return map[string]int{"free": 0, "pro": 10}[tier], nil
}

func (b *Backend) FetchItem(ctx context.Context, id string) (Item, error) {
	b.mu.Lock()
	if b.attempts == nil {
		b.attempts = map[string]int{}
	}
	b.attempts[id]++
	attempt, failures, slow := b.attempts[id], b.Flaky[id], b.Slow[id]
	b.mu.Unlock()
	if err := sleep(ctx, time.Millisecond+slow); err != nil {
		return Item{}, err
	}
	if attempt <= failures {
		return Item{}, fmt.Errorf("fetch item %s: %w", id, errFlaky)
	}
	prices := map[string]int{"apple": 120, "pear": 250, "fig": 999, "plum": 75}
	if cents, ok := prices[id]; ok {
		return Item{id, cents}, nil
	}
	return Item{}, &NotFoundError{"item", id}
}

// --- run it against every kind of outcome -----------------------------------

func main() {
	slowFig := map[string]time.Duration{"fig": time.Second}
	cases := []struct {
		name    string
		body    string
		backend *Backend
		timeout time.Duration // the caller gives up after this long, if set
		within  time.Duration // the answer must arrive within this long, if set
		status  int
		total   int
	}{
		{"happy path", `{"userId":"u1","itemIds":["apple","pear","fig","plum"],"coupon":"FIVEOFF"}`, &Backend{}, 0, 0, 200, 800},
		{"free tier, no coupon", `{"userId":"u2","itemIds":["apple","plum"]}`, &Backend{}, 0, 0, 200, 195},
		{"malformed JSON", `{"userId":`, &Backend{}, 0, 0, 400, 0},
		{"missing fields", `{"userId":"u1"}`, &Backend{}, 0, 0, 400, 0},
		{"unknown user", `{"userId":"nobody","itemIds":["apple"]}`, &Backend{}, 0, 0, 404, 0},
		{"unknown item", `{"userId":"u1","itemIds":["apple","durian"]}`, &Backend{}, 0, 0, 404, 0},
		{"item fails twice, then works", `{"userId":"u1","itemIds":["apple","pear"]}`, &Backend{Flaky: map[string]int{"pear": 2}}, 0, 0, 200, 333},
		{"item fails every time", `{"userId":"u1","itemIds":["apple","pear"]}`, &Backend{Flaky: map[string]int{"pear": 9}}, 0, 0, 500, 0},
		{"item too slow", `{"userId":"u1","itemIds":["apple","fig"]}`, &Backend{Slow: slowFig}, 0, 0, 504, 0},
		// The discount fails after 2ms while the slow item would take a second:
		// the scope notices at once instead of waiting for the items.
		{"discount fails while items are slow", `{"userId":"u1","itemIds":["apple","fig"]}`, &Backend{Slow: slowFig, NoDiscount: true}, 0, 25 * time.Millisecond, 500, 0},
		{"caller gives up after 20ms", `{"userId":"u1","itemIds":["apple","fig"]}`, &Backend{Slow: slowFig}, 20 * time.Millisecond, 0, 504, 0},
	}
	for _, c := range cases {
		ctx := context.Background()
		if c.timeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, c.timeout)
			defer cancel()
		}
		start := time.Now()
		got := Summary(ctx, c.backend, []byte(c.body))
		if got.Status != c.status || got.Total != c.total {
			fmt.Fprintf(os.Stderr, "FAIL %s: got status %d total %d (%s), want %d and %d\n", c.name, got.Status, got.Total, got.Error, c.status, c.total)
			os.Exit(1)
		}
		if c.timeout > 0 && time.Since(start) > 4*c.timeout {
			fmt.Fprintf(os.Stderr, "FAIL %s: took %v after the caller gave up at %v\n", c.name, time.Since(start), c.timeout)
			os.Exit(1)
		}
		if c.within > 0 && time.Since(start) > c.within {
			fmt.Fprintf(os.Stderr, "FAIL %s: took %v, want an answer within %v\n", c.name, time.Since(start), c.within)
			os.Exit(1)
		}
		fmt.Printf("%-30s %d  %s\n", c.name, got.Status, tau.If(got.Status == 200, fmt.Sprintf("total %d", got.Total), got.Error))
	}
}
