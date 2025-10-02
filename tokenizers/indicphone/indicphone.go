package indicphone

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/knadh/dictpress/internal/data"
	"github.com/knadh/knphone"
	"gitlab.com/joice/mlphone-go"
)

type Config struct {
	NumMLKeys int
	NumKNKeys int
}

// IndicPhone is a phonetic tokenizer that generates phonetic tokens for
// Indian languages. It is similar to Metaphone for English.
type IndicPhone struct {
	config Config
	kn     *knphone.KNphone
	ml     *mlphone.MLPhone
}

// New returns a new instance of the Kannada tokenizer.
func New(config Config) *IndicPhone {
	if config.NumKNKeys < 0 {
		config.NumKNKeys = 2
	}
	if config.NumMLKeys < 0 {
		config.NumMLKeys = 2
	}

	return &IndicPhone{
		config: config,
		kn:     knphone.New(),
		ml:     mlphone.New(),
	}
}

// ToTokens tokenizes a string and a language returns an array of tsvector tokens.
// eg: [KRM0 KRM] or [KRM:2 KRM:1] with weights.
func (ip *IndicPhone) ToTokens(s string, lang string) ([]string, error) {
	if lang != "kannada" && lang != "malayalam" {
		return nil, errors.New("unknown language to tokenize")
	}

	var (
		chunks = strings.Split(s, " ")
		tokens = make([]data.Token, 0, len(chunks)*3)

		key0, key1, key2 string
	)
	for _, c := range chunks {
		switch lang {
		case "kannada":
			key0, key1, key2 = ip.kn.Encode(c)
		case "malayalam":
			key0, key1, key2 = ip.ml.Encode(c)
		}

		if key0 == "" {
			continue
		}

		tokens = append(tokens,
			data.Token{Token: key0, Weight: 3},
			data.Token{Token: key1, Weight: 2},
			data.Token{Token: key2, Weight: 1})
	}

	return data.TokensToTSVector(tokens), nil
}

// ToQuery tokenizes a Kannada string into Romanized (knphone) Postgres
// tsquery string.
func (ip *IndicPhone) ToQuery(s string, lang string) (string, error) {
	var (
		key0, key1, key2 string
		numKeys          = 0
	)

	switch lang {
	case "kannada":
		key0, key1, key2 = ip.kn.Encode(s)
		numKeys = ip.config.NumKNKeys
	case "malayalam":
		key0, key1, key2 = ip.ml.Encode(s)
		numKeys = ip.config.NumMLKeys
	}

	if key0 == "" {
		return "", nil
	}
	if numKeys == 0 {
		numKeys = 1
	}

	fmt.Println(numKeys)

	// De-duplicate tokens.
	tokens := slices.Compact([]string{key2, key1, key0})

	if len(tokens) == 0 {
		return "", nil
	}

	return strings.Join(tokens[:min(len(tokens), numKeys)], " | "), nil
}
