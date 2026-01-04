-- name: search
-- FTS5 search with length-based ranking (shorter content ranks higher).
-- Exact content matches get extra boost via negative rank adjustment.
-- Supports both FTS matches and direct content_head matches (for multi-word phrases
-- where tokens may be incomplete). Uses UNION for better index utilization.
-- $1: lang, $2: raw query, $3: FTS query, $4: status, $5: offset, $6: limit
SELECT e.*,
       JSON_ARRAY_LENGTH(e.content) AS content_length,
       -- Rank: weight - (20 - content_length). Shorter content = more negative = ranks first.
       --       20 as that's the max length of the generated content_head column.
       -- Exact matches get extra -1000 boost to always rank highest.
       e.weight + (-1.0 * (20.0 - LENGTH(e.content_head)))
       + CASE WHEN e.content_head = LOWER(SUBSTR($2, 1, 20)) THEN -1000.0 ELSE 0.0 END
       AS rank,
       COUNT(*) OVER() AS total
FROM entries e
WHERE e.id IN (
    -- FTS matches
    SELECT rowid FROM entries_fts WHERE entries_fts MATCH $3
    UNION
    -- Direct content_head matches (for multi-word phrases with incomplete tokens)
    SELECT id FROM entries
    WHERE content_head = LOWER(SUBSTR($2, 1, 20))
      AND ($1 = '' OR lang = $1)
      AND status != 'disabled'
)
  AND EXISTS (SELECT 1 FROM relations r WHERE r.from_id = e.id)
  AND ($1 = '' OR e.lang = $1)
  AND ($4 = '' OR e.status = $4)
ORDER BY rank
LIMIT $6 OFFSET $5;

-- name: search-words
-- Return autocomplete results for a search query.
-- $1: lang, $2: raw query string, $3: FTS query, $4: limit
SELECT e.content
FROM entries e
INNER JOIN entries_fts fts ON fts.rowid = e.id
WHERE entries_fts MATCH $3
  AND ($1 = '' OR e.lang = $1)
  AND e.status = 'enabled'
  AND EXISTS (SELECT 1 FROM relations r WHERE r.from_id = e.id)
ORDER BY LENGTH(e.content_head)
LIMIT $4;

-- name: search-relations
-- Load relations for a set of entry IDs.
-- $1: entry IDs (JSON array), $2: to_lang, $3: types (JSON array), $4: tags (JSON array),
-- $5: status, $6: max per type, $7: max content items (0 = no truncation)
SELECT e.id, e.guid, e.initial, e.weight, e.tokens, e.lang,
       e.tags, e.phones, e.notes, e.meta, e.status, e.created_at, e.updated_at,
       -- Truncate content array if $7 > 0
       CASE WHEN $7 > 0
            THEN (SELECT JSON_GROUP_ARRAY(value) FROM (SELECT value FROM JSON_EACH(e.content) LIMIT $7))
            ELSE e.content
       END AS content,
       JSON_ARRAY_LENGTH(e.content) AS content_length,
       sub.from_id,
       sub.relation_id,
       sub.relation_types,
       sub.relation_tags,
       sub.relation_notes,
       sub.relation_weight,
       sub.relation_status,
       sub.relation_created_at,
       sub.relation_updated_at,
       sub.total_relations
