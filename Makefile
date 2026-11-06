SHELL := /bin/bash
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
PKG     := github.com/thomasbudiarjo/envily/internal/version
LDFLAGS := -s -w -X $(PKG).Version=$(VERSION) -X $(PKG).Commit=$(COMMIT) -X $(PKG).Date=$(DATE)

DEV_DATA     := ./data
DOCKER       ?= docker
DEV_WEB_PORT ?= 5173
DEV_ENV      := ENVILY_PUBLIC_URL=http://localhost:$(DEV_WEB_PORT) \
            ENVILY_DB_DSN=$(DEV_DATA)/envily.db \
            ENVILY_MASTER_KEY_FILE=$(DEV_DATA)/dev-master.key \
            ENVILY_LOG_FORMAT=text ENVILY_LOG_LEVEL=debug

.PHONY: dev web build generate test test-go test-web test-pg e2e lint docker clean

$(DEV_DATA)/dev-master.key:
	@mkdir -p $(DEV_DATA) && chmod 700 $(DEV_DATA)
	@head -c 32 /dev/urandom | base64 > $@ && chmod 600 $@
	@echo "generated development master key in $@"

## dev: run the Go server (:8080) and the Vite dev server (:5173, override with DEV_WEB_PORT) together
dev: $(DEV_DATA)/dev-master.key
	@test -d web/node_modules || (cd web && pnpm install)
	@trap 'kill 0' EXIT; \
	$(DEV_ENV) go run ./cmd/envily server & \
	(cd web && pnpm dev --port $(DEV_WEB_PORT)) & \
	wait

## web: build the SvelteKit app into web/build
web:
	cd web && pnpm install --frozen-lockfile && pnpm build

## build: build the UI and a static binary at bin/envily
build: web
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/envily ./cmd/envily

## generate: regenerate sqlc code
generate:
	sqlc generate
	go generate ./internal/store/...

## test: Go tests (SQLite) and frontend unit tests
test: test-go test-web

test-go:
	go test ./...

test-web:
	cd web && pnpm test

## test-pg: store and API tests against PostgreSQL (testcontainers or ENVILY_TEST_POSTGRES_DSN)
test-pg:
	ENVILY_TEST_POSTGRES=1 go test -count=1 ./internal/store/... ./internal/httpapi/...

## e2e: build, start a throwaway server, and run Playwright
e2e: build
	cd web && pnpm exec playwright test

## lint: Go and frontend linters
lint:
	golangci-lint run ./...
	cd web && pnpm check && pnpm lint

## docker: build the container image
docker:
	$(DOCKER) build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) --build-arg DATE=$(DATE) -t envily:$(VERSION) -t envily:latest .

clean:
	rm -rf bin web/.svelte-kit
	find web/build -mindepth 1 ! -name .gitkeep -delete
