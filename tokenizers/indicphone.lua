-- indicphone.lua is a universal phonetic hashing for Indic scripts (Brahmi-derived).
-- It produces a 3-level hash for each word, with increasing phonetic proximity.
-- The first level (key0) is the most coarse, keeping only consonant classes and ignoring vowels.
-- The second level (key1) adds vowel distinctions and some compound consonants.
-- The third level (key2) is the most specific, with separate codes for all consonants and vowels, including chillus in Malayalam.

local config = { num_keys = 2 }

local table_concat = table.concat
local string_sub, string_gsub, string_lower = string.sub, string.gsub, string.lower
local math_min, math_floor = math.min, math.floor
local utf8 = utf8 or require("utf8")

local CONSONANT, CHILLU, VOWEL, MATRA, VIRAMA_T, ANUSVARA_T, VISARGA_T, OTHER = 1,2,3,4,5,6,7,8

-- All Brahmi-derived Unicode blocks are contiguous (0x0900–0x0D7F), each 0x80 wide.
-- Index by (cp - 0x0900) / 0x80 for easy lookup.
local BLOCKS = {
    {0x0900, 0x097F, "devanagari"}, {0x0980, 0x09FF, "bengali"},
    {0x0A00, 0x0A7F, "gurmukhi"},   {0x0A80, 0x0AFF, "gujarati"},
    {0x0B00, 0x0B7F, "odia"},       {0x0B80, 0x0BFF, "tamil"},
    {0x0C00, 0x0C7F, "telugu"},     {0x0C80, 0x0CFF, "kannada"},
    {0x0D00, 0x0D7F, "malayalam"},
}
local INDIC_LO, INDIC_HI, BLOCK_WIDTH = 0x0900, 0x0D7F, 0x80

local CONSONANTS = {
    [0x15]="K",  [0x16]="K",  [0x17]="K",  [0x18]="K",  [0x19]="NG",
    [0x1A]="C",  [0x1B]="C",  [0x1C]="J",  [0x1D]="J",  [0x1E]="NJ",
    [0x1F]="T",  [0x20]="T",  [0x21]="T",  [0x22]="T",  [0x23]="N1",
    [0x24]="0",  [0x25]="0",  [0x26]="0",  [0x27]="0",  [0x28]="N",
    [0x2A]="P",  [0x2B]="F",  [0x2C]="B",  [0x2D]="B",  [0x2E]="M",
    [0x2F]="Y",  [0x30]="R",  [0x31]="R1", [0x32]="L",  [0x33]="L1",
    [0x34]="Z",  [0x35]="V",
    [0x36]="S1", [0x37]="S1", [0x38]="S",  [0x39]="H",
}

local VOWELS = {
    [0x05]="A", [0x06]="A", [0x07]="I", [0x08]="I",
    [0x09]="U", [0x0A]="U", [0x0B]="R",
    [0x0E]="E", [0x0F]="E", [0x10]="AI",
    [0x12]="O", [0x13]="O", [0x14]="O", [0x60]="R",
}

local MATRAS = {
    [0x3E]="",  [0x3F]="4", [0x40]="4",
    [0x41]="5", [0x42]="5", [0x43]="R", [0x44]="R",
    [0x46]="6", [0x47]="6", [0x48]="7",
    [0x4A]="8", [0x4B]="8", [0x4C]="9",
}

local VIRAMA      = 0x4D
local ANUSVARA    = 0x02
local CHANDRABINDU = 0x01
local VISARGA     = 0x03

local ML_CHILLUS = {
    [0x0D7D]="L", [0x0D7E]="L1", [0x0D7A]="N1",
    [0x0D7B]="N", [0x0D7C]="R1", [0x0D7F]="K",
}
local DEV_NUKTA = {
    [0x0958]="K", [0x0959]="K", [0x095A]="K",
    [0x095B]="Z", [0x095C]="R1", [0x095D]="R1", [0x095E]="F",
}
local ML_AU = { [0x0D57]="9" }

-- Conjuncts where virama-based parsing gives wrong result.
-- Keys: off1*0x100+off2 (both < 0x80, so collision-free).
local COMPOUND_EXCEPTIONS = {
    malayalam  = {[0x26*0x100+0x26]="D", [0x26*0x100+0x27]="D", [0x31*0x100+0x31]="T"},
    devanagari = {[0x1C*0x100+0x1E]="GY"},
    kannada    = {[0x26*0x100+0x26]="D", [0x26*0x100+0x27]="D"},
}

local RETROFLEX_N, RETROFLEX_L, RETROFLEX_R = "\1", "\2", "\3"
local RETROFLEX_MAP = {N=RETROFLEX_N, L=RETROFLEX_L, R=RETROFLEX_R}
local NORMALIZE_MAP = {w="v", x="ks", q="k"}