FROM entries e
INNER JOIN (
    SELECT r.to_id,
           r.from_id,
           r.id AS relation_id,
           r.types AS relation_types,
           r.tags AS relation_tags,
           r.notes AS relation_notes,
           r.weight AS relation_weight,
           r.status AS relation_status,
           r.created_at AS relation_created_at,
           r.updated_at AS relation_updated_at,
           ROW_NUMBER() OVER (PARTITION BY r.from_id, r.types ORDER BY r.weight) AS rn,
           COUNT(*) OVER (PARTITION BY r.from_id) AS total_relations
    FROM relations r
    WHERE r.from_id IN (SELECT value FROM JSON_EACH($1))
      AND ($5 = '' OR r.status = $5)
      AND ($3 = '[]' OR EXISTS (
          SELECT 1 FROM JSON_EACH(r.types) rt, JSON_EACH($3) qt WHERE rt.value = qt.value
      ))
      AND ($4 = '[]' OR EXISTS (
          SELECT 1 FROM JSON_EACH(r.tags) rt, JSON_EACH($4) qt WHERE rt.value = qt.value
      ))
) sub ON sub.to_id = e.id AND ($6 = 0 OR sub.rn <= $6)
WHERE ($2 = '' OR e.lang = $2)
ORDER BY sub.from_id, sub.relation_types, sub.relation_weight;

-- name: get-entry
SELECT *, JSON_ARRAY_LENGTH(content) AS content_length FROM entries
WHERE ($1 > 0 AND id = $1) OR ($1 <= 0 AND $2 != '' AND guid = $2);

-- name: get-parent-relations
SELECT e.*, r.id AS relation_id FROM relations r
JOIN entries e ON e.id = r.from_id WHERE r.to_id = $1
ORDER BY e.weight;

-- name: get-initials
SELECT DISTINCT initial FROM entries
    WHERE lang = $1 AND initial != '' AND status = 'enabled'
    ORDER BY initial;

-- name: get-glossary-words
SELECT e.id, e.guid, e.content, COUNT(*) OVER() AS total FROM entries e
    LEFT JOIN relations r ON r.to_id = e.id
    WHERE r.to_id IS NULL AND e.lang = $1 AND e.initial = $2 AND e.status = 'enabled'
    ORDER BY e.weight
    LIMIT $4 OFFSET $3;

-- name: insert-entry
INSERT INTO entries (guid, content, initial, weight, tokens, lang, tags, phones, notes, meta, status)
    VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
    RETURNING id;

-- name: update-entry
UPDATE entries SET
    content = CASE WHEN $2 != '[]' THEN $2 ELSE content END,
    initial = CASE WHEN $3 != '' THEN $3 ELSE initial END,
    weight = CASE WHEN $4 != 0 THEN $4 ELSE weight END,
    tokens = CASE WHEN $5 != '' THEN $5 ELSE tokens END,
    lang = CASE WHEN $6 != '' THEN $6 ELSE lang END,
    tags = CASE WHEN $7 IS NOT NULL THEN $7 ELSE tags END,
    phones = CASE WHEN $8 IS NOT NULL THEN $8 ELSE phones END,
    notes = CASE WHEN $9 != '' THEN $9 ELSE notes END,
    meta = CASE WHEN $10 != '{}' THEN $10 ELSE meta END,
    status = CASE WHEN $11 != '' THEN $11 ELSE status END,
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE id = $1;

-- name: delete-entry
DELETE FROM entries WHERE id = $1;

-- name: insert-relation
INSERT INTO relations (from_id, to_id, types, tags, notes, weight, status)
    VALUES ($1, $2, $3, $4, $5, $6, $7)
    RETURNING id;

-- name: update-relation
UPDATE relations SET
    types = CASE WHEN $2 IS NOT NULL THEN $2 ELSE types END,
    tags = CASE WHEN $3 IS NOT NULL THEN $3 ELSE tags END,
    notes = $4,
    weight = CASE WHEN $5 != 0 THEN $5 ELSE weight END,
    status = CASE WHEN $6 != '' THEN $6 ELSE status END,
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE id = $1;

-- name: delete-relation
DELETE FROM relations WHERE id = $1;

-- name: reorder-relations
-- Update weights based on array position.
UPDATE relations SET weight = (
    SELECT idx FROM (
        SELECT value, row_number() OVER () AS idx FROM JSON_EACH($1)
    ) WHERE value = relations.id
)
WHERE id IN (SELECT value FROM JSON_EACH($1));

