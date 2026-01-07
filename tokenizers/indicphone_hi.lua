-- IndicPhone (Hindi) - Phonetic hashing for Hindi/Devanagari
-- Based on MLphone/KNphone - https://nadh.in/code/mlphone

local config = {
    -- Number of phonetic keys to use in FTS queries returned by to_query() (1-3)
    num_keys = 2
}

local table_concat = table.concat
local string_find, string_sub, string_gsub = string.find, string.sub, string.gsub
local pairs, ipairs = pairs, ipairs
local math_min = math.min

local vowels = {
    ["अ"] = "A",  ["आ"] = "A",  ["इ"] = "I",  ["ई"] = "I",
    ["उ"] = "U",  ["ऊ"] = "U",  ["ऋ"] = "R",  ["ॠ"] = "R",
    ["ए"] = "E",  ["ऐ"] = "AI", ["ओ"] = "O",  ["औ"] = "O",
}

local consonants = {
    -- Velars (कवर्ग)
    ["क"] = "K",  ["ख"] = "K",  ["ग"] = "K",  ["घ"] = "K",  ["ङ"] = "NG",
    -- Palatals (चवर्ग)
    ["च"] = "C",  ["छ"] = "C",  ["ज"] = "J",  ["झ"] = "J",  ["ञ"] = "NJ",
    -- Retroflexes (टवर्ग)
    ["ट"] = "T",  ["ठ"] = "T",  ["ड"] = "T",  ["ढ"] = "T",  ["ण"] = "N1",
    -- Dentals (तवर्ग) - marked with 0 to distinguish from retroflex
    ["त"] = "0",  ["थ"] = "0",  ["द"] = "0",  ["ध"] = "0",  ["न"] = "N",
    -- Labials (पवर्ग)
    ["प"] = "P",  ["फ"] = "F",  ["ब"] = "B",  ["भ"] = "B",  ["म"] = "M",
    -- Semi-vowels & Sibilants
    ["य"] = "Y",  ["र"] = "R",  ["ल"] = "L",  ["व"] = "V",
    ["श"] = "S1", ["ष"] = "S1", ["स"] = "S",  ["ह"] = "H",
    -- Nukta variants (borrowed/Persian sounds)
    ["क़"] = "K",  ["ख़"] = "K",  ["ग़"] = "K",
    ["ज़"] = "Z",  ["फ़"] = "F",
    ["ड़"] = "R1", ["ढ़"] = "R1",  -- Flap sounds
    -- Rare
    ["ळ"] = "L1",
}

