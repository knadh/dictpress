DROP TABLE IF EXISTS entries CASCADE;
CREATE TABLE entries (
    id              SERIAL PRIMARY KEY,

    -- A custom, unique GUID for every entry, like an MD5 hash
    guid            TEXT,

    -- Actual language content. Dictionary word or definition entries
    content         TEXT NOT NULL,

    -- The first “alphabet” of the content. For English, for the word Apple, the initial is A
    initial         TEXT NOT NULL DEFAULT '',

    -- An optional numeric value to order search results
    weight          INT NOT NULL DEFAULT 0,

    -- Fulltext search tokens. For English, Postgres’ built-in tokenizer gives to_tsvector('fully conditioned') = 'condit':2 'fulli':1
    tokens          TSVECTOR NOT NULL DEFAULT '',

    -- String representing the language of content. Eg: en, english
    lang            TEXT NOT NULL,

    -- Strings describing the types of content. Eg {noun, propernoun}
    types           TEXT[] NOT NULL DEFAULT '{}'::TEXT[],

    -- Optional tags
    tags            TEXT[] NOT NULL DEFAULT '{}'::TEXT[],

    -- Phonetic (pronunciation) descriptions of the content. Eg: {ap(ə)l, aapl} for Apple
    phones          TEXT[] NOT NULL DEFAULT '{}'::TEXT[],

    -- Optional text notes
    notes           TEXT NOT NULL DEFAULT '',

    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
DROP INDEX IF EXISTS idx_entries_guid; CREATE UNIQUE INDEX idx_entries_guid ON entries(guid);
DROP INDEX IF EXISTS idx_content; CREATE INDEX idx_entries_content ON entries((LOWER(SUBSTRING(content, 0, 50))));
DROP INDEX IF EXISTS idx_entries_initial; CREATE INDEX idx_entries_initial ON entries(initial);
DROP INDEX IF EXISTS idx_entries_lang; CREATE INDEX idx_entries_lang ON entries(lang);
DROP INDEX IF EXISTS idx_entries_tokens; CREATE INDEX idx_entries_tokens ON entries USING GIN(tokens);
DROP INDEX IF EXISTS idx_entries_types; CREATE INDEX idx_entries_types ON entries(types);
DROP INDEX IF EXISTS idx_entries_tags; CREATE INDEX idx_entries_tags ON entries(tags);

DROP TABLE IF EXISTS relations CASCADE;
CREATE TABLE relations (
    from_id         INTEGER REFERENCES entries(id) ON DELETE CASCADE ON UPDATE CASCADE,	
    to_id           INTEGER REFERENCES entries(id) ON DELETE CASCADE ON UPDATE CASCADE,

    types           TEXT[] NOT NULL DEFAULT '{}',
    tags            TEXT[] NOT NULL DEFAULT '{}',
    notes           TEXT NOT NULL DEFAULT '',

    -- Optional weight for ordering definitions.
    weight          INT DEFAULT 0,

    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    PRIMARY KEY(from_id, to_id)
);
