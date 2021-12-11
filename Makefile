LAST_COMMIT := $(shell git rev-parse --short HEAD)
VERSION := $(shell git describe --tags --abbrev=0)
BUILDSTR := ${VERSION} (\#${LAST_COMMIT} $(shell date -u +"%Y-%m-%dT%H:%M:%S%z"))

STATIC := config.toml.sample schema.sql queries.sql admin
BIN := dictmaker

# Install dependencies needed for building
.PHONY: deps
deps:
	go get -u github.com/knadh/stuffbin/...

.PHONY: build
build:
	go build -o ${BIN} -ldflags="-s -w -X 'main.buildString=${BUILDSTR}'" cmd/${BIN}/*.go

.PHONY: run
run:
	go run -ldflags="-s -w -X 'main.buildString=${BUILDSTR}'" cmd/${BIN}/*.go

# Compile bin and bundle static assets.
.PHONY: dist
dist: build
	stuffbin -a stuff -in ${BIN} -out ${BIN} ${STATIC}

# pack-releases runn stuffbin packing on the given binary. This is used
# in the .goreleaser post-build hook.
.PHONY: pack-bin
pack-bin:
	stuffbin -a stuff -in ${BIN} -out ${BIN} ${STATIC}

# Use goreleaser to do a dry run producing local builds.
.PHONY: release-dry
release-dry:
	goreleaser --parallelism 1 --rm-dist --snapshot --skip-validate --skip-publish

# Use goreleaser to build production releases and publish them.
.PHONY: release
release:
	goreleaser --parallelism 1 --rm-dist --skip-validate

.DEFAULT_GOAL := dist
