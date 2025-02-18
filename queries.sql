-- name: search
WITH q AS (
    -- Prepare TS_QUERY tokens for querying with either:
    -- a) built in Postgres dictionary/tokenizer ($1=query, $2=Postgres dictionary name)
    -- b) externally computed and supplied tokens ($3)
    SELECT (
        CASE WHEN $2 != '' THEN
            CASE WHEN POSITION(' ' IN $1::TEXT) > 0 OR POSITION('-' IN $1::TEXT) > 0 THEN
                PLAINTO_TSQUERY($2::regconfig, $1) || PLAINTO_TSQUERY($2::regconfig, REPLACE(REPLACE($1, ' ', ''), '-', ''))
            ELSE
                PLAINTO_TSQUERY($2::regconfig, $1)
            END
        ELSE
            $3::TSQUERY
        END
    ) AS query
),
directMatch AS (
    -- Do a direct string match (first 50 chars) of the query or see if there are matches for
    -- "simple" (Postgres token dictionary that merely removes English stopwords) tokens.
    -- Rank is the inverted string length so that all results in this query have a negative
    -- value to rank higher than results from tokenMatch.
    SELECT DISTINCT ON (entries.id) entries.*, -1 * ( 50 - LENGTH(content)) AS rank FROM entries
        INNER JOIN relations ON entries.id = relations.from_id
        WHERE
        ($4 = '' OR lang=$4)
        AND (COALESCE(CARDINALITY($5::TEXT[]), 0) = 0 OR entries.tags && $5)
        AND (
            CASE WHEN $1 = '' THEN TRUE ELSE
                REGEXP_REPLACE(LOWER(SUBSTRING(content, 0, 50)), '[0-9\s]+', '', 'g') = REGEXP_REPLACE(LOWER(SUBSTRING($1, 0, 50)), '[0-9\s]+', '', 'g')
                OR tokens @@ PLAINTO_TSQUERY('simple', $1)
            END
        )
        AND (CASE WHEN $6 != '' THEN entries.status = $6::entry_status ELSE TRUE END)
),
tokenMatch AS (
    -- Full text search for words with proper tokens either from a built-in Postgres dictionary
    -- or externally computed tokens ($3) 
    SELECT DISTINCT ON (entries.id) entries.*, 1 - TS_RANK(tokens, (SELECT query FROM q), 0) AS rank FROM entries
        INNER JOIN relations ON entries.id = relations.from_id
        WHERE
        ($4 = '' OR lang=$4)
        AND (COALESCE(CARDINALITY($5::TEXT[]), 0) = 0 OR entries.tags && $5)
        AND tokens @@ (SELECT query FROM q)
        AND entries.id NOT IN (SELECT id FROM directMatch)
        AND (CASE WHEN $6 != '' THEN entries.status = $6::entry_status ELSE TRUE END)
),
results AS (
    -- Combine results from direct matches and token matches. As directMatches ranks are
    -- forced to be negative, they will rank on top. 
    SELECT DISTINCT ON (combined.id) combined.*
    FROM (
        SELECT * FROM directMatch
        UNION ALL
        SELECT * FROM tokenMatch
    ) AS combined
)
SELECT COUNT(*) OVER () AS total, * FROM results ORDER BY rank OFFSET $7 LIMIT $8;

-- name: search-relations
SELECT entries.*,
    relations.from_id AS from_id,
    relations.types AS relation_types,
    relations.tags AS relation_tags,
    relations.notes AS relation_notes,
    relations.id as relation_id,
    relations.weight as relation_weight,
    relations.status as relation_status,
    relations.created_at as relation_created_at,
    relations.updated_at as relation_updated_at
FROM entries
LEFT JOIN relations ON (relations.to_id = entries.id)
WHERE
    ($1 = '' OR lang=$1)
    AND (COALESCE(CARDINALITY($2::TEXT[]), 0) = 0 OR relations.types && $2)
    AND (COALESCE(CARDINALITY($3::TEXT[]), 0) = 0 OR relations.tags && $3)
    -- AND tokens @@ (CASE WHEN $4 != '' THEN plainto_tsquery($4::regconfig, $5::TEXT) ELSE to_tsquery($5) END)
    AND from_id = ANY($4::INT[])
    AND (CASE WHEN $5 != '' THEN relations.status = $5::entry_status ELSE TRUE END)
ORDER BY relations.weight, relations.types;

-- name: get-pending-entries
WITH ids AS (
    SELECT DISTINCT from_id FROM relations WHERE status = 'pending'
    UNION
    SELECT DISTINCT from_id FROM comments
)
SELECT COUNT(*) OVER () AS total, e.* FROM entries e
    INNER JOIN ids ON (ids.from_id = e.id)
    WHERE ($1 = '' OR lang=$1)
    AND (COALESCE(CARDINALITY($2::TEXT[]), 0) = 0 OR e.tags && $2)
    OFFSET $3 LIMIT $4;

