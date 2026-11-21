# Qtestframe build targets. Works on Linux and macOS (GNU make 3.81+).
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
GOFLAGS_BUILD = -trimpath -ldflags "-s -w -X main.version=$(VERSION)"
BIN ?= bin/qtestframe
COVERAGE_FLOOR ?= 85
# user guide export: output folder and languages (empty: all)
GUIDE_OUT ?= dist/user-guide
GUIDE_LANGS ?=
GUIDE_ARGS = --out $(GUIDE_OUT) $(if $(GUIDE_LANGS),--lang $(GUIDE_LANGS))

.PHONY: all build web web-deps go go-pkcs11 test-pkcs11 test-tpm test test-go test-web coverage coverage-go coverage-web \
        lint vet fmt run dev clean release certs-dev guide guide-md guide-pdf guide-docs refs-docs

all: build

web-deps:
	cd web && npm ci

web: web-deps
	cd web && npm run build

go:
	CGO_ENABLED=0 go build $(GOFLAGS_BUILD) -o $(BIN) ./cmd/qtestframe

build: web go

# With HSM support (secrets.provider: pkcs11). PKCS#11 modules are C
# libraries, so this binary needs cgo and is linked dynamically against
# libc (the default build stays static, CGO_ENABLED=0).
go-pkcs11:
	CGO_ENABLED=1 go build -tags pkcs11 $(GOFLAGS_BUILD) -o $(BIN) ./cmd/qtestframe

# The HSM provider against SoftHSM2 (apt-get install softhsm2) and the TPM
# provider against swtpm (apt-get install swtpm); both fail instead of
# skipping when the tool is missing.
test-pkcs11:
	QTF_TEST_SOFTHSM=require CGO_ENABLED=1 go test -count=1 -tags pkcs11 ./internal/secrets

test-tpm:
	QTF_TEST_SWTPM=require CGO_ENABLED=0 go test -count=1 -run TPM ./internal/secrets

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

# The user guide (docs/user-guide) as one document per language, in
# $(GUIDE_OUT): qtestframe-user-guide-<lang>.md with an images/ folder,
# and qtestframe-user-guide-<lang>.pdf (printed with Chromium: Playwright's,
# a system Chrome, or QTF_CHROME). E.g. make guide GUIDE_LANGS=sv
guide: guide-pdf

guide-md: web/node_modules/.package-lock.json
	node web/scripts/guide-export.mjs --no-pdf $(GUIDE_ARGS)

# writes the Markdown too: the PDF is printed from it
guide-pdf: web/node_modules/.package-lock.json
	node web/scripts/guide-export.mjs $(GUIDE_ARGS)

# the single-file Markdown guides kept in the repository
# (docs/user-guide/qtestframe-user-guide-<lang>.md, next to the images they
# use); CI fails when they are out of date with the guide pages
guide-docs: web/node_modules/.package-lock.json
	node web/scripts/guide-export.mjs --no-pdf --out docs/user-guide

# the "Sources cited in the catalogue" section of the guide's references
# page (docs/user-guide/<lang>/references.md), generated from the source
# registry internal/catalog/data/sources.yaml; go test fails when it is out
# of date. Then refresh the single-file guides (guide-docs).
refs-docs:
	go run ./internal/catalog/cmd/refsdoc -dir docs/user-guide

web/node_modules/.package-lock.json: web/package-lock.json
	cd web && npm ci

certs-dev:
	scripts/certs/gen-server-cert.sh --cn localhost --san DNS:localhost,IP:127.0.0.1 --self-signed --out certs

clean:
	go clean
	rm -f coverage.out coverage.html
	rm -rf bin dist web/coverage
	find internal/webui/dist -mindepth 1 ! -name .gitkeep -delete
