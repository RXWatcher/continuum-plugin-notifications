BINARY := continuum-plugin-notifications
GO ?= go
PNPM ?= pnpm

.PHONY: build web test clean

build: web
	$(GO) build -o $(BINARY) ./cmd/continuum-plugin-notifications

# Build the SPA so web/embed.go has dist/ to embed. Re-runs idempotently;
# vite skips work if nothing changed.
web:
	cd web && $(PNPM) install --silent && $(PNPM) build

test: web
	$(GO) test ./...

clean:
	rm -f $(BINARY)
	rm -rf web/dist