local compounds = {
    -- Common conjuncts
    ["क्ष"] = "KS1", ["त्र"] = "0R",  ["ज्ञ"] = "GY",  ["श्र"] = "S1R",
    -- Geminates (doubled consonants)
    ["क्क"] = "K2",  ["ख्ख"] = "K",   ["ग्ग"] = "K",   ["घ्घ"] = "K",   ["ङ्ङ"] = "NG",
    ["च्च"] = "C2",  ["छ्छ"] = "C",   ["ज्ज"] = "J",   ["झ्झ"] = "J",   ["ञ्ञ"] = "NJ",
    ["ट्ट"] = "T2",  ["ठ्ठ"] = "T",   ["ड्ड"] = "T",   ["ढ्ढ"] = "T",   ["ण्ण"] = "N2",
    ["त्त"] = "0",   ["थ्थ"] = "0",   ["द्द"] = "0",   ["ध्ध"] = "0",   ["न्न"] = "NN",
    ["प्प"] = "P2",  ["फ्फ"] = "F",   ["ब्ब"] = "B",   ["भ्भ"] = "B",   ["म्म"] = "M2",
    ["य्य"] = "Y",   ["र्र"] = "R",   ["ल्ल"] = "L2",  ["व्व"] = "V",
    ["श्श"] = "S1",  ["ष्ष"] = "S1",  ["स्स"] = "S",   ["ह्ह"] = "H",
    -- Nasal clusters
    ["ङ्क"] = "NK",  ["ङ्ख"] = "NK",  ["ङ्ग"] = "NK",  ["ङ्घ"] = "NK",
    ["ञ्च"] = "NC",  ["ञ्छ"] = "NC",  ["ञ्ज"] = "NJ",  ["ञ्झ"] = "NJ",
    ["ण्ट"] = "N1T", ["ण्ठ"] = "N1T", ["ण्ड"] = "N1T", ["ण्ढ"] = "N1T",
    ["न्त"] = "N0",  ["न्थ"] = "N0",  ["न्द"] = "N0",  ["न्ध"] = "N0",
    ["म्प"] = "MP",  ["म्फ"] = "MF",  ["म्ब"] = "MB",  ["म्भ"] = "MB",
    -- Special conjuncts
    ["द्ध"] = "0",   ["द्भ"] = "0B",  ["द्व"] = "0V",  ["द्य"] = "0Y",
    ["न्ह"] = "NH",  ["ल्ह"] = "LH",  ["र्य"] = "RY",
    ["स्थ"] = "S0",  ["स्त"] = "S0",  ["स्न"] = "SN",  ["स्म"] = "SM",
    ["स्व"] = "SV",  ["स्य"] = "SY",
    ["ह्न"] = "HN",  ["ह्म"] = "HM",  ["ह्य"] = "HY",  ["ह्र"] = "HR",  ["ह्ल"] = "HL",
    -- Retroflex + य/व
    ["ट्य"] = "TY",  ["ड्य"] = "TY",  ["ण्य"] = "N1Y",
    -- Dental + य/व
    ["त्य"] = "0Y",  ["द्य"] = "0Y",  ["न्य"] = "NY",
    -- Common Hindi clusters
    ["प्र"] = "PR",  ["ब्र"] = "BR",  ["क्र"] = "KR",  ["ग्र"] = "KR",
    ["द्र"] = "0R",  ["ध्र"] = "0R",  ["श्व"] = "S1V", ["ध्व"] = "0V",
    ["क्य"] = "KY",  ["ख्य"] = "KY",  ["ग्य"] = "KY",  ["व्य"] = "VY",
}

