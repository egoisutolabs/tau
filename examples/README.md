# Examples

Each directory is a standalone program. Run one from inside its directory:

```bash
cd examples/order_summary && go run .
```

Every example checks its own result and exits nonzero on a mismatch; CI
runs all of them.

| Example | Shows |
| --- | --- |
| [`errors`](errors) | Throwing inside a package and returning a plain error at its boundary; `Wrapf` for context; `Handle` to catch one error type; reading the stack of a thrown error |
| [`concurrency`](concurrency) | Async functions that overlap, `MapAsync` with a limit, `ForEachAsync`, `MapSettled`, a `WithScope` task group, `All` cancelling siblings on failure, `Retry` with `Permanent`, `Timeout`, `Race`, `Sleep` |
| [`lists`](lists) | A chain that filters, sorts and maps; `GroupBy`, `Partition`, `Chunk`; `OrderedMap` and `Set` |
| [`validation`](validation) | `ParseJSON` with `validate` tags and a `Validate` method; every failed field by JSON path; strict decoding; `Dig` on untyped JSON |
| [`order_summary`](order_summary) | A whole endpoint: validated input recognised as a bad request through `InputError`, a task group that fetches a discount and a list of items together, a concurrency limit, retries and per-call timeouts, cancellation when the caller gives up or one fetch fails, and errors mapped to statuses. Eleven scenarios, each checked |
