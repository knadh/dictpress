dictpress comes with a built in CSV to database importer tool. Once dictionary data has been organised into
the below described structure, import it by running `./dictpress --import=yourfile.csv`.

Entries with the same content in the same language are not inserted into the database multiple times, but are instead re-used.
For instance, if there are multiple `Apple (English)` entries, it is inserted once but re-used in multiple relations.

## Sample CSV format
```csv
-,A,Apple,english,Optional note,english,"",optional-tag1|tag2,"ˈæp.əl|aapl","","{""etym"": ""ml""}"
^,"","round, red or yellow, edible fruit of a small tree",english,"","","","","",noun,""
^,"","the tree, cultivated in most temperate regions.",english,"","","","","",noun,""
^,"","il pomo.",italian,"","","","","",sost,""
-,A,Application,english,Optional note,italian,"","","aplɪˈkeɪʃ(ə)n","",""
^,"","the act of putting to a special use or purpose",english,"","","","","",noun,""
^,"","le applicazione",italian,"","","","","",sost,""

```

Every line in the CSV file contains an entry in a given language described in 10 columns.
Each entry is either a main entry in the dictionary, or a definition of another entry.
This is indicated by the first column in each line. `-` represents a main entry and all subsequent
entries below it marked with `^` represents its definitions in one or more languages.

The above example shows two main English entries, "Apple" and "Application" with multiple
English and Italian definitions below them.

## CSV fields

| Column | Field             |                                                                                                                                                                                                                                                                                                                                                                                                                                              |                   |        |
|:-------|:------------------|:---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|:------------------|:-------|
| 0      | type              | `-` represents a main entry. `^` under it represents a definition entry.                                                                                                                                                                                                                                                                                                                                                                     |                   |        |
| 1      | initial           | The uppercase first character of the entry. Eg: `A` for Apple. If left empty, it is automatically picked up.                                                                                                                                                                                                                                                                                                                                 |                   |        |
| 2      | content           | The entry content (word or phrase).                                                                                                                                                                                                                                                                                                                                                                                                          |                   |        |
| 3      | language          | Language of the entry (as defined in the config).                                                                                                                                                                                                                                                                                                                                                                                            |                   |        |
| 4      | notes             | Optional notes describing the entry.                                                                                                                                                                                                                                                                                                                                                                                                         |                   |        |
| 5      | tsvector_language | If the language has a built in Postgres fulltext tokenizer, the name of the tokenizer language. For languages that do not have Postgres tokenizers, this should be empty.                                                                                                                                                                                                                                                                    |                   |        |
| 6      | tsVector_tokens   | Postgres fulltext search tokens for the entry (Content). If `tsvector_language` is specified, this field can be left empty as the tokens are automatically created in the database using `TO_TSVECTOR($tsvector_language, $content)`. For languages without Postgres tokenizers, the [tsvector](https://www.postgresql.org/docs/10/datatype-textsearch.html#DATATYPE-TSVECTOR) token string should be computed externally and provided here. |                   |        |
| 7      | tags              | Optional tags describing the entry. Separate multiple tags by `\                                                                                                                                                                                                                                                                                                                                                                             | `.                |        |
| 8      | phones            | Optional phonetic notations representing the pronunciations of the entry. Separate multiple phones by `\                                                                                                                                                                                                                                                                                                                                     | `.                |        |
| 9      | definition-types  | This should only be set for definition entries that ar marked with `Type = ^`. One or more parts-of-speech types separated by `\                                                                                                                                                                                                                                                                                                             | `. Example `noun\ | verb`. |
| 10     | meta              | Otional JSON metadata. Quotes inside JSON are escaped by doubling them. Eg: `{"etym": "ml"} => {""etym"": ""ml""}` |


# Importing with SQL
Generating SQL for dictionary data and loading that directly into the database can give fine grained control
The following is the SQL equivalent of the above CSV. The Postgres database tables schemas are [described here](data-structure.md).


```sql
-- If the DB is not empty, to wipe everything and get a clean slate, run:
-- TRUNCATE TABLE entries RESTART IDENTITY CASCADE; TRUNCATE TABLE relations RESTART IDENTITY CASCADE;

-- Insert head words apple, application (id=1, 2)
INSERT INTO entries (lang, content, initial, tokens, phones) VALUES
    ('english', 'Apple', 'A', TO_TSVECTOR('apple'), '{/ˈæp.əl/, aapl}'),
    ('english', 'Application', 'A', TO_TSVECTOR('application'), '{/aplɪˈkeɪʃ(ə)n/}');


-- Insert English definitions for apple. (id=3, 4, 5)
INSERT INTO entries (lang, content) VALUES
    ('english', 'round, red or yellow, edible fruit of a small tree'),
    ('english', 'the tree, cultivated in most temperate regions.'),
    ('english', 'anything resembling an apple in size and shape, as a ball, especially a baseball.');
-- Insert English apple-definition relationships.
INSERT INTO relations (from_id, to_id, types, weight) VALUES
    (1, 3, '{noun}', 0),
    (1, 4, '{noun}', 1),
    (1, 5, '{noun}', 2);

-- Insert Italian definitions for apple. (id=6, 7)
INSERT INTO entries (lang, content) VALUES
    ('italian', 'mela'),
    ('italian', 'il pomo.');
-- Insert Italian apple-definition relationships.
INSERT INTO relations (from_id, to_id, types, weight) VALUES
    (1, 6, '{noun}', 0),
    (1, 7, '{noun}', 1);


--
-- Insert English definitions for application. (id=8, 9)
INSERT INTO entries (lang, content) VALUES
    ('english', 'the act of putting to a special use or purpose'),
    ('english', 'the act of requesting.');
-- Insert English application-definition relationships.
INSERT INTO relations (from_id, to_id, types, weight) VALUES
    (2, 3, '{noun}', 8),
    (2, 4, '{noun}', 9);

-- Insert Italian definitions for application. (id=10, 11, 12)
INSERT INTO entries (lang, content) VALUES
    ('italian', 'le applicazione'),
    ('italian', 'la domanda'),
    ('italian', 'la richiesta');
-- Insert Italian application-definition relationships.
INSERT INTO relations (from_id, to_id, types, weight) VALUES
    (2, 10, '{noun}', 0),
    (2, 11, '{noun}', 1),
    (2, 12, '{noun}', 1);
```
