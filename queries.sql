-- name: search
WITH q AS (
    -- Prepare TS_QUERY tokens for querying with either:
    -- a) built in Postgres dictionary/tokenizer ($1=query, $2=Postgres dictionary name)
    -- b) externally computed and supplied tokens ($3)
    SELECT (CASE WHEN $2 != '' THEN PLAINTO_TSQUERY($2::regconfig, $1::TEXT) ELSE TO_TSQUERY($3) END) AS query
),
directMatch AS (
    -- Do a direct string match (first 50 chars) of the query or see if there are matches for
    -- "simple" (Postgres token dictionary that merely removes English stopwords) tokens.
    -- Rank is the inverted string length so that all results in this query have a negative
    -- rank, where the smaller numbers represent shorter strings. That is, shorter strings
    -- are considered closer matches. 
    SELECT COUNT(*) OVER () AS total, entries.*, -1 * ( 50 - LENGTH(content)) AS rank FROM entries WHERE
        ($4 = '' OR lang=$4)
        AND (COALESCE(CARDINALITY($5::TEXT[]), 0) = 0 OR types && $5)
        AND (COALESCE(CARDINALITY($6::TEXT[]), 0) = 0 OR tags && $6)
        AND (
            LOWER(SUBSTRING(content, 0, 50))=LOWER(SUBSTRING($1, 0, 50))
            OR tokens @@ PLAINTO_TSQUERY('simple', $1)
        )
        AND (CASE WHEN $7 != '' THEN status = $7::entry_status ELSE TRUE END)
),
tokenMatch AS (
    -- Full text search for words with proper tokens either from a built-in Postgres dictionary
    -- or externally computed tokens ($3) 
    SELECT COUNT(*) OVER () AS total, entries.*, 1 - TS_RANK(tokens, (SELECT query FROM q), 1) AS rank FROM entries WHERE
        ($4 = '' OR lang=$4)
        AND (COALESCE(CARDINALITY($5::TEXT[]), 0) = 0 OR types && $5)
        AND (COALESCE(CARDINALITY($6::TEXT[]), 0) = 0 OR tags && $6)
        AND tokens @@ (SELECT query FROM q)
        AND id NOT IN (SELECT id FROM directMatch)
        AND (CASE WHEN $7 != '' THEN status = $7::entry_status ELSE TRUE END)
)
-- Combine results from direct matches and token matches. As directMatches ranks are
-- forced to be negative, they will rank on top. 
SELECT * FROM directMatch UNION ALL SELECT * FROM tokenMatch ORDER BY rank OFFSET $8 LIMIT $9;


-- name: get-relations
SELECT entries.*,
    relations.from_id AS from_id,
    relations.types AS relation_types,
    relations.tags AS relation_tags,
    relations.notes AS relation_notes
FROM entries
LEFT JOIN relations ON (relations.to_id = entries.id)
WHERE
    ($1 = '' OR lang=$1)
    AND (COALESCE(CARDINALITY($2::TEXT[]), 0) = 0 OR relations.types && $2)
    AND (COALESCE(CARDINALITY($3::TEXT[]), 0) = 0 OR relations.tags && $3)
    -- AND tokens @@ (CASE WHEN $4 != '' THEN plainto_tsquery($4::regconfig, $5::TEXT) ELSE to_tsquery($5) END)
    AND from_id = ANY($4::INT[])
    AND (CASE WHEN $5 != '' THEN status = $5::entry_status ELSE TRUE END)
ORDER BY relations.weight;


-- name: get-initials
-- Gets the list of unique "initial"s (first character) across all the words
-- for a given language. Useful for building indexes and glossaries.
SELECT DISTINCT(initial) as initial FROM entries
    WHERE lang=$1 AND initial != '' AND status='enabled'
    ORDER BY initial;


-- name: get-glossary-words
-- Gets words for a language to build a glossary.
SELECT COUNT(*) OVER () AS total, id, guid, content FROM entries
    WHERE lang=$1 AND initial=$2 AND status='enabled'
    ORDER BY weight OFFSET $3 LIMIT $4;
