-- name: search
WITH q AS (
    SELECT (CASE WHEN $2 != '' THEN plainto_tsquery($2::regconfig, $1::TEXT) ELSE to_tsquery($3) END) AS query
),
directMatch AS (
    -- If res did not return any results, match with 'simple' tokens (that avoid stopwords).
    -- The UNION here only executes this leg of the query if the previous query did not
    -- return anything, hence there is no added cost.
    SELECT COUNT(*) OVER () AS total, entries.*, LENGTH(content) AS rank, weight FROM entries WHERE
        ($4 = '' OR lang=$4)
        AND (COALESCE(CARDINALITY($5::TEXT[]), 0) = 0 OR types && $5)
        AND (COALESCE(CARDINALITY($6::TEXT[]), 0) = 0 OR tags && $6)
        AND (LOWER(SUBSTRING(content, 0, 50))=LOWER(SUBSTRING($1, 0, 50)) OR tokens @@ TO_TSQUERY($2::regconfig, $1))
        ORDER BY rank DESC
        OFFSET $7 LIMIT $8
),
tokenMatch AS (
    -- Match records with the proper tokens
    SELECT COUNT(*) OVER () AS total, entries.*, TS_RANK(tokens, (SELECT query FROM q), 1) AS rank, weight FROM entries WHERE
        ($4 = '' OR lang=$4)
        AND (COALESCE(CARDINALITY($5::TEXT[]), 0) = 0 OR types && $5)
        AND (COALESCE(CARDINALITY($6::TEXT[]), 0) = 0 OR tags && $6)
        AND tokens @@ (SELECT query FROM q) AND $1=$1
        AND id NOT IN (SELECT id FROM directMatch)
        ORDER BY rank DESC
        OFFSET $7 LIMIT $8
)
SELECT * FROM directMatch UNION SELECT * FROM tokenMatch;

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
ORDER BY relations.weight;

-- name: get-initials
-- Gets the list of unique "initial"s (first character) across all the words
-- for a given language. Useful for building indexes and glossaries.
SELECT DISTINCT(initial) as initial FROM entries WHERE lang=$1 AND initial != '' ORDER BY initial;

-- name: get-glossary-words
-- Gets words for a language to build a glossary.
SELECT COUNT(*) OVER () AS total, id, guid, content FROM entries WHERE lang=$1 AND initial=$2 ORDER BY weight OFFSET $3 LIMIT $4;
