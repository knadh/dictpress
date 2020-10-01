STATIC := config.toml.sample schema.sql queries.sql

# Install dependencies needed for building
.PHONY: deps
deps:
	go get -u github.com/knadh/stuffbin/...

.PHONY: build
build:
	go build -o dictmaker
	stuffbin -a stuff -in dictmaker -out dictmaker ${STATIC}

.PHONY: build-tokenizers
build-tokenizers:
	# Compile the Kannada tokenizer.
	go build -ldflags="-s -w" -buildmode=plugin -o kannada.tk tokenizers/kannada/kannada.go
	go build -ldflags="-s -w" -buildmode=plugin -o malayalam.tk tokenizers/malayalam/malayalam.go

# pack-releases runs stuffbin packing on a given list of
# binaries. This is used with goreleaser for packing
# release builds for cross-build targets.
.PHONY: pack-releases
pack-releases:
	$(foreach var,$(RELEASE_BUILDS),stuffbin -a stuff -in ${var} -out ${var} ${STATIC} $(var);)
