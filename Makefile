.PHONY: all build test test-race test-e2e test-integration lint vet fmt tidy clean run release clean-dist

GO ?= go
PKG := ./...
BIN := euphrates
DIST := dist
TARGETS := windows/amd64 darwin/arm64 linux/amd64 linux/arm64

all: lint test build

build:
	$(GO) build -o $(BIN) .

release:
	@mkdir -p $(DIST)
	@set -e; \
	for target in $(TARGETS); do \
		os=$${target%/*}; arch=$${target#*/}; \
		ext=""; if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
		out="$(DIST)/$(BIN)-$$os-$$arch$$ext"; \
		echo "building $$target -> $$out"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch $(GO) build -o "$$out" .; \
	done

run: build
	./$(BIN)

test:
	$(GO) test $(PKG)

test-race:
	$(GO) test -race -count=1 $(PKG)

# End-to-end tests use an in-process IRC server to exercise network paths.
test-e2e:
	$(GO) test -count=1 ./internal/irc -run TestClientE2E_InProcessServer

# Integration tests require a live IRC server (default: devcontainer sidecar).
test-integration:
	IRC_TEST_ADDR=$${IRC_TEST_ADDR:-irc:6667} $(GO) test -tags=integration -count=1 ./internal/irc -run TestClientIntegration_LiveServer

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

clean-dist:
	rm -rf $(DIST)
