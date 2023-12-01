CREATE EXTENSION IF NOT EXISTS pgcrypto;

DROP TYPE IF EXISTS entry_status CASCADE; CREATE TYPE entry_status AS ENUM ('pending', 'enabled', 'disabled');

-- entries
DROP TABLE IF EXISTS entries CASCADE;
CREATE TABLE entries (
    -- Internal unique ID.
    id              SERIAL PRIMARY KEY,

    -- Publicly visible unique ID (used in public APIs such as submissions and corrections).
    guid            UUID NOT NULL UNIQUE DEFAULT GEN_RANDOM_UUID(),

    -- Actual language content. Dictionary word or definition entries
    content         TEXT NOT NULL CHECK (content <> ''),

    -- The first “alphabet” of the content. For English, for the word Apple, the initial is A
    initial         TEXT NOT NULL DEFAULT '',

    -- An optional numeric value to order search results
    weight          DECIMAL NOT NULL DEFAULT 0,

    -- Fulltext search tokens. For English, Postgres’ built-in tokenizer gives to_tsvector('fully conditioned') = 'condit':2 'fulli':1
    tokens          TSVECTOR NOT NULL DEFAULT '',

    -- String representing the language of content. Eg: en, english
    lang            TEXT NOT NULL CHECK (lang <> ''),

    -- Optional tags
    tags            TEXT[] NOT NULL DEFAULT '{}'::TEXT[],

    -- Phonetic (pronunciation) descriptions of the content. Eg: {ap(ə)l, aapl} for Apple
    phones          TEXT[] NOT NULL DEFAULT '{}'::TEXT[],

    -- Optional text notes
    notes           TEXT NOT NULL DEFAULT '',

    -- Optional arbitrary metadata
    meta            JSONB NOT NULL DEFAULT '{}',

    status          entry_status NOT NULL DEFAULT 'enabled',
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
DROP INDEX IF EXISTS idx_content; CREATE INDEX idx_entries_content ON entries((LOWER(SUBSTRING(content, 0, 50))));
DROP INDEX IF EXISTS idx_entries_initial; CREATE INDEX idx_entries_initial ON entries(initial);
DROP INDEX IF EXISTS idx_entries_lang; CREATE INDEX idx_entries_lang ON entries(lang);
DROP INDEX IF EXISTS idx_entries_tokens; CREATE INDEX idx_entries_tokens ON entries USING GIN(tokens);
DROP INDEX IF EXISTS idx_entries_tags; CREATE INDEX idx_entries_tags ON entries(tags);

-- relations
DROP TABLE IF EXISTS relations CASCADE;
CREATE TABLE relations (
    id              SERIAL PRIMARY KEY,
    from_id         INTEGER REFERENCES entries(id) ON DELETE CASCADE ON UPDATE CASCADE,	
    to_id           INTEGER REFERENCES entries(id) ON DELETE CASCADE ON UPDATE CASCADE,

    types           TEXT[] NOT NULL DEFAULT '{}',
    tags            TEXT[] NOT NULL DEFAULT '{}',
    notes           TEXT NOT NULL DEFAULT '',
    weight          DECIMAL DEFAULT 0,

    status          entry_status NOT NULL DEFAULT 'enabled',
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
DROP INDEX IF EXISTS idx_relations; CREATE UNIQUE INDEX idx_relations ON relations(from_id, to_id);

-- comments
-- This table holds change suggestions submitted by the public. It can either be on an entry
-- or on a relation.
DROP TABLE IF EXISTS comments CASCADE;
CREATE TABLE comments (
    id             SERIAL PRIMARY KEY,
    from_id        INTEGER NOT NULL REFERENCES entries(id) ON DELETE CASCADE ON UPDATE CASCADE, 
    to_id          INTEGER NULL REFERENCES entries(id) ON DELETE CASCADE ON UPDATE CASCADE, 
    comments       TEXT NOT NULL DEFAULT '',

    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- settings
DROP TABLE IF EXISTS settings CASCADE;
CREATE TABLE settings (
    key             TEXT NOT NULL UNIQUE,
    value           JSONB NOT NULL DEFAULT '{}',
    updated_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
DROP INDEX IF EXISTS idx_settings_key; CREATE INDEX idx_settings_key ON settings(key);
