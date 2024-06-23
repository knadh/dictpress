# Try to get the commit hash from 1) git 2) the VERSION file 3) fallback.
LAST_COMMIT := $(or $(shell git rev-parse --short HEAD 2> /dev/null),$(shell head -n 1 VERSION | grep -oP -m 1 "^[a-z0-9]+$$"),"UNKNOWN")

# Try to get the semver from 1) git 2) the VERSION file 3) fallback.
VERSION := $(or $(shell git describe --tags --abbrev=0 2> /dev/null),$(shell grep -oP "tag: \K(.*)(?=,)" VERSION),"v0.0.0")

BUILDSTR := ${VERSION} (\#${LAST_COMMIT} $(shell date -u +"%Y-%m-%dT%H:%M:%S%z"))

YARN ?= yarn
GOPATH ?= $(HOME)/go
STUFFBIN ?= $(GOPATH)/bin/stuffbin

BIN := dictpress
STATIC := config.sample.toml schema.sql queries.sql admin

.PHONY: build
build: $(BIN)

$(STUFFBIN):
	go install github.com/knadh/stuffbin/...

$(BIN): $(shell find . -type f -name "*.go")
	CGO_ENABLED=0 go build -o ${BIN} -ldflags="-s -w -X 'main.buildString=${BUILDSTR}' -X 'main.versionString=${VERSION}'" cmd/${BIN}/*.go

.PHONY: run
run:
	CGO_ENABLED=0 go run -ldflags="-s -w -X 'main.buildString=${BUILDSTR}' -X 'main.versionString=${VERSION}'" cmd/${BIN}/*.go


run-mock-db:
	docker run --rm -d -p 5432:5432 --name dictpress-db -e POSTGRES_PASSWORD=dictpress -e POSTGRES_USER=dictpress -e POSTGRES_DB=dictpress postgres:13 -c log_statement=all
	sleep 1
	docker exec -i dictpress-db psql -U dictpress -d dictpress < schema.sql
	touch run-mock-db

clean-mock-db:
	docker rm -f dictpress-db
	rm run-mock-db

# Run Go tests.
.PHONY: test
test: run-mock-db unit-test clean-mock-db

unit-test:
	go test ./...

.PHONY: dist
dist: $(STUFFBIN) build pack-bin

# pack-releases runns stuffbin packing on the given binary. This is used
# in the .goreleaser post-build hook.
.PHONY: pack-bin
pack-bin: $(BIN) $(STUFFBIN)
	$(STUFFBIN) -a stuff -in ${BIN} -out ${BIN} ${STATIC}

# Use goreleaser to do a dry run producing local builds.
.PHONY: release-dry
release-dry:
	goreleaser --parallelism 1 --rm-dist --snapshot --skip-validate --skip-publish

# Use goreleaser to build production releases and publish them.
.PHONY: release
release:
	goreleaser --parallelism 1 --rm-dist --skip-validate