-- name: get-all-words
-- Fetch all words for a given language that have at least one relation.
-- Used for pre-loading autocomplete tries.
-- $1: lang
SELECT DISTINCT LOWER(j.value) AS word
FROM entries, JSON_EACH(entries.content) AS j
WHERE entries.lang = $1 AND entries.status = 'enabled'
  AND EXISTS (SELECT 1 FROM relations r WHERE r.from_id = entries.id)
ORDER BY word ASC;

-- name: get-stats
SELECT json_object(
    'entries', (SELECT COUNT(*) FROM entries),
    'relations', (SELECT COUNT(*) FROM relations),
    'languages', (
        SELECT JSON_GROUP_OBJECT(lang, cnt) FROM (
            SELECT lang, COUNT(*) AS cnt FROM entries GROUP BY lang
        )
    )
);

-- name: get-pending-entries
SELECT e.*, JSON_ARRAY_LENGTH(e.content) AS content_length, COUNT(*) OVER() AS total FROM entries e
    WHERE e.id IN (
        SELECT DISTINCT from_id FROM relations WHERE status = 'pending'
        UNION
        SELECT DISTINCT from_id FROM comments
    )
    AND ($1 = '' OR e.lang = $1)
    LIMIT $3 OFFSET $2;

-- name: insert-submission-entry
-- Check if content+lang exists, return existing ID. Otherwise insert new.
INSERT INTO entries (guid, content, initial, weight, tokens, lang, tags, phones, notes, meta, status)
    SELECT $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
    WHERE NOT EXISTS (
        SELECT 1 FROM entries
        WHERE content_head = LOWER(SUBSTR(JSON_EXTRACT($2, '$[0]'), 1, 20))
        AND lang = $6 AND status != 'disabled'
    )
    RETURNING id;

-- name: insert-submission-relation
-- Only insert if from_id + to_id + overlapping types doesn't exist.
INSERT INTO relations (from_id, to_id, types, tags, notes, weight, status)
    SELECT $1, $2, $3, $4, $5, $6, $7
    WHERE NOT EXISTS (
        SELECT 1 FROM relations
        WHERE from_id = $1 AND to_id = $2
        AND EXISTS (SELECT 1 FROM JSON_EACH(types) rt, JSON_EACH($3) nt WHERE rt.value = nt.value)
    )
    RETURNING id;

-- name: approve-submission
-- Approve entry and all its pending relations.
UPDATE entries SET status = 'enabled', updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
    WHERE id = $1 AND status = 'pending';

-- name: approve-submission-relations
UPDATE relations SET status = 'enabled', updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
    WHERE from_id = $1 AND status = 'pending';

-- name: approve-submission-to-entries
UPDATE entries SET status = 'enabled', updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
    WHERE status = 'pending' AND id IN (SELECT to_id FROM relations WHERE from_id = $1);

-- name: reject-submission
DELETE FROM entries WHERE id = $1 AND status = 'pending';

-- name: reject-submission-relations
DELETE FROM relations WHERE from_id = $1 AND status = 'pending';

-- name: reject-submission-to-entries
DELETE FROM entries WHERE status = 'pending'
    AND id IN (SELECT to_id FROM relations WHERE from_id = $1);

-- name: insert-comment
INSERT INTO comments (from_id, to_id, comments)
    VALUES (
        (SELECT id FROM entries WHERE guid = $1),
        (SELECT id FROM entries WHERE guid = $2),
        $3
    );

-- name: get-comments
SELECT c.*, e1.guid AS from_guid, e2.guid AS to_guid
FROM comments c
LEFT JOIN entries e1 ON c.from_id = e1.id
LEFT JOIN entries e2 ON c.to_id = e2.id;

-- name: delete-comment
DELETE FROM comments WHERE id = $1;

-- name: delete-all-pending
DELETE FROM entries WHERE status = 'pending';

-- name: delete-all-pending-relations
DELETE FROM relations WHERE status = 'pending';

-- name: delete-all-comments
DELETE FROM comments;
