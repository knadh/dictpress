-- indicphone.lua is a universal phonetic hashing for Indic scripts (Brahmi-derived).
-- It produces a 3-level hash for each word, with increasing phonetic proximity.
-- The first level (key0) is the most coarse, keeping only consonant classes and ignoring vowels.
-- The second level (key1) adds vowel distinctions and some compound consonants.
-- The third level (key2) is the most specific, with separate codes for all consonants and vowels, including chillus in Malayalam.

local config = { num_keys = 2 }

local table_concat = table.concat
local string_sub, string_gsub, string_lower = string.sub, string.gsub, string.lower
local math_floor = math.floor
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
local SCRIPT_BASES = {}
for _, b in ipairs(BLOCKS) do SCRIPT_BASES[b[3]] = b[1] end

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

local RETROFLEX_N, RETROFLEX_L, RETROFLEX_R, RETROFLEX_T, RETROFLEX_D = "\1", "\2", "\3", "\4", "\5"
local RETROFLEX_MAP = {N=RETROFLEX_N, L=RETROFLEX_L, R=RETROFLEX_R, T=RETROFLEX_T, D=RETROFLEX_D}
local NORMALIZE_MAP = {w="v", x="ks", q="k"}

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

local MAX_TRANSLITERATION_VARIANTS = 32

local function chr(base, off)
    return utf8.char(base + off)
end