-- name: approve-pending-entry
WITH e AS (
    UPDATE entries SET status='active' WHERE id = $1
)
UPDATE relations WHERE from_id = $1;

-- name: reject-pending-entry
WITH e AS (
    DELETE FROM relations WHERE to_id = (SELECT DISTINCT to_id FROM )
),
r AS (
    DELETE FROM entries WHERE id = $1
)
UPDATE relations WHERE from_id = $1;

-- name: get-entry
SELECT * FROM entries WHERE id=$1;

-- name: get-parent-relations
SELECT entries.*, relations.id as relation_id FROM entries
    LEFT JOIN relations ON (relations.from_id = entries.id)
    WHERE to_id = $1
    ORDER BY weight;

-- name: get-initials
-- Gets the list of unique "initial"s (first character) across all the words
-- for a given language. Useful for building indexes and glossaries.
SELECT DISTINCT(initial) as initial FROM entries
    WHERE lang=$1 AND initial != '' AND status='enabled'
    ORDER BY initial;


-- name: get-glossary-words
-- Gets words for a language to build a glossary.
SELECT COUNT(*) OVER () AS total, e.id, e.content FROM entries e
    LEFT JOIN relations ON (relations.to_id = e.id)
    WHERE relations.to_id IS NULL AND e.lang=$1 AND e.initial=$2 AND e.status='enabled'
    ORDER BY e.weight OFFSET $3 LIMIT $4;

-- name: insert-entry
WITH w AS (
    -- If weight ($4) is 0, compute a new weight by looking up the last weight
    -- for the initial of the given word and add +1 to it.
    SELECT MAX(weight) + 1 AS weight FROM entries WHERE $3=0 AND (initial=$2 AND lang=$6)
)
INSERT INTO entries (content, initial, weight, tokens, lang, tags, phones, notes, meta, status)
    VALUES(
        $1,
        $2,
        COALESCE((SELECT weight FROM w), $3),
        (CASE WHEN $5 != '' THEN TO_TSVECTOR($5::regconfig, $1::TEXT) ELSE $4::TSVECTOR END),
        $6,
        $7,
        $8,
        $9,
        $10,
        $11
    )
    RETURNING id;

-- name: update-entry
UPDATE entries SET
    content = (CASE WHEN $2 != '' THEN $2 ELSE content END),
    initial = (CASE WHEN $3 != '' THEN $3 ELSE initial END),
    weight = (CASE WHEN $4::DECIMAL != 0 THEN $4 ELSE weight END),
    tokens = (CASE WHEN $5 != '' THEN $5::TSVECTOR ELSE tokens END),
    lang = (CASE WHEN $6 != '' THEN $6 ELSE lang END),
    tags = (CASE WHEN $7::TEXT[] IS NOT NULL THEN $7 ELSE tags END),
    phones = (CASE WHEN $8::TEXT[] IS NOT NULL THEN $8 ELSE phones END),
    notes = (CASE WHEN $9 != '' THEN $9 ELSE notes END),
    meta = (CASE WHEN $10 != '' THEN $10::JSONB ELSE meta END),
    status = (CASE WHEN $11 != '' THEN $11::entry_status ELSE status END),
    updated_at = NOW()
    WHERE id = $1;

-- name: insert-relation
WITH w AS (
    -- If weight ($4) is 0, compute a new weight by looking up the last weight
    -- for the initial of the given word and add +1 to it.
    SELECT MAX(weight) + 1 AS weight FROM relations WHERE $6=0 AND from_id=$1
)
INSERT INTO relations (from_id, to_id, types, tags, notes, weight, status)
    VALUES($1, $2, $3, $4, $5, COALESCE((SELECT weight FROM w), $6), $7) RETURNING id;

-- name: reorder-relations
-- Updates the weights from 1 to N given ordered relation IDs in an array. 
UPDATE relations AS r SET weight = c.weight
    FROM (SELECT * FROM UNNEST($1::INT[]) WITH ORDINALITY w(id, weight)) AS c
    WHERE c.id = r.id;

-- name: delete-entry
DELETE FROM entries WHERE id=$1;

-- name: delete-relation
DELETE FROM relations WHERE id=$1;

-- name: get-stats
SELECT JSON_BUILD_OBJECT('entries', (SELECT COUNT(*) FROM entries),
                            'relations', (SELECT COUNT(*) FROM relations),
                            'languages', (
                                SELECT JSON_OBJECT_AGG (lang, num) FROM
                                (SELECT lang, COUNT(*) AS num FROM entries GROUP BY lang) r
                            )
                        );

