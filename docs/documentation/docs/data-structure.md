# Data structure

dictpress is language agnostic and has no concept of language semantics. It stores all data in a Postgres database in just two tables entries and relations. To make a universal dictionary interface possible, it treats all dictionary entries as UTF-8 strings that can be accurately searched with Postgres DB's fulltext capabilities by storing tsvector tokens alongside them. The tokens that encode and make the entries searchable can be anything—simple stemmed words or phonetic hashes like Metaphone.

Postgres comes with built-in tokenizers for two dozen languages (\dFd to see the full list on psql). 

- There can be any number of languages defined in the dictionary. eg: 'english', 'malayalam', 'kannada' etc.
- All content, the entry words and their definitions, are stored in the `entries` table
- Entry-definition many-to-many relationships are stored in the `relations` table, represented by `from_id` (entry word) -> `to_id` (definition word), where both IDs refer to the `entries` table.

## Database tables

### entries
| Field     | Type   |                                                                                                                                     |
|-----------|------------|-------------------------------------------------------------------------------------------------------------------------------------|
| `id`      | `SERIAL`   | Automtaically generated numeric ID used internally                                                                                                                                    |
| `guid`    | `TEXT`     | Automtaically generated unique id (UUID) used in public facing APIs                                      |
| `content` | `TEXT`     | Actual language content. Dictionary word or definition entries                                                                      |
| `initial` | `TEXT`     | The first "alphabet" of the content. For English, for the word `Apple`, the initial is `A`                                          |
| `weight`  | `INT`      | An optional numeric value to sort search results in ascending order                                                                                   |
| `tokens`  | `TSVECTOR` | Fulltext search tokens. For English, Postgres' built-in tokenizer gives `to_tsvector('fully conditioned')` = `'condit':2 'fulli':1` |
| `types`   | `TEXT[]`   | Types of content as defined in the content. Eg `{noun, propernoun}`                                                                    |
| `tags`    | `TEXT[]`   | Optional tags                                                                                                                       |
| `phones`  | `TEXT[]`   | Phonetic (pronunciation) descriptions of the content. Eg: `{ap(ə)l, aapl}` for `Apple`                                              |
| `notes`   | `TEXT`     | Optional additional textual description of the content.                                                                                                                 |
| `status`  | `ENUM`     | `enabled` (show the entry in search results), `disabled` (hide from search results), `pending` (public submission pending moderator review)|


### relations
| Field     | Type     |                                                                        |
|-----------|----------|------------------------------------------------------------------------|
| `from_id` | `INT`    | ID of the head word or the dictionary entry in the entries table       |
| `to_id`   | `INT`    | ID of the definition content in the entry table                        |
| `types`   | `TEXT[]` | Types defining the definition as defined in the config. Eg `{noun, propernoun}`                                                                    |
| `weight`  | `INT`    | An optional numeric value to order definition results                  |
| `tags`    | `TEXT[]` | Optional tags                                                          |
| `status`  | `ENUM`     | `enabled` (show the entry in search results), `disabled` (hide from search results), `pending` (public submission pending moderator review)|
| `notes`   | `TEXT`   | Optional additional textual description of the content.                                                    |