local modifiers = {
    ["ा"] = "",   ["ः"] = "",   ["्"] = "",   ["ृ"] = "R",  ["ॄ"] = "R",
    ["ं"] = "3",  ["ँ"] = "3",  -- Both anusvara and chandrabindu → nasal marker
    ["ि"] = "4",  ["ी"] = "4",
    ["ु"] = "5",  ["ू"] = "5",
    ["े"] = "6",  ["ै"] = "7",
    ["ो"] = "8",  ["ौ"] = "9",
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

-- Strip non-Hindi (Devanagari) characters with a unicode range check.
-- Devanagari Unicode block: U+0900 - U+097F
local function strip_non_hi(s)
    return utils.filter_unicode_range(s, 0x0900, 0x097F)
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
    -- Remove all non-Devanagari characters
    input = strip_non_hi(input)

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

-- Encode Hindi text to three phonetic keys
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
-- (Hinglish) English chars to Indicphone transliteration mappings

-- Placeholders for uppercase retroflex markers (captured before lowercasing)
local RETROFLEX_N = "\1"  -- N → N1 (ण)
local RETROFLEX_L = "\2"  -- L → L1 (ळ)

-- English compound patterns (longest-first).
-- For patterns with same length, order in the list takes priority.
local en_compounds_list = {
    -- 4-char patterns
    {"ksha", "KS1"},  {"kshi", "KS14"}, {"kshu", "KS15"}, {"kshe", "KS16"},
    {"gyan", "KY3"},  {"gnya", "KY"},   {"shra", "S1R"},  {"shri", "S1R4"},
    {"thra", "0R"},   {"dhra", "0R"},

    -- 3-char patterns
    {"ksh", "KS1"},   {"tra", "0R"},    {"tri", "0R4"},   {"gya", "KY"},
    {"shr", "S1R"},   {"thr", "0R"},    {"dhr", "0R"},
    {"kka", "K2"},    {"gga", "K"},     {"nga", "NG"},
    {"cca", "C2"},    {"cha", "C"},     {"jja", "J"},
    {"tta", "T2"},    {"dda", "T"},     {"nna", "NN"},
    {"ppa", "P2"},    {"bba", "B"},     {"mma", "M2"},
    {"yya", "Y"},     {"lla", "L2"},    {"vva", "V"},
    {"ssa", "S"},     {"sha", "S1"},
    {"nda", "N0"},    {"nta", "N0"},    {"nth", "N0"},
    {"mpa", "MP"},    {"mba", "MB"},
    {"nka", "NK"},    {"nch", "NC"},
    {"tth", "0"},     {"ddh", "0"},

    -- 2-char patterns
    {"kk", "K2"},     {"gg", "K"},      {"ng", "NG"},
    {"cc", "C2"},     {"ch", "C"},      {"jj", "J"},
    {"tt", "T2"},     {"dd", "T"},
    {"pp", "P2"},     {"bb", "B"},      {"mm", "M2"},
    {"yy", "Y"},      {"ll", "L2"},     {"vv", "V"},
    {"ss", "S"},      {"sh", "S1"},
    {"th", "0"},      {"dh", "0"},      -- Dental aspirates
    {"nd", "N0"},     {"nt", "N0"},     {"nn", "NN"},
    {"mp", "MP"},     {"mb", "MB"},     {"nk", "NK"},
    {"ph", "F"},      {"bh", "B"},
    {"kh", "K"},      {"gh", "K"},
    {"jh", "J"},      {"zh", "Z"},
    {"tr", "0R"},     {"dr", "0R"},
    {"pr", "PR"},     {"br", "BR"},
    {"kr", "KR"},     {"gr", "KR"},
}

-- Build a lookup table from list
local en_compounds = {}
for _, pair in ipairs(en_compounds_list) do
    en_compounds[pair[1]] = pair[2]
end

-- Single English consonants
local en_consonants = {
    ["k"] = "K",  ["g"] = "K",  ["c"] = "C",  ["j"] = "J",
    ["t"] = "T",  ["d"] = "T",  -- Default to retroflex (common in Hinglish)
    ["n"] = "N",  ["p"] = "P",  ["f"] = "F",  ["b"] = "B",  ["m"] = "M",
    ["y"] = "Y",  ["r"] = "R",  ["l"] = "L",  ["v"] = "V",
    ["s"] = "S",  ["h"] = "H",  ["z"] = "Z",
    -- Retroflex placeholders (from uppercase N/L)
    [RETROFLEX_N] = "N1",  [RETROFLEX_L] = "L1",
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

    -- 2. Now lowercase the rest
    input = string.lower(input)

    -- 3. Normalize common variations
    input = string_gsub(input, "w", "v")
    input = string_gsub(input, "x", "ks")
    input = string_gsub(input, "q", "k")

    -- 4. Remove non-alphabetic characters (but keep our placeholders)
    input = string_gsub(input, "[^a-z\1\2]", "")

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

    -- Handle word-final anusvara: final M after consonant becomes nasal marker (3)
    -- This handles common patterns like "hindustanam" → हिंदुस्तानं
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

-- Check if string contains Hindi (Devanagari) characters (U+0900-U+097F range)
local function has_native(text)
    return string_find(text, "[\xE0\xA4\x80-\xE0\xA5\xBF]") ~= nil
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
-- Auto-detects Hindi vs Hinglish input
-- Returns keys joined with " OR " for SQLite FTS5 OR matching
function to_query(text, lang)
    local key0, key1, key2

    -- If no Hindi chars, use English transliteration
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
