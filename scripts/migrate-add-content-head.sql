-- Migration script: Add content_head generated column for fast search
-- Run this on existing data.db before using the updated code.
--
-- Usage: sqlite3 data.db < scripts/migrate-add-content-head.sql

-- Add generated column for fast content matching (first 50 chars, lowercased).
-- This avoids computing json_extract + substr on every row during search.
ALTER TABLE entries ADD COLUMN content_head TEXT
    GENERATED ALWAYS AS (lower(substr(json_extract(content, '$[0]'), 1, 50))) STORED;

-- Create index on the generated column for fast lookups.
CREATE INDEX idx_entries_content_head ON entries(content_head);

-- Rebuild FTS index (good practice after schema changes).
INSERT INTO entries_fts(entries_fts) VALUES('rebuild');

-- Verify the migration
SELECT 'Migration complete. Sample content_head values:';
SELECT id, content_head FROM entries LIMIT 5;
