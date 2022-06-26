dictpress comes with a built in CSV to database importer tool. Once dictionary data has been organised into
the below described structure, import it by running `./dictpress --import=yourfile.csv`.

Entries with the same content in the same language are not inserted into the database multiple times, but are instead re-used.
For instance, if there are multiple `Apple (English)` entries, it is inserted once but re-used in multiple relations.

## Sample CSV format
```csv
-,A,Apple,english,Optional note,english,"",optional-tag1|tag2,"ˈæp.əl|aapl",""
^,"","round, red or yellow, edible fruit of a small tree",english,"","","","","",noun
^,"","the tree, cultivated in most temperate regions.",english,"","","","","",noun
^,"","il pomo.",italian,"","","","","",noun
-,A,Application,english,Optional note,italian,"","","aplɪˈkeɪʃ(ə)n",""
^,"","the act of putting to a special use or purpose",english,"","","","","",noun
^,"","le applicazione",italian,"","","","","",noun

```

Every line in the CSV file contains an entry in a given language described in 10 columns.
Each entry is either a main entry in the dictionary, or a definition of another entry.
This is indicated by the first column in each line. `-` represents a main entry and all subsequent
entries below it marked with `^` represents its definitions in one or more languages.

The above example shows two main English entries, "Apple" and "Application" with multiple
English and Italian definitions below them.

## CSV fields

| Column | Field            |                                                                                                                                                                                                                                                                                                                                                       |
|--------|------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| 0      | type             | `-` represents a main entry. `^` under it represents a definition entry.                                                                                                                                                                                                                                                                              |
| 1      | content          | The entry content (word or phrase).                                                                                                                                                                                                                                                                                                                    |
| 2      | initial          | The uppercase first character of the entry. Eg: `A` for Apple. If left empty, it is automatically picked up.                                                                                                                                                                                                                                                                                        |
| 3      | language         | Language of the entry (as defined in the config).                                                                                                                                                                                                                                                                                                      |
| 4      | notes            | Optional notes describing the entry.                                                                                                                                                                                                                                                                                                                          |
| 5      | tsvector_language | If the language has a built in Postgres fulltext tokenizer, the name of the tokenizer language. For languages that do not have Postgres tokenizers, this should be empty.                                                                                                                                                                             |
| 6      | tsVector_tokens   | Postgres fulltext search tokens for the entry (Content). If `tsvector_language` is specified, this field can be left empty as the tokens are automatically created in the database using `TO_TSVECTOR($tsvector_language, $content)`. For languages without Postgres tokenizers, the [tsvector](https://www.postgresql.org/docs/10/datatype-textsearch.html#DATATYPE-TSVECTOR) token string should be computed externally and provided here. |
| 7      | tags             | Optional tags describing the entry. Separate multiple tags by `\|`.                                                                                                                                                                                                                                                                                   |
| 8      | phones           | Optional phonetic notations representing the pronunciations of the entry. Separate multiple phones by `\|`.                                                                                                                                                                                                                                           |
| 9      | definition-types | This should only be set for definition entries that ar marked with `Type = ^`. One or more parts-of-speech types separated by `\|`. Example `noun\|verb`.                                                                                                                                                                                             |
