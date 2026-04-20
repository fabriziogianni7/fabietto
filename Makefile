GOFMT_FILES := $(shell gofmt -l .)

.PHONY: ci fmt-check vet test eval benchmark release-gate

ci: fmt-check vet test eval

fmt-check:
	@if [ -n "$(GOFMT_FILES)" ]; then \
		echo "These files are not gofmt-formatted:"; \
		echo "$(GOFMT_FILES)"; \
		exit 1; \
	fi

vet:
	go vet ./...

test:
	go test ./...

eval:
	go run ./cmd/eval

benchmark:
	go run ./cmd/benchmark

release-gate:
	./scripts/release-gate.sh
