-- name: search
-- FTS5 search with length-based ranking (shorter content ranks higher).
-- Exact content matches get extra boost via negative rank adjustment.
-- $1: lang, $2: raw query, $3: FTS query, $4: status, $5: offset, $6: limit
SELECT DISTINCT e.*,
       -- Rank: weight - (50 - content_length). Shorter content = more negative = ranks first.
       -- Exact matches get extra -1000 boost to always rank highest.
       e.weight + (-1.0 * (50.0 - length(JSON_EXTRACT(e.content, '$[0]'))))
       + CASE WHEN e.content_head = LOWER(SUBSTR($2, 1, 50)) THEN -1000.0 ELSE 0.0 END
       AS rank,
       COUNT(*) OVER() AS total
FROM entries e
INNER JOIN entries_fts fts ON fts.rowid = e.id
INNER JOIN relations r ON e.id = r.from_id
WHERE entries_fts MATCH $3
  AND ($1 = '' OR e.lang = $1)
  AND ($4 = '' OR e.status = $4)
  AND $2 != ''
ORDER BY rank
LIMIT $6 OFFSET $5;

-- name: search-relations
-- Load relations for a set of entry IDs.
-- $1: entry IDs (JSON array), $2: to_lang, $3: types (JSON array), $4: tags (JSON array), $5: status, $6: max per type
SELECT e.*,
       r.from_id,
       r.id AS relation_id,
       r.types AS relation_types,
       r.tags AS relation_tags,
       r.notes AS relation_notes,
       r.weight AS relation_weight,
       r.status AS relation_status,
       r.created_at AS relation_created_at,
       r.updated_at AS relation_updated_at
FROM entries e
INNER JOIN relations r ON r.to_id = e.id
WHERE r.from_id IN (SELECT value FROM json_each($1))
  AND ($2 = '' OR e.lang = $2)
  AND ($5 = '' OR r.status = $5)
  AND ($3 = '[]' OR EXISTS (
      SELECT 1 FROM json_each(r.types) rt, json_each($3) qt WHERE rt.value = qt.value
  ))
  AND ($4 = '[]' OR EXISTS (
      SELECT 1 FROM json_each(r.tags) rt, json_each($4) qt WHERE rt.value = qt.value
  ))
ORDER BY r.from_id, r.types, r.weight;

-- name: get-entry
SELECT * FROM entries WHERE
    CASE
        WHEN $1 > 0 THEN id = $1
        WHEN $2 != '' THEN guid = $2
        ELSE 0
    END;

-- name: get-parent-relations
SELECT e.*, r.id AS relation_id FROM entries e
    LEFT JOIN relations r ON r.from_id = e.id
    WHERE r.to_id = $1
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
        SELECT value, row_number() OVER () AS idx FROM json_each($1)
    ) WHERE value = relations.id
)
WHERE id IN (SELECT value FROM json_each($1));

-- name: get-stats
SELECT json_object(
    'entries', (SELECT COUNT(*) FROM entries),
    'relations', (SELECT COUNT(*) FROM relations),
    'languages', (
        SELECT json_group_object(lang, cnt) FROM (
            SELECT lang, COUNT(*) AS cnt FROM entries GROUP BY lang
        )
    )
);

-- name: get-pending-entries
SELECT e.*, COUNT(*) OVER() AS total FROM entries e
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
        WHERE lower(substr(json_extract(content, '$[0]'), 1, 50)) = lower(substr(json_extract($2, '$[0]'), 1, 50))
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
        AND EXISTS (SELECT 1 FROM json_each(types) rt, json_each($3) nt WHERE rt.value = nt.value)
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