local en_compounds = (function()
    local list = {
        {"ntha","N0"},  {"njch","NC"},  {"ksha","KS1"}, {"nkha","NK"},
        {"zhla","L1"},  {"zhna","N1"},  {"zhra","R1"},
        {"kka","K2"},   {"gga","K"},    {"cca","C2"},   {"cha","C"},
        {"tta","T2"},   {"dda","T2"},   {"ppa","P2"},   {"mma","M2"},
        {"lla","L2"},   {"nna","NN"},   {"rra","T"},    {"yya","Y"},
        {"vva","V"},    {"ssa","S"},
        {"sha","S1"},   {"zha","Z"},    {"nga","NG"},   {"nja","NJ"},
        {"nda","N1T"},  {"nta","NT"},   {"mpa","MP"},   {"nka","NK"},
        {"tth","0"},    {"nth","N0"},   {"nch","NC"},   {"ksh","KS1"},
        {"zhl","L1"},   {"zhn","N1"},   {"zhr","R1"},
        {"zl","L1"},    {"zn","N1"},    {"zr","R1"},
        {"kk","K2"},    {"gg","K"},     {"cc","C2"},    {"ch","C"},
        {"tt","T2"},    {"dd","T2"},    {"pp","P2"},    {"mm","M2"},
        {"ll","L2"},    {"nn","NN"},    {"rr","T"},     {"yy","Y"},
        {"vv","V"},     {"ss","S"},
        {"sh","S1"},    {"zh","Z"},     {"ng","NG"},    {"nj","NJ"},
        {"th","0"},     {"dh","0"},     {"nd","N1T"},   {"nt","NT"},
        {"mp","MP"},    {"nk","NK"},    {"ph","F"},     {"bh","B"},
        {"kh","K"},     {"gh","K"},     {"jh","J"},
    }
    local map = {}
    for _, pair in ipairs(list) do
        if not map[pair[1]] then map[pair[1]] = pair[2] end
    end
    return map
end)()

local en_consonants = {
    k="K", g="K", c="C", j="J", t="T", d="T", n="N", p="P", f="F",
    b="B", m="M", y="Y", r="R", l="L", v="V", s="S", h="H", z="Z",
    [RETROFLEX_N]="N1", [RETROFLEX_L]="L1", [RETROFLEX_R]="R1",
}

local en_vowels_standalone = {
    aa="A", ai="AI", au="O", ou="O",
    ee="I", ii="I", oo="U", uu="U",
    ea="I", ie="I", oa="O",
    a="A", i="I", u="U", e="E", o="O",
}

local en_vowels_modifier = {
    aa="", ai="7", au="9", ou="9",
    ee="4", ii="4", oo="5", uu="5",
    ea="4", ie="4", oa="8",
    a="", i="4", u="5", e="6", o="8",
}

local function detect_script(text)
    local ok, iter, state, start = pcall(utf8.codes, text)
    if not ok then return nil, nil end

    for _, cp in iter, state, start do
        if cp >= INDIC_LO and cp <= INDIC_HI then
            local blk = BLOCKS[math_floor((cp - INDIC_LO) / BLOCK_WIDTH) + 1]
            if blk and cp >= blk[1] and cp <= blk[2] then
                return blk[3], blk[1]
            end
        end

        -- Malayalam chillus sit outside the main block. Handle them as a special case.
        if cp >= 0x0D7A and cp <= 0x0D7F then
            return "malayalam", 0x0D00
        end
    end
    return nil, nil
end

local function classify(cp, base, script)
    if script == "malayalam" then
        if ML_CHILLUS[cp] then return CHILLU, ML_CHILLUS[cp], nil end
        if ML_AU[cp]       then return MATRA, ML_AU[cp], nil end
    end
    if script == "devanagari" and DEV_NUKTA[cp] then
        return CONSONANT, DEV_NUKTA[cp], nil
    end

    if cp < base or cp > base + 0x7F then
        return OTHER, nil, nil
    end

    local o = cp - base
    if CONSONANTS[o] then return CONSONANT, CONSONANTS[o], o end
    if VOWELS[o]     then return VOWEL,     VOWELS[o],     o end
    if MATRAS[o]     then return MATRA,     MATRAS[o],     o end
    if o == VIRAMA   then return VIRAMA_T,  nil,           o end
    if o == ANUSVARA or o == CHANDRABINDU then return ANUSVARA_T, "3", o end
    if o == VISARGA  then return VISARGA_T, "",            o end
    return OTHER, nil, nil
end

-- 3-state left-to-right pass
-- START(0)=idle, NEXT_C(1)=holding consonant, IN_CONJ(2)=virama seen.
local START, NEXT_C, IN_CONJ = 0, 1, 2

