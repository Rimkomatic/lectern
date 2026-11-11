# Shortcuts for building, running and testing Lectern. Run `make help`.
# Needs Go 1.24+ and Node 22.13+; the db and image targets also need Docker.
# To just run Lectern, you only need Docker: see README.md (Quick start).

BIN     := bin/lectern
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
GOFILES := cmd internal e2e
# MySQL for development: the same compose file, plus MySQL published on 127.0.0.1.
COMPOSE_DEV := docker compose -f docker-compose.yml -f docker-compose.dev.yml

.DEFAULT_GOAL := help
.PHONY: help setup web build run test test-go test-web e2e fmt db db-test db-down image clean

help: ## Show this list
	@grep -hE '^[a-z0-9-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*## "} {printf "  %-9s %s\n", $$1, $$2}'

setup: ## Once: fetch Go and npm dependencies (go mod tidy keeps go.sum in step)
	go mod tidy
	cd web && npm ci

web: ## Build the web app into internal/webui/dist
	@test -d web/node_modules || (cd web && npm ci)
	cd web && npm run build

build: web ## Build the server, with the web app embedded, into bin/lectern
	@go list -deps ./cmd/lectern >/dev/null 2>&1 || { echo "Go dependencies are missing: run 'make setup' first."; exit 1; }
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o $(BIN) ./cmd/lectern

run: build ## Build, then start the server (it reads .env from this folder)
	./$(BIN)

test: test-go test-web ## All unit tests (the Go ones need `make db db-test` first)

test-go: ## Go: gofmt check, vet and tests against the test database
	@test -z "$$(gofmt -l $(GOFILES))" || { echo "gofmt needed on:"; gofmt -l $(GOFILES); exit 1; }
	go vet ./...
	LECTERN_TEST_REQUIRE_DB=1 go test -p 1 -count=1 ./...

test-web: ## Web: type check, unit tests and the colour contrast check
	@test -d web/node_modules || (cd web && npm ci)
	cd web && npm run check && npm test && npm run contrast

e2e: ## Browser smoke test with headless Chrome (empties the test database)
	./e2e/run.sh

fmt: ## Format the Go code
	gofmt -w $(GOFILES)

db: ## Start only MySQL, on 127.0.0.1:3306, and wait until it is ready
	$(COMPOSE_DEV) up -d --wait mysql

db-test: ## Create the throw-away lectern_test database and user for the tests
	$(COMPOSE_DEV) exec -T mysql sh -c 'MYSQL_PWD="$$MYSQL_ROOT_PASSWORD" mysql -uroot' < docker/test-db.sql

db-down: ## Stop the containers (the data is kept in its volume)
	$(COMPOSE_DEV) down

image: ## Build the Docker image of the app from this source, as lectern:local
	docker build --build-arg VERSION=$(VERSION) -t lectern:local .

clean: ## Remove bin/ and the built web app
	rm -rf bin
	find internal/webui/dist -mindepth 1 ! -name .gitkeep -delete
