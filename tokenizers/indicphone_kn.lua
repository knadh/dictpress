-- IndicPhone (Kannada) based on KNphone - https://github.com/knadh/knphone

local config = {
    -- Number of phonetic keys to use in FTS queries returned by to_query() (1-3)
    num_keys = 2
}

local table_concat = table.concat
local string_find, string_gsub = string.find, string.gsub
local pairs, ipairs = pairs, ipairs
local math_min = math.min

local vowels = {
    ["ಅ"] = "A",  ["ಆ"] = "A",  ["ಇ"] = "I",  ["ಈ"] = "I",
    ["ಉ"] = "U",  ["ಊ"] = "U",  ["ಋ"] = "R",  ["ಎ"] = "E",
    ["ಏ"] = "E",  ["ಐ"] = "AI", ["ಒ"] = "O",  ["ಓ"] = "O",
    ["ಔ"] = "O",
}

local consonants = {
    ["ಕ"] = "K",  ["ಖ"] = "K",  ["ಗ"] = "K",  ["ಘ"] = "K",  ["ಙ"] = "NG",
    ["ಚ"] = "C",  ["ಛ"] = "C",  ["ಜ"] = "J",  ["ಝ"] = "J",  ["ಞ"] = "NJ",
    ["ಟ"] = "T",  ["ಠ"] = "T",  ["ಡ"] = "T",  ["ಢ"] = "T",  ["ಣ"] = "N1",
    ["ತ"] = "0",  ["ಥ"] = "0",  ["ದ"] = "0",  ["ಧ"] = "0",  ["ನ"] = "N",
    ["ಪ"] = "P",  ["ಫ"] = "F",  ["ಬ"] = "B",  ["ಭ"] = "B",  ["ಮ"] = "M",
    ["ಯ"] = "Y",  ["ರ"] = "R",  ["ಲ"] = "L",  ["ವ"] = "V",
    ["ಶ"] = "S1", ["ಷ"] = "S1", ["ಸ"] = "S",  ["ಹ"] = "H",
    ["ಳ"] = "L1", ["ೞ"] = "Z",  ["ಱ"] = "R1",
}

local compounds = {
    ["ಕ್ಕ"] = "K2",  ["ಗ್ಗ"] = "K",   ["ಙ್ಙ"] = "NG",
    ["ಚ್ಚ"] = "C2",  ["ಜ್ಜ"] = "J",   ["ಞ್ಞ"] = "NJ",
    ["ಟ್ಟ"] = "T2",  ["ಣ್ಣ"] = "N2",
    ["ತ್ತ"] = "0",   ["ದ್ದ"] = "D",   ["ದ್ಧ"] = "D",   ["ನ್ನ"] = "NN",
    ["ಬ್ಬ"] = "B",
    ["ಪ್ಪ"] = "P2",  ["ಮ್ಮ"] = "M2",
    ["ಯ್ಯ"] = "Y",   ["ಲ್ಲ"] = "L2",  ["ವ್ವ"] = "V",   ["ಶ್ಶ"] = "S1",
    ["ಸ್ಸ"] = "S",   ["ಳ್ಳ"] = "L12",
    ["ಕ್ಷ"] = "KS1",
}

local modifiers = {
    ["ಾ"] = "",   ["ಃ"] = "",   ["್"] = "",   ["ೃ"] = "R",
    ["ಂ"] = "3",  ["ಿ"] = "4",  ["ೀ"] = "4",  ["ು"] = "5",
    ["ೂ"] = "5",  ["ೆ"] = "6",  ["ೇ"] = "6",  ["ೈ"] = "7",
    ["ೊ"] = "8",  ["ೋ"] = "8",  ["ೌ"] = "9",
}

