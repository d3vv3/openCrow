GOLANGCI_LINT ?= $(HOME)/bin/golangci-lint
GO            ?= go

.PHONY: build test lint lint-go lint-web format-web fmt-go

# ── Go server ──────────────────────────────────────────────────────────────────

build:
	cd server && PATH=$(PATH):/usr/local/go/bin $(GO) build ./...

test:
	cd server && PATH=$(PATH):/usr/local/go/bin $(GO) test ./...

lint-go:
	cd server && PATH=$(PATH):/usr/local/go/bin $(GOLANGCI_LINT) run ./...

fmt-go:
	cd server && PATH=$(PATH):/usr/local/go/bin gofmt -w ./

# ── Web UI ─────────────────────────────────────────────────────────────────────

lint-web:
	cd web && npm run lint

format-web:
	cd web && npm run format

# ── All ────────────────────────────────────────────────────────────────────────

lint: lint-go lint-web
