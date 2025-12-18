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
-- (Manglish) English chars to Indicphone transliteration mappings

-- Placeholders for uppercase retroflex markers (captured before lowercasing)
local RETROFLEX_N = "\1"  -- N → N1
local RETROFLEX_L = "\2"  -- L → L1
local RETROFLEX_R = "\3"  -- R → R1

-- English compound patterns (longest-first).
-- For patterns with same length, order in the list takes priority.
local en_compounds_list = {
    {"ntha", "N0"},   {"njch", "NC"},   {"ksha", "KS1"},  {"nkha", "NK"},
    {"zhla", "L1"},   {"zhna", "N1"},   {"zhra", "R1"},


    {"kka", "K2"},    {"gga", "K"},     {"cca", "C2"},    {"cha", "C"},
    {"tta", "T2"},    {"dda", "T2"},    {"ppa", "P2"},    {"mma", "M2"},
    {"lla", "L2"},    {"nna", "NN"},    {"rra", "T"},     {"yya", "Y"},
    {"vva", "V"},     {"ssa", "S"},
    {"sha", "S1"},    {"zha", "Z"},     {"nga", "NG"},    {"nja", "NJ"},
    {"nda", "N1T"},   {"nta", "NT"},    {"mpa", "MP"},    {"nka", "NK"},
    {"tth", "0"},     {"nth", "N0"},    {"nch", "NC"},    {"ksh", "KS1"},
    {"zhl", "L1"},    {"zhn", "N1"},    {"zhr", "R1"},
    {"zl", "L1"},     {"zn", "N1"},     {"zr", "R1"},

    {"kk", "K2"},     {"gg", "K"},      {"cc", "C2"},     {"ch", "C"},
    {"tt", "T2"},     {"dd", "T2"},     {"pp", "P2"},     {"mm", "M2"},
    {"ll", "L2"},     {"nn", "NN"},     {"rr", "T"},      {"yy", "Y"},
    {"vv", "V"},      {"ss", "S"},
    {"sh", "S1"},     {"zh", "Z"},      {"ng", "NG"},     {"nj", "NJ"},
    {"th", "0"},      {"dh", "0"},      {"nd", "N1T"},    {"nt", "NT"},
    {"mp", "MP"},     {"nk", "NK"},     {"ph", "F"},      {"bh", "B"},
    {"kh", "K"},      {"gh", "K"},      {"jh", "J"},
}

-- Build a lookup table from list
local en_compounds = {}
for _, pair in ipairs(en_compounds_list) do
    en_compounds[pair[1]] = pair[2]
end

-- Single English consonants
local en_consonants = {
    ["k"] = "K",  ["g"] = "K",  ["c"] = "C",  ["j"] = "J",
    ["t"] = "T",  ["d"] = "T",  -- retroflex (standard Manglish convention)
    ["n"] = "N",  ["p"] = "P",  ["f"] = "F",  ["b"] = "B",  ["m"] = "M",
    ["y"] = "Y",  ["r"] = "R",  ["l"] = "L",  ["v"] = "V",
    ["s"] = "S",  ["h"] = "H",  ["z"] = "Z",
    -- Retroflex placeholders (from uppercase N/L/R)
    [RETROFLEX_N] = "N1",  [RETROFLEX_L] = "L1",  [RETROFLEX_R] = "R1",
}

-- English vowels - standalone (at word start or after another vowel)
local en_vowels_standalone = {
    ["aa"] = "A",  ["ai"] = "AI", ["au"] = "O",  ["ou"] = "O",
    ["ee"] = "I",  ["ii"] = "I",  ["oo"] = "U",  ["uu"] = "U",
    ["ea"] = "I",  ["ie"] = "I",  ["oa"] = "O",
    ["a"] = "A",   ["i"] = "I",   ["u"] = "U",   ["e"] = "E",  ["o"] = "O",
}

-- English vowels - modifiers (after consonants)
local en_vowels_modifier = {
    ["aa"] = "",   ["ai"] = "7",  ["au"] = "9",  ["ou"] = "9",
    ["ee"] = "4",  ["ii"] = "4",  ["oo"] = "5",  ["uu"] = "5",
    ["ea"] = "4",  ["ie"] = "4",  ["oa"] = "8",
    ["a"] = "",    ["i"] = "4",   ["u"] = "5",   ["e"] = "6",  ["o"] = "8",
}