local function expand_variants(vars, choices)
    local out, seen = {}, {}
    for _, prefix in ipairs(vars) do
        for _, choice in ipairs(choices) do
            local v = prefix .. choice
            if not seen[v] then
                out[#out+1], seen[v] = v, true
                if #out >= MAX_TRANSLITERATION_VARIANTS then return out end
            end
        end
    end
    return out
end

local function transliterate_roman(text, script)
    local base = SCRIPT_BASES[script]
    if not base or detect_script(text) then return {} end

    local virama, anusvara = chr(base, VIRAMA), chr(base, ANUSVARA)
    local function c(off) return chr(base, off) end
    local function raw(...)
        local out, n = {}, select("#", ...)
        for i = 1, n do out[i] = c(select(i, ...)) end
        return table_concat(out)
    end
    local function j(...)
        local out, n = {}, select("#", ...)
        for i = 1, n do
            if i > 1 then out[#out+1] = virama end
            out[#out+1] = c(select(i, ...))
        end
        return table_concat(out)
    end
    local function jm(matra, ...)
        return j(...) .. c(matra)
    end
    local function rj(off)
        if script == "malayalam" then return utf8.char(0x0D7C) .. c(off) end
        return j(0x30, off)
    end
    local zh = script == "malayalam" and 0x34 or 0x33
    local au_matra = script == "malayalam" and c(0x57) or c(0x4C)

    local consonants = {
        k={c(0x15)}, kh={c(0x16)}, g={c(0x17)}, gh={c(0x18)},
        ch={c(0x1A)}, c={c(0x1A)}, j={c(0x1C)}, jh={c(0x1D)},
        t={c(0x24), c(0x1F)}, d={c(0x26), c(0x21)},
        [RETROFLEX_T]={c(0x1F)}, [RETROFLEX_D]={c(0x21)},
        [RETROFLEX_N]={c(0x23)}, [RETROFLEX_L]={c(0x33)}, [RETROFLEX_R]={c(0x31)},
        th={c(0x24), c(0x25)}, dh={c(0x26), c(0x27)}, p={c(0x2A)},
        ph={c(0x2B), c(0x2A)}, f={c(0x2B)}, b={c(0x2C)}, bh={c(0x2D)}, m={c(0x2E)},
        y={c(0x2F)}, r={c(0x30)}, l={c(0x32)}, v={c(0x35)}, w={c(0x35)},
        s={c(0x38)}, sh={c(0x36)}, h={c(0x39)}, n={c(0x28), c(0x23)},
        ng={c(0x19)}, nj={c(0x1E)}, z={script == "malayalam" and c(0x34) or c(0x1C)},
        zh={c(zh)},
    }
    local clusters = {
        kshm={j(0x15,0x37,0x2E)},
        ksh={j(0x15,0x37)}, ks={j(0x15,0x37)},
        rish={raw(0x0B,0x37)}, rush={raw(0x0B,0x37)},
        gy={j(0x17,0x2F), j(0x1C,0x1E)}, jn={j(0x1C,0x1E)},
        jny={j(0x1C,0x1E)}, gn={j(0x1C,0x1E)},
        rr={c(0x31)}, zhch={j(zh,0x1A)},
        kk={j(0x15,0x15)}, gg={j(0x17,0x17)}, cc={j(0x1A,0x1A)}, chch={j(0x1A,0x1A)},
        jj={j(0x1C,0x1C)}, tt={j(0x24,0x24), j(0x1F,0x1F)}, dd={j(0x26,0x26), j(0x21,0x21)},
        pp={j(0x2A,0x2A)}, bb={j(0x2C,0x2C)}, mm={j(0x2E,0x2E)},
        ll={j(0x32,0x32), j(0x33,0x33)}, yy={j(0x2F,0x2F)}, vv={j(0x35,0x35)}, ss={j(0x38,0x38)},
        sri={raw(0x38,0x43), jm(0x40,0x36,0x30)}, sru={raw(0x38,0x43)},
        shri={jm(0x40,0x36,0x30), raw(0x38,0x43)}, shru={jm(0x43,0x36,0x30)},
        str={j(0x38,0x24,0x30), j(0x37,0x1F,0x30)},
        st={j(0x38,0x24), j(0x37,0x1F)}, sht={j(0x37,0x1F), j(0x37,0x20)}, shth={j(0x37,0x20)},
        sth={j(0x37,0x1F), j(0x38,0x25)},
        nsth={anusvara..j(0x38,0x25), anusvara..j(0x37,0x1F)},
        msth={anusvara..j(0x38,0x25), anusvara..j(0x37,0x1F)},
        nsk={anusvara..j(0x38,0x15)}, msk={anusvara..j(0x38,0x15)},
        nst={anusvara..j(0x38,0x24), anusvara..j(0x37,0x1F)},
        mst={anusvara..j(0x38,0x24), anusvara..j(0x37,0x1F)},
        ns={anusvara..c(0x38), j(0x28,0x38)}, ms={anusvara..c(0x38), j(0x2E,0x38)},
        shn={j(0x37,0x23)}, shm={j(0x36,0x2E), j(0x37,0x2E)},
        sk={j(0x38,0x15)}, shk={j(0x36,0x15)}, sp={j(0x38,0x2A)}, sph={j(0x38,0x2B)},
        sm={j(0x38,0x2E)}, sn={j(0x38,0x28)}, sl={j(0x38,0x32)}, sv={j(0x38,0x35)}, sw={j(0x38,0x35)},
        mb={anusvara..c(0x2C), j(0x2E,0x2C), j(0x2E,0x2A)}, mbh={anusvara..c(0x2D), j(0x2E,0x2D)},
        mp={anusvara..c(0x2A), j(0x2E,0x2A)}, mph={anusvara..c(0x2B), j(0x2E,0x2B)},
        nk={anusvara..c(0x15), j(0x19,0x15)}, nkh={anusvara..c(0x16), j(0x19,0x16)},
        ng={anusvara..c(0x17), j(0x19,0x17), c(0x19)}, ngh={anusvara..c(0x18), j(0x19,0x18)},
        nch={j(0x1E,0x1A), anusvara..c(0x1A)}, nj={j(0x1E,0x1C), anusvara..c(0x1C)},
        njh={j(0x1E,0x1D), anusvara..c(0x1D)},
        nn={j(0x28,0x28), j(0x23,0x23)}, nna={j(0x28,0x28), j(0x23,0x23)},
        ["n"..RETROFLEX_T]={j(0x23,0x1F)}, ["n"..RETROFLEX_D]={j(0x23,0x21)},
        nth={j(0x28,0x25), j(0x28,0x24), anusvara..c(0x25)},
        ndh={j(0x28,0x27), j(0x28,0x26), anusvara..c(0x27)},
        nt={j(0x28,0x24), j(0x23,0x1F), anusvara..c(0x24)},
        nd={j(0x28,0x26), j(0x23,0x21), anusvara..c(0x26)},
        ny={j(0x28,0x2F), j(0x23,0x2F)},
        ty={j(0x24,0x2F)}, thy={j(0x24,0x2F)}, dy={j(0x26,0x2F)}, dhy={j(0x26,0x2F)},
        ky={j(0x15,0x2F)}, khy={j(0x16,0x2F)}, py={j(0x2A,0x2F)}, by={j(0x2C,0x2F)},
        bhy={j(0x2D,0x2F)}, my={j(0x2E,0x2F)}, vy={j(0x35,0x2F)},
        ly={j(0x32,0x2F)}, lhy={j(0x32,0x39,0x2F)}, ry={j(0x30,0x2F)},
        kr={j(0x15,0x30)}, khr={j(0x16,0x30)}, gr={j(0x17,0x30)}, ghr={j(0x18,0x30)},
        cr={j(0x1A,0x30)}, chr={j(0x1A,0x30)}, jr={j(0x1C,0x30)}, tr={j(0x24,0x30)},
        thr={j(0x24,0x30), j(0x25,0x30)}, dr={j(0x26,0x30)}, dhr={j(0x26,0x30), j(0x27,0x30)},
        pr={j(0x2A,0x30)}, phr={j(0x2B,0x30), j(0x2A,0x30)}, br={j(0x2C,0x30)}, bhr={j(0x2D,0x30)},
        mr={j(0x2E,0x30)}, nr={j(0x28,0x30)}, kv={j(0x15,0x35)}, gv={j(0x17,0x35)},
        tv={j(0x24,0x35)}, tw={j(0x24,0x35)}, dv={j(0x26,0x35)}, dw={j(0x26,0x35)},
        vr={j(0x35,0x30)}, wr={j(0x35,0x30)},
        rsh={rj(0x37), rj(0x36)}, rth={rj(0x25)}, rdh={rj(0x27)}, rbh={rj(0x2D)},
        rk={rj(0x15)}, rg={rj(0x17)}, rt={rj(0x24)}, rd={rj(0x26)},
        rp={rj(0x2A)}, rb={rj(0x2C)}, rm={rj(0x2E)}, rn={rj(0x23), rj(0x28)}, rv={rj(0x35)},
        sr={j(0x38,0x30), j(0x36,0x30)}, shr={j(0x36,0x30)}, hr={j(0x39,0x30)},
        hm={j(0x39,0x2E)}, hn={j(0x39,0x28)}, hy={j(0x39,0x2F)}, hl={j(0x39,0x32)},
    }
    local function add_cluster(key, vals)
        clusters[key] = clusters[key] or {}
        for _, v in ipairs(vals) do
            local exists = false
            for _, cur in ipairs(clusters[key]) do
                if cur == v then exists = true; break end
            end
            if not exists then clusters[key][#clusters[key]+1] = v end
        end
    end
    for roman, offs in pairs({
        k={0x15}, kh={0x16}, g={0x17}, gh={0x18}, ch={0x1A}, c={0x1A}, j={0x1C}, jh={0x1D},
        t={0x24,0x1F}, th={0x24,0x25}, d={0x26,0x21}, dh={0x26,0x27}, n={0x28,0x23},
        p={0x2A}, ph={0x2B,0x2A}, b={0x2C}, bh={0x2D}, m={0x2E}, y={0x2F},
        r={0x30}, l={0x32}, v={0x35}, w={0x35}, s={0x38}, sh={0x36}, h={0x39},
    }) do
        local riy, ri, ru = {}, {}, {}
        for _, off in ipairs(offs) do
            ri[#ri+1] = c(off)..c(0x43)
            ri[#ri+1] = j(off,0x30)..c(0x3F)
            ru[#ru+1] = c(off)..c(0x43)
            ru[#ru+1] = j(off,0x30)..c(0x41)
            riy[#riy+1] = j(off,0x30)..c(0x3F)..c(0x2F)
        end
        add_cluster(roman.."riy", riy)
        add_cluster(roman.."ri", ri)
        add_cluster(roman.."ru", ru)
    end
    local vowels = {
        aa={c(0x06), c(0x3E)}, a={c(0x05), ""},
        ee={c(0x08), c(0x40)}, ii={c(0x08), c(0x40)}, i={c(0x07), c(0x3F), c(0x08), c(0x40)},
        u={c(0x09), c(0x41), c(0x0A), c(0x42)}, oo={c(0x0A), c(0x42)}, uu={c(0x0A), c(0x42)},
        ri={c(0x0B), c(0x43)}, ru={c(0x0B), c(0x43)},
        e={c(0x0F), c(0x47), c(0x0E), c(0x46)}, ai={c(0x10), c(0x48)},
        o={c(0x13), c(0x4B), c(0x12), c(0x4A)}, au={c(0x14), au_matra}, ou={c(0x14), au_matra},
    }
    local lens = {4, 3, 2, 1}
    local t = string_gsub(text, "[NLRTD]", RETROFLEX_MAP)
    t = string_lower(t)
    t = string_gsub(t, "ow", "au")
    t = string_gsub(t, "[wxq]", NORMALIZE_MAP)
    t = string_gsub(t, "[^a-z\1\2\3\4\5]", "")
    if t == "" then return {} end

    local variants, after_c, pos = {""}, false, 1
    while pos <= #t do
        local matched = false
        for _, len in ipairs(lens) do
            if pos + len - 1 <= #t then
                local sub = string_sub(t, pos, pos + len - 1)
                if clusters[sub] then
                    variants = expand_variants(variants, clusters[sub])
                    after_c, pos, matched = true, pos + len, true
                    break
                end
                if consonants[sub] then
                    if sub == "m" and pos + len - 1 == #t then
                        variants = expand_variants(variants, {anusvara, consonants[sub][1]})
                    else
                        variants = expand_variants(variants, consonants[sub])
                    end
                    after_c, pos, matched = true, pos + len, true
                    break
                end
                if vowels[sub] and (sub ~= "ri" and sub ~= "ru" or after_c or pos == 1) then
                    local choices = {}
                    for i = after_c and 2 or 1, #vowels[sub], 2 do
                        choices[#choices+1] = vowels[sub][i]
                    end
                    variants = expand_variants(variants, choices)
                    after_c, pos, matched = false, pos + len, true
                    break
                end
            end
        end

        if not matched then pos = pos + 1 end
    end

    return variants
end

local function script_for(lang, cfg)
    cfg = cfg or {}
    return cfg.transliterate_script or (SCRIPT_BASES[lang] and lang) or ""
end

local function encode(text, script)
    text = utils.trim(text)
    if text == "" then return "", "", "" end

    local detected, base = detect_script(text)
    local key2
    if detected then
        key2 = hash_native(text, detected, base)
    else
        local native = transliterate_roman(text, script)[1]
        if not native then return "", "", "", true end
        key2 = hash_native(native, script, SCRIPT_BASES[script])
    end

    local key1 = string_gsub(key2, "[24-9]", "")
    local key0 = string_gsub(key2, "[1-24-9]", "")
    return key0, key1, key2, detected == nil
end

local function add_query_keys(out, seen, key2)
    local key1 = string_gsub(key2, "[24-9]", "")
    local key0 = string_gsub(key2, "[1-24-9]", "")
    local n = 0
    for _, key in ipairs({key2, key1, key0}) do
        if key ~= "" and not seen[key] then
            out[#out+1], seen[key] = key, true
            n = n + 1
            if n >= config.num_keys then return end
        end
    end
end

function tokenize(text, lang)
    local tokens = {}
    for word in utils.words(text) do
        local key0, key1, key2 = encode(word, lang)
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

function to_query(text, lang, cfg)
    text = utils.trim(text)
    if text == "" then return {raw_text=text, fts_query=""} end

    local keys, seen = {}, {}
    local raw_text = text
    local detected, base = detect_script(text)
    if detected then
        add_query_keys(keys, seen, hash_native(text, detected, base))
    else
        local script = script_for(lang, cfg)
        local base = SCRIPT_BASES[script]
        if not base then return {raw_text=raw_text, fts_query=""} end
        local variants = transliterate_roman(text, script)
        raw_text = variants[1] or raw_text
        for _, native in ipairs(variants) do
            add_query_keys(keys, seen, hash_native(native, script, base))
        end
    end

    return {raw_text=raw_text, fts_query=table_concat(keys, " OR ")}
end

function transliterate(text, lang, cfg)
    return transliterate_roman(text, script_for(lang, cfg))
end
