// Copyright (c) 2026 Egoisuto Labs. MIT License, see LICENSE.

// Package dighook lets a tau sub-package teach tau.Dig about concrete types
// it would otherwise handle by reflection. The root package cannot name
// those types itself without importing their module into every program.
package dighook

// Key and Index resolve one step of a Dig path. Each reports whether it
// recognised cur's type; if it did not, Dig falls back to reflection. They
// are set from a package's init function and never changed afterwards.
var (
	Key   func(cur any, key string) (next any, found, handled bool)
	Index func(cur any, i int) (next any, found, handled bool)
)
