BIN := $(shell basename $$PWD)
HASH := $(shell git rev-parse HEAD | cut -c 1-8)
COMMIT_DATE := $(shell git show -s --format=%ci ${HASH})
BUILD_DATE := $(shell date '+%Y-%m-%d %H:%M:%S')
VERSION := ${HASH} (${COMMIT_DATE})

STATIC := config.toml.sample schema.sql queries.sql

# Install dependencies needed for building
.PHONY: deps
deps:
	go get -u github.com/knadh/stuffbin/...

.PHONY: build
build:
	go build -o ${BIN} -ldflags="-X 'main.buildVersion=${VERSION}' -X 'main.buildDate=${BUILD_DATE}'"
	stuffbin -a stuff -in dictmaker -out dictmaker ${STATIC}

.PHONY: build-tokenizers
build-tokenizers:
	# Compile the Kannada tokenizer.
	go build -ldflags="-s -w" -buildmode=plugin -o kannada.tk tokenizers/kannada/kannada.go

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
