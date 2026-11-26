package tau

import "github.com/samber/lo"

// Ptr returns a pointer to v, for optional fields: tau.Ptr("x"), tau.Ptr(3).
func Ptr[T any](v T) *T { return lo.ToPtr(v) }

// Deref returns the value p points to, or fallback if p is nil. It is the
// `p ?? fallback` of optional fields.
func Deref[T any](p *T, fallback T) T { return lo.FromPtrOr(p, fallback) }

// If is the ternary operator: a when cond is true, otherwise b. Both
// arguments are always evaluated; use an if statement when that matters.
func If[T any](cond bool, a, b T) T { return lo.Ternary(cond, a, b) }