-- Build sorted keys list (longer strings first for correct replacement order)
local function sorted_keys(tbl)
    local keys = {}
    for k in pairs(tbl) do
        keys[#keys + 1] = k
    end

    table.sort(keys, function(a, b) return #a > #b end)
    return keys
end

-- Pre-compute sorted keys for each mapping table.
local compound_keys = sorted_keys(compounds)
local consonant_keys = sorted_keys(consonants)
local vowel_keys = sorted_keys(vowels)
local modifier_keys = sorted_keys(modifiers)

-- Strip non-Kannada characters with a unicode range check.
-- Kannada Unicode block: U+0C80 - U+0CFF
local function strip_non_kn(s)
    return utils.filter_unicode_range(s, 0x0C80, 0x0CFF)
end

-- Replace modified glyphs: glyph followed by modifier
-- Replaces glyph+modifier combinations with glyph_value+modifier_value
local function replace_modified_glyphs(input, glyphs, glyph_keys)
    for _, glyph in ipairs(glyph_keys) do
        local glyph_val = glyphs[glyph]
        for _, mod in ipairs(modifier_keys) do
            local combined = glyph .. mod
            -- Only call replace_all if the combined string exists
            if string_find(input, combined, 1, true) then
                input = utils.replace_all(input, combined, glyph_val .. modifiers[mod])
            end
        end
    end
    return input
end

-- Main processing pipeline
local function process(input)
    -- Remove all non-Kannada characters
    input = strip_non_kn(input)

    -- All replacements are grouped in {} to maintain separability

    -- Replace modified compounds first (compound + modifier)
    input = replace_modified_glyphs(input, compounds, compound_keys)

    -- Replace unmodified compounds
    for _, k in ipairs(compound_keys) do
        input = utils.replace_all(input, k, "{" .. compounds[k] .. "}")
    end

    -- Replace modified consonants and vowels
    input = replace_modified_glyphs(input, consonants, consonant_keys)
    input = replace_modified_glyphs(input, vowels, vowel_keys)

    -- Replace unmodified consonants
    for _, k in ipairs(consonant_keys) do
        input = utils.replace_all(input, k, "{" .. consonants[k] .. "}")
    end

    -- Replace unmodified vowels
    for _, k in ipairs(vowel_keys) do
        input = utils.replace_all(input, k, "{" .. vowels[k] .. "}")
    end

    -- Replace all modifiers
    for _, k in ipairs(modifier_keys) do
        input = utils.replace_all(input, k, modifiers[k])
    end

    -- Remove non-alphanumeric characters (this removes the {} grouping)
    input = string_gsub(input, "[^0-9A-Z]", "")

    return input
end

-- Encode Kannada text to three phonetic keys
local function encode(input)
    -- key2: full phonetic representation with all modifiers
    local key2 = process(input)

    -- key1: removes doubled sounds and vowel modifiers [2,4-9]
    local key1 = string_gsub(key2, "[24-9]", "")

    -- key0: removes hard sounds, doubled sounds, and modifiers [1,2,4-9]
    local key0 = string_gsub(key2, "[1-24-9]", "")

    return key0, key1, key2
end


-- #############################################################################

-- Tokenize text for indexing: returns weighted tsvector tokens
-- Format: {"TOKEN:weight", ...} where weight 3=highest, 1=lowest
function tokenize(text, lang)
    local tokens = {}
    for word in utils.words(text) do
        local key0, key1, key2 = encode(word)
        if key0 ~= "" then
            -- Add tokens with weights (key0 most broad = weight 3, key2 most specific = weight 1)
            tokens[#tokens + 1] = key0 .. ":3"
            if key1 ~= key0 then
                tokens[#tokens + 1] = key1 .. ":2"
            end
            if key2 ~= key1 and key2 ~= key0 then
                tokens[#tokens + 1] = key2 .. ":1"
            end
        end
    end
    return tokens
end

-- Convert search query to FTS5 query string
-- Returns keys joined with " OR " for SQLite FTS5 OR matching
function to_query(text, lang)
    local key0, key1, key2 = encode(text)

    if key0 == "" then
        return ""
    end

    -- Collect unique keys (most specific first)
    local keys = {}
    local seen = {}

    if key2 ~= "" and not seen[key2] then
        keys[#keys + 1] = key2
        seen[key2] = true
    end
    if key1 ~= "" and not seen[key1] then
        keys[#keys + 1] = key1
        seen[key1] = true
    end
    if key0 ~= "" and not seen[key0] then
        keys[#keys + 1] = key0
        seen[key0] = true
    end

    if #keys == 0 then
        return ""
    end

    -- Return up to num_keys keys
    local result = {}
    for i = 1, math_min(#keys, config.num_keys) do
        result[#result + 1] = keys[i]
    end

    return table_concat(result, " OR ")
end
