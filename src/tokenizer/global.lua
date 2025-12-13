-- Helper library available as `utils` in all tokenizer scripts.
utils = {}

-- Split string by whitespace(s), returns iterator.
function utils.words(s)
    return s:gmatch("%S+")
end

-- Trim whitespace from around the string.
function utils.trim(s)
    return s:match("^%s*(.-)%s*$")
end

-- Split string by delimiter.
function utils.split(s, delim)
    local result = {}
    for match in (s .. delim):gmatch("(.-)" .. delim) do
        table.insert(result, match)
    end
    return result
end