local function hash_native(text, script, base)
    local exc = COMPOUND_EXCEPTIONS[script] or {}
    local out = {}
    local st = START
    local prev_code, prev_off = nil, nil

    local ok, iter, state, s = pcall(utf8.codes, text)
    if not ok then return "" end

    for _, cp in iter, state, s do
        local ctype, code, off = classify(cp, base, script)

        if ctype == CONSONANT or ctype == CHILLU then
            if st == IN_CONJ then
                -- Check compound exception.
                if prev_off and off then
                    local ek = prev_off * 0x100 + off
                    if exc[ek] then
                        out[#out+1] = exc[ek]
                        st, prev_code, prev_off = START, nil, nil
                        goto continue
                    end
                end

                -- Geminate: same consonant on both sides of virama.
                if code == prev_code then
                    out[#out+1] = prev_code .. "2"
                    st, prev_code, prev_off = START, nil, nil
                    goto continue
                end
                out[#out+1] = prev_code
            elseif st == NEXT_C and prev_code then
                out[#out+1] = prev_code
            end

            if ctype == CHILLU then
                out[#out+1] = code
                st, prev_code, prev_off = START, nil, nil
            else
                st, prev_code, prev_off = NEXT_C, code, off
            end

        elseif ctype == VIRAMA_T then
            st = IN_CONJ

        elseif ctype == MATRA then
            if (st == NEXT_C or st == IN_CONJ) and prev_code then
                out[#out+1] = prev_code
            end
            if code and code ~= "" then
                out[#out+1] = code
            end
            st, prev_code, prev_off = START, nil, nil

        elseif ctype == VOWEL then
            if (st == NEXT_C or st == IN_CONJ) and prev_code then
                out[#out+1] = prev_code
            end
            out[#out+1] = code
            st, prev_code, prev_off = START, nil, nil

        elseif ctype == ANUSVARA_T then
            if (st == NEXT_C or st == IN_CONJ) and prev_code then
                out[#out+1] = prev_code
            end
            out[#out+1] = code
            st, prev_code, prev_off = START, nil, nil

        elseif ctype == VISARGA_T then
            if st == NEXT_C and prev_code then
                out[#out+1] = prev_code
            end
            st, prev_code, prev_off = START, nil, nil

        else
            if (st == NEXT_C or st == IN_CONJ) and prev_code then
                out[#out+1] = prev_code
            end
            st, prev_code, prev_off = START, nil, nil
        end

        ::continue::
    end

    -- Flush pending consonant.
    if (st == NEXT_C or st == IN_CONJ) and prev_code then
        out[#out+1] = prev_code
    end

    return table_concat(out)
end

local COMPOUND_LENS = {4, 3, 2}
local VOWEL_LENS = {2, 1}

local function hash_romanized(text)
    -- Mark uppercase retroflex N/L/R before lowercasing.
    local t = string_gsub(text, "[NLR]", RETROFLEX_MAP)
    t = string_lower(t)
    t = string_gsub(t, "[wxq]", NORMALIZE_MAP)
    t = string_gsub(t, "[^a-z\1\2\3]", "")

    local out = {}
    local pos = 1
    local len = #t
    local after_c = false

    while pos <= len do
        local matched = false

        for _, plen in ipairs(COMPOUND_LENS) do
            if pos + plen - 1 <= len then
                local sub = string_sub(t, pos, pos + plen - 1)
                if en_compounds[sub] then
                    out[#out+1] = en_compounds[sub]
                    pos = pos + plen
                    after_c = true
                    matched = true
                    break
                end
            end
        end

        if not matched then
            for _, vlen in ipairs(VOWEL_LENS) do
                if pos + vlen - 1 <= len then
                    local sub = string_sub(t, pos, pos + vlen - 1)
                    if after_c and en_vowels_modifier[sub] then
                        local mod = en_vowels_modifier[sub]
                        if mod ~= "" then out[#out+1] = mod end
                        pos = pos + vlen
                        after_c = false
                        matched = true
                        break
                    elseif not after_c and en_vowels_standalone[sub] then
                        out[#out+1] = en_vowels_standalone[sub]
                        pos = pos + vlen
                        after_c = false
                        matched = true
                        break
                    end
                end
            end
        end

        if not matched then
            local ch = string_sub(t, pos, pos)
            if en_consonants[ch] then
                out[#out+1] = en_consonants[ch]
                after_c = true
            end
            pos = pos + 1
        end
    end

    local result = table_concat(out)
    return (string_gsub(result, "([^AEIOU])M$", "%13"))
end

local function encode(text)
    text = utils.trim(text)
    if text == "" then return "", "", "" end

    local script, base = detect_script(text)
    local key2
    if script then
        key2 = hash_native(text, script, base)
    else
        key2 = hash_romanized(text)
    end

    local key1 = string_gsub(key2, "[24-9]", "")
    local key0 = string_gsub(key2, "[1-24-9]", "")
    return key0, key1, key2
end

function tokenize(text, lang)
    local tokens = {}
    for word in utils.words(text) do
        local key0, key1, key2 = encode(word)
        if key0 ~= "" then
            tokens[#tokens+1] = key0 .. ":3"
            if key1 ~= key0 then
                tokens[#tokens+1] = key1 .. ":2"
            end
            if key2 ~= key1 and key2 ~= key0 then
                tokens[#tokens+1] = key2 .. ":1"
            end
        end
    end
    return tokens
end

function to_query(text, lang)
    local key0, key1, key2 = encode(text)
    if key0 == "" then return "" end

    -- Collect unique keys (most specific first).
    local keys = {key2}
    if key1 ~= key2 then keys[#keys+1] = key1 end
    if key0 ~= key1 and key0 ~= key2 then keys[#keys+1] = key0 end

    if #keys > config.num_keys then
        for i = #keys, config.num_keys + 1, -1 do keys[i] = nil end
    end
    return table_concat(keys, " OR ")
end
