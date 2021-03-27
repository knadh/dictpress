LAST_COMMIT := $(shell git rev-parse --short HEAD)
VERSION := $(shell git describe --tags --abbrev=0)
BUILDSTR := ${VERSION} (\#${LAST_COMMIT} $(shell date -u +"%Y-%m-%dT%H:%M:%S%z"))

STATIC := config.toml.sample schema.sql queries.sql
BIN := dictmaker

# Install dependencies needed for building
.PHONY: deps
deps:
	go get -u github.com/knadh/stuffbin/...

.PHONY: build
build:
	go build -o ${BIN} -ldflags="-s -w -X 'main.buildString=${BUILDSTR}'" cmd/${BIN}/*.go
	stuffbin -a stuff -in ${BIN} -out ${BIN} ${STATIC}

.PHONY: build-tokenizers
build-tokenizers:
	go build -ldflags="-s -w" -buildmode=plugin -o kannada.tk tokenizers/kannada/kannada.go
	go build -ldflags="-s -w" -buildmode=plugin -o malayalam.tk tokenizers/malayalam/malayalam.go

# pack-releases runs stuffbin packing on a given list of
# binaries. This is used with goreleaser for packing
# release builds for cross-build targets.
.PHONY: pack-releases
pack-releases:
	$(foreach var,$(RELEASE_BUILDS),stuffbin -a stuff -in ${var} -out ${var} ${STATIC} $(var);)

# Absolutely awesome: http://marmelab.com/blog/2016/02/29/auto-documented-makefile.html
.PHONY: help
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := build