-- Sorted pattern lengths for English matching
local en_compound_lengths = {4, 3, 2}
local en_vowel_lengths = {2, 1}

-- Preprocess English input: handle uppercase retroflex markers, then lowercase
local function preprocess_en(input)
    -- 1. First, replace uppercase retroflex markers with placeholders
    --    This must happen BEFORE lowercasing to preserve the distinction
    input = string_gsub(input, "N", RETROFLEX_N)
    input = string_gsub(input, "L", RETROFLEX_L)
    input = string_gsub(input, "R", RETROFLEX_R)

    -- 2. Now lowercase the rest
    input = string.lower(input)

    -- 3. Normalize common variations
    input = string_gsub(input, "w", "v")
    input = string_gsub(input, "x", "ks")
    input = string_gsub(input, "q", "k")

    -- 4. Remove non-alphabetic characters (but keep our placeholders)
    input = string_gsub(input, "[^a-z\1\2\3]", "")

    return input
end

-- Core transliteration processing: English input → phonetic string
local function transliterate(input)
    input = preprocess_en(input)

    local output = {}
    local pos = 1
    local len = #input
    local after_consonant = false

    while pos <= len do
        local matched = false
        local char = string_sub(input, pos, pos)

        -- Try compound consonants first (longest match)
        for _, plen in ipairs(en_compound_lengths) do
            if pos + plen - 1 <= len and not matched then
                local substr = string_sub(input, pos, pos + plen - 1)
                if en_compounds[substr] then
                    output[#output + 1] = en_compounds[substr]
                    pos = pos + plen
                    after_consonant = true
                    matched = true
                end
            end
        end

        if not matched then
            -- Try vowel patterns (longest first)
            for _, vlen in ipairs(en_vowel_lengths) do
                if pos + vlen - 1 <= len and not matched then
                    local substr = string_sub(input, pos, pos + vlen - 1)

                    if after_consonant and en_vowels_modifier[substr] then
                        -- Vowel modifier after consonant
                        local mod = en_vowels_modifier[substr]
                        if mod ~= "" then
                            output[#output + 1] = mod
                        end
                        pos = pos + vlen
                        after_consonant = false
                        matched = true
                    elseif en_vowels_standalone[substr] then
                        -- Standalone vowel (word start or after vowel)
                        output[#output + 1] = en_vowels_standalone[substr]
                        pos = pos + vlen
                        after_consonant = false
                        matched = true
                    end
                end
            end
        end

        if not matched then
            -- Try single consonants (including retroflex placeholders)
            if en_consonants[char] then
                output[#output + 1] = en_consonants[char]
                pos = pos + 1
                after_consonant = true
                matched = true
            end
        end

        if not matched then
            -- Skip unrecognized character
            pos = pos + 1
        end
    end

    local result = table_concat(output)

    -- Apply phonetic exception: uthsavam -> ulsavam pattern
    result = string_gsub(result, "^([AVTSUMO])L([KS])", "%10%2")

    -- Handle word-final anusvara: final M after consonant becomes nasal marker (3)
    -- This handles common patterns like "malayalam" → "malayaLaM" (മലയാളം)
    result = string_gsub(result, "([^AEIOU])M$", "%13")

    return result
end

-- Encode English transliteration to three phonetic keys
local function encode_en(input)
    local key2 = transliterate(input)
    local key1 = string_gsub(key2, "[24-9]", "")
    local key0 = string_gsub(key2, "[1-24-9]", "")
    return key0, key1, key2
end

-- Check if string contains Malayalam characters (U+0D00-U+0D7F range)
local function has_native(text)
    return string_find(text, "[\xE0\xB4\x80-\xE0\xB5\xBF]") ~= nil
end

-- #################
-- Public functions

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
-- Auto-detects Malayalam vs Manglish input
-- Returns keys joined with " OR " for SQLite FTS5 OR matching
function to_query(text, lang)
    local key0, key1, key2

    -- If no Malayalam chars, use English transliteration
    if not has_native(text) then
        key0, key1, key2 = encode_en(text)
    else
        key0, key1, key2 = encode(text)
    end

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
