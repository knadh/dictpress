# Tokenization

dictpress uses tokenizers to convert dictionary entries into searchable tokens for SQLite FTS5 fulltext search. There are two types of tokenizers: built-in default tokenizers and custom Lua tokenizers.

## Default tokenizers

dictpress bundles Snowball stemming algorithms for 18 languages. These tokenizers lowercase and stem words to their root forms. For example, "running" becomes "run" in English so that searches for "run", "running" both are a match.

**Supported languages:** arabic, danish, dutch, english, finnish, french, german, greek, hungarian, italian, norwegian, portuguese, romanian, russian, spanish, swedish, tamil, turkish

**Configuration:**
```toml
[lang.english]
tokenizer = "english"
tokenizer_type = "default"
```

**CSV import format:** `default:english` in the tokenizer column.

## Lua tokenizers

For languages or use cases not covered by built-in stemmers, custom Lua tokenizers can be used. These are `.lua` scripts placed in the `./tokenizers` directory (defined in config.toml). Lua tokenizers are useful for phonetic search (like Metaphone), transliteration-based search, or any custom tokenization logic.

**Configuration**
In the config.toml file:

```toml
[lang.malayalam]
tokenizer = "indicphone_ml.lua"
tokenizer_type = "lua"
```

**CSV import format** `lua:indicphone_ml.lua` in the `tokenizer` column.

## Writing a custom Lua tokenizer

A Lua tokenizer must export two functions: `tokenize()` for indexing and `to_query()` for search queries.

### Required functions

```lua
-- Convert text to tokens for indexing.
-- Returns: table of token strings
function tokenize(text, lang)
    local tokens = {}
    for word in utils.words(text) do
        tokens[#tokens + 1] = word:lower()
    end
    return tokens
end

-- Convert search query to FTS5 query format.
-- Returns: string (FTS5 query)
function to_query(text, lang)
    return text:lower()
end
```

### Global utils

All Lua tokenizers have access to a global `utils` table with helper functions:

| Function                                        | Description                                         |
| ----------------------------------------------- | --------------------------------------------------- |
| `utils.words(s)`                                | Returns an iterator over whitespace-separated words |
| `utils.trim(s)`                                 | Trims leading/trailing whitespace                   |
| `utils.split(s, delim)`                         | Splits string by delimiter (plain text)             |
| `utils.replace_all(s, old, new)`                | Replaces all literal occurrences                    |
| `utils.replace_all_pattern(s, pattern, repl)`   | Replaces all Lua pattern matches                    |
| `utils.filter_unicode_range(s, min_cp, max_cp)` | Keeps only characters in Unicode codepoint range    |

### Example

See [indicphone_ml.lua](https://github.com/knadh/dictpress/blob/master/tokenizers/indicphone_ml.lua) for a full example of a phonetic tokenizer for Malayalam. It converts Malayalam script to phonetic keys, enabling fuzzy search that matches words by pronunciation rather than exact spelling.
