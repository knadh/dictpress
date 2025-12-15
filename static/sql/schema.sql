-- name: pragma
-- Concurrency (minimal write concern)
PRAGMA journal_mode       = WAL;
PRAGMA busy_timeout       = 1000;    -- Shorter; writes are rare.
PRAGMA wal_autocheckpoint = 0;       -- Disable auto-checkpoint; do it manually during maintenance
PRAGMA cache_size         = -256000; -- 256MB cache (or more if available)
PRAGMA temp_store         = MEMORY;
PRAGMA mmap_size          = 1073741824; -- 1GB mmap - keep entire DB in memory if possible
PRAGMA foreign_keys       = ON;
PRAGMA query_only         = OFF;
PRAGMA analysis_limit     = 1000;


-- name: schema
CREATE TABLE IF NOT EXISTS entries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    guid TEXT NOT NULL UNIQUE,
    content TEXT NOT NULL DEFAULT '[]',
    initial TEXT NOT NULL DEFAULT '',
    weight REAL NOT NULL DEFAULT 0,
    tokens TEXT NOT NULL DEFAULT '',
    lang TEXT NOT NULL,
    tags TEXT NOT NULL DEFAULT '[]',
    phones TEXT NOT NULL DEFAULT '[]',
    notes TEXT NOT NULL DEFAULT '',
    meta TEXT NOT NULL DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'enabled' CHECK(status IN ('pending', 'enabled', 'disabled')),
    created_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),

    -- Generated column for direct word matching (first 20 chars, lowercased).
    content_head TEXT GENERATED ALWAYS AS (LOWER(SUBSTR(JSON_EXTRACT(content, '$[0]'), 1, 20))) STORED
) STRICT;

CREATE INDEX IF NOT EXISTS idx_entries_initial ON entries(lang, status, initial);
CREATE INDEX IF NOT EXISTS idx_entries_lang_initial ON entries(lang, initial, status);
CREATE INDEX IF NOT EXISTS idx_entries_lang_contenthead ON entries(lang, content_head) WHERE status != 'disabled';

-- FTS5 virtual table for fulltext search.
CREATE VIRTUAL TABLE IF NOT EXISTS entries_fts USING fts5(tokens, content='');

-- Triggers to keep FTS in sync with entries.
CREATE TRIGGER IF NOT EXISTS trg_entries_ai AFTER INSERT ON entries BEGIN
    INSERT INTO entries_fts(rowid, tokens) VALUES (NEW.id, NEW.tokens);
END;

CREATE TRIGGER IF NOT EXISTS trg_entries_ad AFTER DELETE ON entries BEGIN
    INSERT INTO entries_fts(entries_fts, rowid, tokens) VALUES('delete', OLD.id, OLD.tokens);
END;

CREATE TRIGGER IF NOT EXISTS trg_entries_au AFTER UPDATE OF tokens ON entries BEGIN
    INSERT INTO entries_fts(entries_fts, rowid, tokens) VALUES('delete', OLD.id, OLD.tokens);
    INSERT INTO entries_fts(rowid, tokens) VALUES (NEW.id, NEW.tokens);
END;

CREATE TABLE IF NOT EXISTS relations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    from_id INTEGER NOT NULL REFERENCES entries(id) ON DELETE CASCADE,
    to_id INTEGER NOT NULL REFERENCES entries(id) ON DELETE CASCADE,
    types TEXT NOT NULL DEFAULT '[]',
    tags TEXT NOT NULL DEFAULT '[]',
    notes TEXT NOT NULL DEFAULT '',
    weight REAL NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'enabled' CHECK(status IN ('pending', 'enabled', 'disabled')),
    created_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE(from_id, to_id)
) STRICT;

CREATE INDEX IF NOT EXISTS idx_relations_to ON relations(to_id);
CREATE INDEX IF NOT EXISTS idx_relations_status ON relations(status);
CREATE INDEX IF NOT EXISTS idx_relations_from ON relations(from_id, status, weight);

CREATE TABLE IF NOT EXISTS comments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    from_id INTEGER NOT NULL REFERENCES entries(id) ON DELETE CASCADE,
    to_id INTEGER REFERENCES entries(id) ON DELETE CASCADE,
    comments TEXT NOT NULL DEFAULT '',
    created_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
) STRICT;

CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL DEFAULT '{}',
    updated_at TEXT DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
) STRICT;
