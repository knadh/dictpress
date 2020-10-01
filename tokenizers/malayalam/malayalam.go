package malayalam

import (
	"fmt"

	"github.com/knadh/dictmaker/search"
	"gitlab.com/joice/mlphone-go"
)

// Malayalam is the Kannada tokenizer that generates tsvectors for romanized (knphone algorithm)
// Kannada strings
type Malayalam struct {
	ph *mlphone.MLPhone
}

// New returns a new instance of the Malayalam tokenizer.
func New() (search.Tokenizer, error) {
	return &Malayalam{
		ph: mlphone.New(),
	}, nil
}

// ID returns the ID of the tokenizer.
func (*Malayalam) ID() string {
	return "kannada"
}

// Name returns the name of the tokenizer.
func (*Malayalam) Name() string {
	return "Kannada"
}

// Tokenize tokenizes a Kannada string into Romanized (mlphone) Postgres
// tsquery string.
func (ml *Malayalam) Tokenize(in string) string {
	key0, key1, key2 := ml.ph.Encode(in)
	if key0 == "" {
		return ""
	}
	return fmt.Sprintf("%s | (%s & %s) ", key2, key1, key0)
}
