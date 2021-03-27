package main

import (
	"fmt"

	"github.com/knadh/dictmaker/internal/search"
	"github.com/knadh/knphone"
)

// Kannada is the Kannada tokenizer that generates tsvectors for romanized (knphone algorithm)
// Kannada strings
type Kannada struct {
	ph *knphone.KNphone
}

// ID returns the ID of the tokenizer.
func (kn *Kannada) ID() string {
	return "kannada"
}

// Name returns the name of the tokenizer.
func (kn *Kannada) Name() string {
	return "Kannada"
}

// Tokenize tokenizes a Kannada string into Romanized (knphone) Postgres
// tsquery string.
func (kn *Kannada) Tokenize(in string) string {
	key0, key1, key2 := kn.ph.Encode(in)
	if key0 == "" {
		return ""
	}
	return fmt.Sprintf("%s | (%s & %s) ", key2, key1, key0)
}

// New returns a new instance of the Kannada tokenizer.
func New() (search.Tokenizer, error) {
	return &Kannada{
		ph: knphone.New(),
	}, nil
}
