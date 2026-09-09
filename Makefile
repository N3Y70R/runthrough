# SPDX-License-Identifier: GPL-3.0-or-later
BINARY  := runthrough
PKG     := github.com/N3Y70R/runthrough/internal/version
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X $(PKG).Version=$(VERSION) -X $(PKG).Commit=$(COMMIT) -X $(PKG).Date=$(DATE)

.PHONY: build test vet fmt check clean

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/runthrough

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w .

check: fmt vet test

clean:
	rm -rf bin
