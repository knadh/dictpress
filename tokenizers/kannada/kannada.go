package main

import (
	"fmt"
	"strings"

	"github.com/knadh/dictmaker/internal/data"
	"github.com/knadh/knphone"
)

// Kannada is the Kannada tokenizer that generates tsvectors for romanized (knphone algorithm)
// Kannada strings
type Kannada struct {
	ph *knphone.KNphone
}

// New returns a new instance of the Kannada tokenizer.
func New() (data.Tokenizer, error) {
	return &Kannada{
		ph: knphone.New(),
	}, nil
}

// ID returns the ID of the tokenizer.
func (*Kannada) ID() string {
	return "kannada"
}

// Name returns the name of the tokenizer.
func (*Kannada) Name() string {
	return "Kannada"
}

// ToTokens tokenizes a Kannada string and returns an array of tsvector tokens.
// eg: [KRM0 KRM] or [KRM:2 KRM:1] with weights.
func (kn *Kannada) ToTokens(s string) []string {
	var (
		chunks = strings.Split(s, " ")
		tokens = make([]data.Token, 0, len(chunks)*3)
	)
	for _, c := range chunks {
		key0, key1, key2 := kn.ph.Encode(c)
		tokens = append(tokens,
			data.Token{Token: key0, Weight: 3},
			data.Token{Token: key1, Weight: 2},
			data.Token{Token: key2, Weight: 1})
	}

	return data.TokensToTSVector(tokens)
}

// ToQuery tokenizes a Kannada string into Romanized (knphone) Postgres
// tsquery string.
func (kn *Kannada) ToQuery(in string) string {
	key0, key1, key2 := kn.ph.Encode(in)
	if key0 == "" {
		return ""
	}
	return fmt.Sprintf("%s | (%s & %s) ", key2, key1, key0)
}
