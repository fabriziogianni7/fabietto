GOFMT_FILES := $(shell gofmt -l .)

.PHONY: ci fmt-check vet test

ci: fmt-check vet test

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
