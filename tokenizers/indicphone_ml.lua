-- IndicPhone (ML) based on MLphone - https://nadh.in/code/mlphone

local config = {
    -- Number of phonetic keys to use in FTS queries returned by to_query() (1-3)
    num_keys = 2
}

local table_concat = table.concat
local string_find, string_sub, string_gsub = string.find, string.sub, string.gsub
local pairs, ipairs = pairs, ipairs
local math_min = math.min

local vowels = {
    ["അ"] = "A",  ["ആ"] = "A",  ["ഇ"] = "I",  ["ഈ"] = "I",
    ["ഉ"] = "U",  ["ഊ"] = "U",  ["ഋ"] = "R",  ["എ"] = "E",
    ["ഏ"] = "E",  ["ഐ"] = "AI", ["ഒ"] = "O",  ["ഓ"] = "O",
    ["ഔ"] = "O",
}

local consonants = {
    ["ക"] = "K",  ["ഖ"] = "K",  ["ഗ"] = "K",  ["ഘ"] = "K",  ["ങ"] = "NG",
    ["ച"] = "C",  ["ഛ"] = "C",  ["ജ"] = "J",  ["ഝ"] = "J",  ["ഞ"] = "NJ",
    ["ട"] = "T",  ["ഠ"] = "T",  ["ഡ"] = "T",  ["ഢ"] = "T",  ["ണ"] = "N1",
    ["ത"] = "0",  ["ഥ"] = "0",  ["ദ"] = "0",  ["ധ"] = "0",  ["ന"] = "N",
    ["പ"] = "P",  ["ഫ"] = "F",  ["ബ"] = "B",  ["ഭ"] = "B",  ["മ"] = "M",
    ["യ"] = "Y",  ["ര"] = "R",  ["ല"] = "L",  ["വ"] = "V",
    ["ശ"] = "S1", ["ഷ"] = "S1", ["സ"] = "S",  ["ഹ"] = "H",
    ["ള"] = "L1", ["ഴ"] = "Z",  ["റ"] = "R1",
}

local chillus = {
    ["ൽ"] = "L",  ["ൾ"] = "L1", ["ൺ"] = "N1",
    ["ൻ"] = "N",  ["ർ"] = "R1", ["ൿ"] = "K",
}

local compounds = {
    ["ക്ക"] = "K2",  ["ഗ്ഗ"] = "K",   ["ങ്ങ"] = "NG",
    ["ച്ച"] = "C2",  ["ജ്ജ"] = "J",   ["ഞ്ഞ"] = "NJ",
    ["ട്ട"] = "T2",  ["ണ്ണ"] = "N2",
    ["ത്ത"] = "0",   ["ദ്ദ"] = "D",   ["ദ്ധ"] = "D",   ["ന്ന"] = "NN",
    ["ന്ത"] = "N0",  ["ങ്ക"] = "NK",  ["ണ്ട"] = "N1T", ["ബ്ബ"] = "B",
    ["പ്പ"] = "P2",  ["മ്മ"] = "M2",
    ["യ്യ"] = "Y",   ["ല്ല"] = "L2",  ["വ്വ"] = "V",   ["ശ്ശ"] = "S1",
    ["സ്സ"] = "S",   ["ള്ള"] = "L12",
    ["ഞ്ച"] = "NC",  ["ക്ഷ"] = "KS1", ["മ്പ"] = "MP",
    ["റ്റ"] = "T",   ["ന്റ"] = "NT",
    ["്രി"] = "R",   ["്രു"] = "R",
}

local modifiers = {
    ["ാ"] = "",   ["ഃ"] = "",   ["്"] = "",   ["ൃ"] = "R",
    ["ം"] = "3",  ["ി"] = "4",  ["ീ"] = "4",  ["ു"] = "5",
    ["ൂ"] = "5",  ["െ"] = "6",  ["േ"] = "6",  ["ൈ"] = "7",
    ["ൊ"] = "8",  ["ോ"] = "8",  ["ൌ"] = "9",  ["ൗ"] = "9",
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
local chillu_keys = sorted_keys(chillus)
local modifier_keys = sorted_keys(modifiers)

-- Strip non-Malayalam characters with a unicode range check.
local function strip_non_ml(s)
    return utils.filter_unicode_range(s, 0x0D00, 0x0D7F)
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
    -- Remove all non-Malayalam characters
    input = strip_non_ml(input)

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

    -- Replace chillus
    for _, k in ipairs(chillu_keys) do
        input = utils.replace_all(input, k, "{" .. chillus[k] .. "}")
    end

    -- Replace all modifiers
    for _, k in ipairs(modifier_keys) do
        input = utils.replace_all(input, k, modifiers[k])
    end

    -- Remove non-alphanumeric characters (this removes the {} grouping)
    input = string_gsub(input, "[^0-9A-Z]", "")

    -- Phonetic exception: uthsavam -> ulsavam pattern
    input = string_gsub(input, "^([AVTSUMO])L([KS])", "%10%2")

    return input
end

-- Encode Malayalam text to three phonetic keys
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
