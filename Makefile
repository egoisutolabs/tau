# `make verify` is the full verification gate: the same checks CI runs.
.PHONY: verify fmt-check vet test examples

verify: fmt-check vet test examples

fmt-check:
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then echo "These files need gofmt:"; echo "$$unformatted"; exit 1; fi

vet:
	go vet ./...

test:
	go test -race ./...

# Every example checks its own result and exits nonzero on a mismatch.
examples:
	@for d in examples/*/; do (cd "$$d" && go run .) > /dev/null || { echo "example failed: $$d"; exit 1; }; done
