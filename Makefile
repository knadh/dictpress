# Try to get the semver from 1) git 2) fallback.
VERSION := $(or $(shell git describe --tags --abbrev=0 2> /dev/null),0.0.0)

COMMIT := $(or $(shell git rev-parse --short HEAD 2> /dev/null),"unknown")

BIN := dictpress

.PHONY: build
build:
	VERSION=$(VERSION) cargo build --release

.PHONY: build-debug
build-debug:
	VERSION=$(VERSION) cargo build

.PHONY: run
run:
	cargo run

.PHONY: test
test:
	cargo test

.PHONY: fmt
fmt:
	cargo fmt

.PHONY: lint
lint:
	cargo clippy -- -D warnings

.PHONY: clean
clean:
	cargo clean

.PHONY: dist
dist: build
	@echo "Binary at: target/release/$(BIN)"