-- name: insert-submission-entry
-- This differs from insert-entry which always inserts a new non-unique entry for content+lang.
-- This query checks if content+lang exists and returns its ID. If it doesn't exist, the entry
-- is inserted and the new ID is returned. This is used for public submissions which thus always
-- get related to an existing entry in the database.
WITH w AS (
    SELECT MAX(weight) + 1 AS weight FROM entries WHERE $3=0 AND (initial=$2 AND lang=$6)
),
old AS (
    SELECT id FROM entries WHERE
    LOWER(SUBSTRING(content, 0, 50)) = LOWER(SUBSTRING($1, 0, 50)) AND lang = $6 AND status != 'disabled'
    LIMIT 1
),
e AS (
    INSERT INTO entries (content, initial, weight, tokens, lang, tags, phones, notes, meta, status)
    SELECT
        $1,
        $2,
        COALESCE((SELECT weight FROM w), $3),
        (CASE WHEN $5::TEXT != '' THEN TO_TSVECTOR($5::regconfig, $1::TEXT) ELSE $4::TSVECTOR END),
        $6,
        $7,
        $8,
        $9,
        $10,
        $11
    WHERE NOT EXISTS (SELECT * FROM old)
    RETURNING id
)
SELECT id FROM e UNION ALL SELECT id FROM old;

-- name: insert-submission-relation
-- Only inserts if the same from_id + to_id + type[] doesn't already exist.
WITH old AS (
    SELECT id FROM relations WHERE from_id = $1 AND to_id = $2 AND types && $3 LIMIT 1
),
w AS (
    -- If weight ($4) is 0, compute a new weight by looking up the last weight
    -- for the initial of the given word and add +1 to it.
    SELECT MAX(weight) + 1 AS weight FROM relations WHERE from_id=$1 AND $6=0
),
e AS (
    INSERT INTO relations (from_id, to_id, types, tags, notes, weight, status)
    SELECT $1, $2, $3, $4, $5, COALESCE((SELECT weight FROM w), $6), $7
    WHERE NOT EXISTS (SELECT * FROM old)
    RETURNING id
)
SELECT id FROM e UNION ALL SELECT id FROM old;

-- name: update-relation
UPDATE relations SET
    types = (CASE WHEN $2::TEXT[] IS NOT NULL THEN $2 ELSE types END),
    tags = (CASE WHEN $3::TEXT[] IS NOT NULL THEN $3 ELSE tags END),
    notes = $4,
    weight = (CASE WHEN $5::DECIMAL != 0 THEN $5 ELSE weight END),
    updated_at = NOW()
WHERE id = $1;

-- name: approve-submission
WITH e AS (
    -- Approve the pending main entry.
    UPDATE entries SET status = 'enabled', updated_at = NOW() WHERE id = $1 AND status = 'pending'
),
e2 AS (
    -- Approve all the definition entries connected to the main entry.
    UPDATE entries SET status = 'enabled', updated_at = NOW()
    WHERE status = 'pending' AND id = ANY(SELECT to_id FROM relations WHERE from_id = $1)
)
UPDATE relations SET status = 'enabled', updated_at = NOW() WHERE from_id = $1 AND status = 'pending';

-- name: reject-submission
WITH e AS (
    DELETE FROM entries WHERE id = $1 AND status = 'pending'
),
e2 AS (
    DELETE FROM entries WHERE status = 'pending'
    AND id = ANY(SELECT to_id FROM relations WHERE from_id = $1)
)
DELETE FROM relations WHERE from_id = $1 AND status = 'pending';

-- name: insert-comments
-- Insert comments / suggestions coming from the public.
WITH f AS (SELECT id FROM entries WHERE $1::TEXT != '' AND guid = $1::UUID),
     t AS (SELECT id FROM entries WHERE $2::TEXT != '' AND guid = $2::UUID)
INSERT INTO comments (from_id, to_id, comments)
    VALUES((SELECT id FROM f), (SELECT id FROM t), $3);

-- name: get-comments
SELECT * FROM comments;

-- name: delete-comments
DELETE FROM comments WHERE id = $1;

-- name: delete-all-pending
WITH d1 AS (
    DELETE FROM entries WHERE status = 'pending'
), d2 AS (
    DELETE FROM relations WHERE status = 'pending'
)
DELETE FROM comments;

-- name: get-entries-for-sitemap
SELECT DISTINCT e.content FROM entries e INNER JOIN relations r ON e.id = r.from_id WHERE e.lang = $1 ORDER BY content;
