-- If the DB is not empty, to get a clean slate, run
-- TRUNCATE TABLE entries RESTART IDENTITY CASCADE; TRUNCATE TABLE relations RESTART IDENTITY CASCADE;

-- Insert head words apple, application (id=1, 2)
INSERT INTO entries (guid, lang, content, initial, tokens, phones ) VALUES
    (MD5('apple'), 'english', 'Apple', 'A', TO_TSVECTOR('apple'), '{/ˈæp.əl/, aapl}'),
    (MD5('application'), 'english', 'Application', 'A', TO_TSVECTOR('application'), '{/aplɪˈkeɪʃ(ə)n/}');


-- Insert English definitions for apple. (id=3, 4, 5)
INSERT INTO entries (guid, lang, content) VALUES
    (MD5('apple-1'), 'english', 'round, red or yellow, edible fruit of a small tree'),
    (MD5('apple-2'), 'english', 'the tree, cultivated in most temperate regions.'),
    (MD5('apple-3'), 'english', 'anything resembling an apple in size and shape, as a ball, especially a baseball.');
-- Insert English apple-definition relationships.
INSERT INTO relations (from_id, to_id, types, weight) VALUES
    (1, 3, '{noun}', 0),
    (1, 4, '{noun}', 1),
    (1, 5, '{noun}', 2);

-- Insert Italian definitions for apple. (id=6, 7)
INSERT INTO entries (guid, lang, content) VALUES
    (MD5('apple-it-1'), 'italian', 'mela'),
    (MD5('apple-it-2'), 'italian', 'il pomo.');
-- Insert Italian apple-definition relationships.
INSERT INTO relations (from_id, to_id, types, weight) VALUES
    (1, 6, '{noun}', 0),
    (1, 7, '{noun}', 1);


--
-- Insert English definitions for application. (id=8, 9)
INSERT INTO entries (guid, lang, content) VALUES
    (MD5('application-1'), 'english', 'the act of putting to a special use or purpose'),
    (MD5('application-2'), 'english', 'the act of requesting.');
-- Insert English application-definition relationships.
INSERT INTO relations (from_id, to_id, types, weight) VALUES
    (2, 3, '{noun}', 8),
    (2, 4, '{noun}', 9);

-- Insert Italian definitions for application. (id=10, 11, 12)
INSERT INTO entries (guid, lang, content) VALUES
    (MD5('application-it-1'), 'italian', 'le applicazione'),
    (MD5('application-it-2'), 'italian', 'la domanda'),
    (MD5('application-it-3'), 'italian', 'la richiesta');
-- Insert Italian application-definition relationships.
INSERT INTO relations (from_id, to_id, types, weight) VALUES
    (2, 10, '{noun}', 0),
    (2, 11, '{noun}', 1),
    (2, 12, '{noun}', 1);
