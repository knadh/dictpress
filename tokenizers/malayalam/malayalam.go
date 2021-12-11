package main

import (
	"fmt"
	"strings"

	"github.com/knadh/dictpress/internal/data"
	"gitlab.com/joice/mlphone-go"
)

// Malayalam is the Kannada tokenizer that generates tsvectors for romanized (knphone algorithm)
// Kannada strings
type Malayalam struct {
	ph *mlphone.MLPhone
}

// New returns a new instance of the Malayalam tokenizer.
func New() (data.Tokenizer, error) {
	return &Malayalam{
		ph: mlphone.New(),
	}, nil
}

// ID returns the ID of the tokenizer.
func (*Malayalam) ID() string {
	return "malayalam"
}

// Name returns the name of the tokenizer.
func (*Malayalam) Name() string {
	return "Malayalam"
}

// ToTokens tokenizes a Malaylam string and returns an array of tsvector tokens.
func (m *Malayalam) ToTokens(s string) []string {
	var (
		chunks = strings.Split(s, " ")
		tokens = make([]data.Token, 0, len(chunks)*3)
	)
	for _, c := range chunks {
		key0, key1, key2 := m.ph.Encode(c)
		tokens = append(tokens,
			data.Token{Token: key0, Weight: 3},
			data.Token{Token: key1, Weight: 2},
			data.Token{Token: key2, Weight: 1})
	}

	return data.TokensToTSVector(tokens)
}

// ToQuery tokenizes a Malayalam string into Romanized (mlphone) Postgres
// tsquery string.
func (ml *Malayalam) ToQuery(in string) string {
	key0, key1, key2 := ml.ph.Encode(in)
	if key0 == "" {
		return ""
	}
	return fmt.Sprintf("%s | (%s & %s) ", key2, key1, key0)
}
