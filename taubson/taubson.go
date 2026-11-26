// Copyright (c) 2026 Egoisuto Labs. MIT License, see LICENSE.

// Package taubson makes tau.Dig fast on MongoDB documents.
//
// tau.Dig already reads bson.M, bson.A and bson.D without this package, by
// reflection. Importing taubson registers a direct path for the types of
// the official driver (go.mongodb.org/mongo-driver/v2/bson), which makes a
// lookup about three times faster and as cheap as one on decoded JSON:
//
//	import _ "github.com/egoisutolabs/tau/taubson"
//
// The package has no functions to call; the import is the whole of it. It
// is separate from tau so that programs that do not use MongoDB do not
// carry the driver's bson package (about 1.4 MB).
package taubson

import (
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/egoisutolabs/tau/internal/dighook"
)

func init() {
	dighook.Key, dighook.Index = key, index
}

func key(cur any, k string) (next any, found, handled bool) {
	switch doc := cur.(type) {
	case bson.M:
		next, found = doc[k]
		return next, found, true
	case bson.D:
		for _, e := range doc {
			if e.Key == k {
				return e.Value, true, true
			}
		}
		return nil, false, true
	}
	return nil, false, false
}

func index(cur any, i int) (next any, found, handled bool) {
	list, ok := cur.(bson.A)
	if !ok {
		return nil, false, false
	}
	if i < 0 {
		i += len(list)
	}
	if i < 0 || i >= len(list) {
		return nil, false, true
	}
	return list[i], true, true
}
