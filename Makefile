.PHONY: all build test test-race lint vet fmt tidy clean run

GO ?= go
PKG := ./...
BIN := euphrates

all: lint test build

build:
	$(GO) build -o $(BIN) .

run: build
	./$(BIN)

test:
	$(GO) test $(PKG)

test-race:
	$(GO) test -race -count=1 $(PKG)

vet:
	$(GO) vet $(PKG)

fmt:
	$(GO) fmt $(PKG)

# `lint` runs gofmt-check + vet. golangci-lint is optional and used if present.
lint: vet
	@out=$$(gofmt -l . | grep -v '^vendor/' || true); \
	if [ -n "$$out" ]; then \
		echo "gofmt needs to run on:"; echo "$$out"; exit 1; \
	fi
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run $(PKG); \
	else \
		echo "(golangci-lint not installed; skipping)"; \
	fi

tidy:
	$(GO) mod tidy

clean:
	rm -f $(BIN)
	$(GO) clean -testcache
