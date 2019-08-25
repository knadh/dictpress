.PHONY: build
build:
	go build -o dictmaker

.PHONY: build-tokenizers
build-tokenizers:
	# Compile the Kannada tokenizer.
	go build -ldflags="-s -w" -buildmode=plugin -o kannada.tk tokenizers/kannada/kannada.go

.PHONY: run
run: build
	./dictmaker --site alar
