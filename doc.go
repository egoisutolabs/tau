// Copyright (c) 2026 Egoisuto Labs. MIT License, see LICENSE.

// Package tau brings Node.js conveniences to Go server code behind one
// import: thrown errors with stacks, promises and async functions with
// cancellation, chainable lists, and strict JSON parsing with validation.
//
//	func loadUser(ctx context.Context, body []byte) (u User, err error) {
//		defer tau.Catch(&err)                     // thrown errors become the returned error
//		req := tau.ParseJSON[Request](body)       // strict, validated, throws on bad input
//		return tau.Must(db.FindUser(ctx, req.ID)), nil
//	}
//
// # Thrown errors
//
// Throw, Throwf, Must and Check raise an error; Catch, Try, TryValue, Handle
// and HandleIs turn it back into a value. Only errors raised through tau are
// caught: an ordinary panic, such as a nil dereference, keeps propagating.
// A thrown error records the stack of its first throw (StackOf, or the %+v
// verb), and Wrapf adds context to whatever is thrown through a function.
//
// Throw inside a package and return (T, error) from its exported functions,
// so callers need not know tau is used.
//
// # Concurrency
//
// Async starts a function in a goroutine and returns a Promise; an async
// function in the JavaScript sense is an ordinary function that returns
// Async. WithScope runs tasks of different result types together: a failure
// in one cancels the rest, and nothing outlives the scope. All, AllSettled,
// Race, Any, Timeout, Sleep, Retry, RetryWith, and List's MapAsync,
// MapSettled and ForEachAsync cover the other common patterns. Each takes the caller's context first,
// gives the work a derived one, and cancels work it no longer needs.
// Cancellation is cooperative: a function that ignores its context keeps
// running.
//
// # Data
//
// List is a slice with chainable methods (Map, Filter, GroupBy and more),
// most of them thin wrappers over samber/lo. ParseJSON decodes strictly and
// applies `validate` struct tags through go-playground/validator; ReadJSON
// does the same from a reader with a size limit, and both throw an
// *InputError for anything that is the input's fault. Field tells an omitted
// key from an explicit null. Dig reads from untyped JSON. OrderedMap and Set keep insertion order.
//
// # When to use it
//
// Tau suits the edges of a program: handlers, calls to other services, jobs
// and scripts. A throw costs a panic and a stack capture, about one to two
// microseconds, so do not throw for an expected answer on a hot path. The
// guides under docs/ in the repository cover each area in more depth.
package tau
