BINARY := silo-plugin-notifications
GO ?= go
PNPM ?= pnpm

.PHONY: build web test clean

build: web
	$(GO) build -o $(BINARY) ./cmd/silo-plugin-notifications

# Build the SPA so web/embed.go has dist/ to embed. Re-runs idempotently;
# vite skips work if nothing changed.
web:
	cd web && $(PNPM) install --frozen-lockfile && $(PNPM) build

test: test-go test-web

test-go:
	$(GO) test ./...

test-web:
	cd web && $(PNPM) run test --run

clean:
	rm -f $(BINARY)
	rm -rf web/dist
