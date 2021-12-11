# DictPress

**DictPress** is a stand-alone, single-binary server application for building and publishing dictionary websites. [Alar](https://alar.ink) (Kannada-English dictionary) is an example in production.

- Generic entry-relation (many)-entry structure for dictionary data in a Postgres database with just two tables (entries, relations)
- Entirely built on top of Postgres full text search using [tsvector](https://www.postgresql.org/docs/10/datatype-textsearch.html) tokens
- Works with any language. Plug in external tokenizers or use a built in tokenizer supported by Postgres for search
- Possible to have entries and definitions in any number of languages in a single database
- HTTP/JSON REST like APIs
- Themes and templates to publish dictionary websites
- Paginated A-Z (or any alphabet for any language) glossary generation for dictionary words

## How it works
DictPress has no concept of language or semantics. To make a universal dictionary interface possible, it treats all data as unicode strings that can be searched with Postgres DB's fulltext capabilities, where search tokens are generated and stored along with the dictionary entries. There are several built-in fulltext dictionaries and tokenizers that Postgres supports out of the box, mostly European languages (`\dFd` lists installed dictionaries). For languages that do not have fulltext dictionaries, it is possible to generate search tokens manually using an external algorithm. For instance, to make a phonetic English dictionary, [Metaphone](https://en.wikipedia.org/wiki/Metaphone) tokens can be generated manually and search queries issued against them.

### Concepts
- There can be any number of languages defined in the dictionary. eg: 'english', 'malayalam', 'kannada' etc.
- All content, the entry words and their definitions, are stored in the `entries` table
- Entry-definition many-to-many relationships are stored in the `relations` table, represented by `from_id` (entry word) -> `to_id` (definition word), where both IDs refer to the `entries` table.

## entries table schema
| `id`      | `SERIAL`   |                                                                                                                                     |
|-----------|------------|-------------------------------------------------------------------------------------------------------------------------------------|
| `guid`    | `TEXT`     | UUID used in public facing APIs                                      |
| `content` | `TEXT`     | Actual language content. Dictionary word or definition entries                                                                      |
| `initial` | `TEXT`     | The first "alphabet" of the content. For English, for the word `Apple`, the initial is `A`                                          |
| `weight`  | `INT`      | An optional numeric value to order search results                                                                                   |
| `tokens`  | `TSVECTOR` | Fulltext search tokens. For English, Postgres' built-in tokenizer gives `to_tsvector('fully conditioned')` = `'condit':2 'fulli':1` |
| `lang`    | `TEXT`     | String representing the language of content. Eg: `en`, `english`                                                                    |
| `types`   | `TEXT[]`   | Strings describing the types of content. Eg `{noun, propernoun}`                                                                    |
| `tags`    | `TEXT[]`   | Optional tags                                                                                                                       |
| `phones`  | `TEXT[]`   | Phonetic (pronunciation) descriptions of the content. Eg: `{ap(ə)l, aapl}` for `Apple`                                              |
| `notes`   | `TEXT`     | Optional text notes                                                                                                                 |
## relations table schema
| `from_id` | `INT`    | ID of the head word or the dictionary entry in the entries table       |
|-----------|----------|------------------------------------------------------------------------|
| `to_id`   | `INT`    | ID of the definition content in the entry table                        |
| `types`   | `TEXT[]` | Strings describing the types of this relation. Eg `{noun, propernoun}` |
| `weight`  | `INT`    | An optional numeric value to order definition results                  |
| `tags`    | `TEXT[]` | Optional tags                                                          |
| `notes`   | `TEXT`   | Optional text notes                                                    |

# Installation
1. Download the latest release [release](https://github.com/knadh/dictpress/releases).
1. Run `./dictpress --new` to generate a sample config.toml. Edit the config.
1. Define your dictionary's languages and properties along with other config in `config.toml`
1. Run `./dictpress --install` to install the Postgres DB schema.
1. Populate the `entries` and `relations` tables with your dictionary data. See the "Sample dictionary" section below
1. Run the dictionary server: `./dictpress`
 
## Dictionary query API
```shell
# /dictionary/from_lang/to_lang/word
# Optional query params: ?type=noun&type=noun2&tag=a&tag=b ...
curl localhost:8080/api/dictionary/english/english/apple
```

## Sample dictionary
The `sample/sample.sql` shows how to setup an English-English and English-Italian dictionary. Retaining the English-Italian config in the generated sample config file, execute `sample/sample.sql` on your database.

Then try:
```shell
curl localhost:8080/api/dictionary/english/english/apple
curl localhost:8080/api/dictionary/english/italian/apple
```

## Themes
See the [alar-dict/alar.ink](https://github.com/alar-dict/alar.ink) repository that powers the [Alar](https://alar.ink) dictionary. A theme is a directory with a collection of Go HTML templates. Run a theme by passing `./dictpress --site=theme_dir`.

Licensed under the AGPL v3 license.
