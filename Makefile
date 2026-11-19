# Qtestframe build targets. Works on Linux and macOS (GNU make 3.81+).
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
GOFLAGS_BUILD = -trimpath -ldflags "-s -w -X main.version=$(VERSION)"
BIN ?= bin/qtestframe
COVERAGE_FLOOR ?= 85

.PHONY: all build web web-deps go test test-go test-web coverage coverage-go coverage-web \
        lint vet fmt run dev clean release certs-dev

all: build

web-deps:
	cd web && npm ci

web: web-deps
	cd web && npm run build

go:
	CGO_ENABLED=0 go build $(GOFLAGS_BUILD) -o $(BIN) ./cmd/qtestframe

build: web go

# Static binaries (cross compiled; works from macOS).
release: web
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build $(GOFLAGS_BUILD) -o dist/qtestframe-linux-amd64 ./cmd/qtestframe
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build $(GOFLAGS_BUILD) -o dist/qtestframe-linux-arm64 ./cmd/qtestframe
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build $(GOFLAGS_BUILD) -o dist/qtestframe-darwin-arm64 ./cmd/qtestframe

test: test-go test-web

test-go:
	CGO_ENABLED=0 go test -count=1 ./...

test-web:
	cd web && npm run test

# Enforces the 85% floor for Go (all packages combined) and the web client.
coverage: coverage-go coverage-web

coverage-go:
	COVERAGE_FLOOR=$(COVERAGE_FLOOR) scripts/coverage.sh

coverage-web:
	cd web && npm run coverage

vet:
	go vet ./...
	GOOS=darwin go vet ./...

fmt:
	gofmt -l -w cmd internal

lint: vet
	@test -z "$$(gofmt -l cmd internal)" || (echo "gofmt needed:"; gofmt -l cmd internal; exit 1)
	cd web && npm run typecheck

dev:
	scripts/dev.sh

certs-dev:
	scripts/certs/gen-server-cert.sh --cn localhost --san DNS:localhost,IP:127.0.0.1 --self-signed --out certs

clean:
	go clean
	rm -f coverage.out coverage.html
	rm -rf bin dist web/coverage
	find internal/webui/dist -mindepth 1 ! -name .gitkeep -delete
