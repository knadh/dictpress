-- Helper library available as `utils` in all tokenizer scripts.
utils = {}

-- Split string by whitespace(s) and return an iterator.
function utils.words(s)
    return s:gmatch("%S+")
end

-- Trim whitespace from around the string.
function utils.trim(s)
    return s:match("^%s*(.-)%s*$")
end

-- Split string by delimiter (plain text, not pattern).
function utils.split(s, delim)
    local result = {}
    -- Escape Lua pattern magic characters in delimiter
    local escaped = delim:gsub("([%.%+%-%*%?%[%]%^%$%(%)%%])", "%%%1")
    for match in (s .. delim):gmatch("(.-)" .. escaped) do
        table.insert(result, match)
    end

    return result
end

-- Replace all literal occurrences of 'old' with 'new' in string 's'
function utils.replace_all(s, old, new)
    if old == "" then return s end

    local i = 1
    local result = {}
    local old_len = #old
    local string_find, string_sub = string.find, string.sub

    while true do
        local j = string_find(s, old, i, true)
        if not j then
            result[#result + 1] = string_sub(s, i)
            break
        end
        result[#result + 1] = string_sub(s, i, j - 1)
        result[#result + 1] = new
        i = j + old_len
    end

    return table.concat(result)
end

-- Replace all occurrences of Lua pattern 'pattern' with 'repl' in string 's'.
-- Eg: replace_all_pattern("foo.bar", ".", "-"), here '.' is any character.
function utils.replace_all_pattern(s, pattern, repl)
    return (s:gsub(pattern, repl))
end

-- Filter string to keep only codepoints within a Unicode range.
-- Returns empty string on invalid UTF-8 input.
function utils.filter_unicode_range(s, min_cp, max_cp)
    local utf8 = utf8 or require("utf8")
    local result = {}

    -- utf8.codes returns (iter_func, state, start) for generic for loop
    local ok, iter, state, start = pcall(utf8.codes, s)
    if not ok then
        return ""
    end

    for _, cp in iter, state, start do
        if cp >= min_cp and cp <= max_cp then
            result[#result + 1] = utf8.char(cp)
        end
    end

    return table.concat(result)
end
