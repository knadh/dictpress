# Data structure

dictpress is language agnostic and has no concept of language semantics. It stores all data in an SQLite database file in just two tables entries and relations. To make a universal dictionary interface possible, it treats all dictionary entries as UTF-8 strings that can be searched with SQLite's fulltext capabilities by storing search tokens alongside them. The tokens that encode and make the entries searchable can be anything—simple stemmed words or phonetic hashes like Metaphone.

All content, the entry words and their definitions, are stored in the `entries` table. Entry-definition many-to-many relationships are stored in the `relations` table, represented by `from_id` (entry word) -> `to_id` (definition word), where both IDs refer to the `entries` table.

### Built-in fulltext languages
dictpress bundles a Snowball stemming algorithm library which supports basic stemming/tokenization for the following languages.

- arabic
- danish
- dutch
- english
- finnish
- french
- german
- greek
- hungarian
- italian
- norwegian
- portuguese
- romanian
- russian
- spanish
- swedish
- tamil
- turkish

These are specified in the config.toml configuration, eg: `tokenizer = english, tokenizer_type = default` or in CSV imports as `default:english`.

## Database tables

### entries
| Field     | Type      |                                                                                                                                                                                                       |
| --------- | --------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `id`      | `SERIAL`  | Automtaically generated numeric ID used internally                                                                                                                                                    |
| `guid`    | `TEXT`    | Automtaically generated unique id (UUID) used in public facing APIs                                                                                                                                   |
| `content` | `TEXT`    | Words/language content stored as a stringified JSON array. Dictionary word or definition entries. Eg: `["apple"]                                                                                      |
| `initial` | `TEXT`    | The first "alphabet" of the content. For English, for the word `Apple`, the initial is `A`                                                                                                            |
| `weight`  | `INTEGER` | An optional numeric value to sort search results in ascending order. This should only be set (to high numeric values) if an entry has to be ranked higher beyond its natural fulltext search ranking. |
| `tokens`  | `TEXT`    | Fulltext search tokens stored as a stringified JSON array                                                                                                                                             |
| `types`   | `TEXT`    | Types of content as defined in the content as a stringified JSON array. Eg `["noun", "propernoun"]`                                                                                                   |
| `tags`    | `TEXT`    | Optional tags as a stringified JSON array                                                                                                                                                             |
| `phones`  | `TEXT`    | Phonetic (pronunciation) descriptions of the content. Eg: `["ap(ə)l", "aapl"]` for `Apple`                                                                                                            |
| `notes`   | `TEXT`    | Optional additional textual description of the content.                                                                                                                                               |
| `status`  | `TEXT`    | `enabled` (show the entry in search results), `disabled` (hide from search results), `pending` (public submission pending moderator review)                                                           |


### relations
| Field     | Type      |                                                                                                                                             |
| --------- | --------- | ------------------------------------------------------------------------------------------------------------------------------------------- |
| `from_id` | `INTEGER` | ID of the head word or the dictionary entry in the entries table                                                                            |
| `to_id`   | `INT`     | ID of the definition content in the entry table                                                                                             |
| `types`   | `TEXT`    | Types defining the definition as defined in the config as a stringified JSON array. Eg `["noun", "propernoun"]`                             |
| `weight`  | `INTEGER` | An optional numeric value to order definition results                                                                                       |
| `tags`    | `TEXT`    | Optional tags as a stringified JSON array                                                                                                   |
| `status`  | `TEXT`    | `enabled` (show the entry in search results), `disabled` (hide from search results), `pending` (public submission pending moderator review) |
| `notes`   | `TEXT`    | Optional additional textual description of the content.                                                                                     |
