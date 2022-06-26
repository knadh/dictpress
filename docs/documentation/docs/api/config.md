# Config

### GET /api/config
Retrieve the dictionary configuration

#### Request
```bash
curl http://localhost:9000/api/config
```

**Response**

```json
{
  "data": {
    "root_url": "http://localhost:9000",
    "languages": {
      "english": {
        "name": "English",
        "types": {
          "abbr": "Abbreviation",
          "adj": "Adjective",
          "adv": "Adverb",
          "auxv": "Auxiliary verb",
          "conj": "Conjugation",
          "idm": "Idiom",
          "interj": "Interjection",
          "noun": "Noun",
          "pfx": "Prefix",
          "ph": "Phrase",
          "phrv": "Phrasal verb",
          "prep": "Preposition",
          "pron": "Pronoun",
          "propn": "Proper Noun",
          "sfx": "Suffix",
          "verb": "Verb"
        },
        "tokenizer": "english",
        "tokenizer_type": "postgres"
      },
      "italian": {
        "name": "Italian",
        "types": {
          "adj": "Adjective",
          "noun": "Noun",
          "verb": "Verb"
        },
        "tokenizer": "italian",
        "tokenizer_type": "postgres"
      },
      "kannada": {
        "name": "Kannada",
        "types": {
          "adj": "Adjective",
          "noun": "Noun",
          "verb": "Verb"
        },
        "tokenizer": "indicphone",
        "tokenizer_type": "custom"
      }
    },
    "version": "v0.3.0",
    "build": "v0.3.0 (#38a1927 2022-06-26T07:56:05+0000)"
  }
}
```