# tau documentation

Guides for using tau. The
[API reference](https://pkg.go.dev/github.com/egoisutolabs/tau) on
pkg.go.dev documents every type and function; these pages explain how the
pieces fit together.

## Start here

1. [Getting started](getting-started.md): install, a first function, the
   boundary rule.
2. [When to use tau](when-to-use.md): where it pays off, where it does
   not, and what it costs.

## Guides

- [Errors](errors.md): throwing, catching, context, stacks, and how thrown
  errors meet ordinary Go errors.
- [Concurrency](concurrency.md): promises, async functions, task groups,
  retry policies, batches, and the cancellation model.
- [Lists](lists.md): chaining, grouping, and what stays a function.
- [JSON and validation](json-and-validation.md): strict parsing,
  validation tags, error reporting, reading untyped JSON.
- [Ordered map and set](collections.md)
- [Coming from Node.js](coming-from-node.md): the mapping from JavaScript
  idioms to tau, and the places where Go differs.

## Reference

- [Examples](../examples/README.md): five complete programs, each
  checking its own result.
- [Changelog](../CHANGELOG.md)
- [Contributing](../CONTRIBUTING.md)
