# Installation

dictpress is a single binary app and has no dependencies.

## Pre-built binary
- Download the [latest release](https://github.com/knadh/dictpress/releases) and extract the binary.
- `./dictpress new-config` to generate config.toml. Then, edit the file.
- `./dictpress --db=data.db install` to create a new SQLite database file with the necessary schema.
- Run `./dictpress --db=data.db` and visit `http://localhost:9000/admin`. This runs the app in HTTP/JSON API mode.

See [Importing data](import.md) to populate the dictionary database from CSVs.

## Upgrading
Some releases may require schema changes to the existing database. If a release prompts for an upgrade, run `./dictpress upgrade` to apply the necessary database migrations.

## Migration from v4x to v5x
dictpress was completely rewritten from Go to Rust in v5. v4x was the last Go (with the Postgres dependency) version. If you have a Postgres dictionary database running dictpress v4 or an older version, it is possible to migrate to v5 (Rust, SQLite database file) or above.

> If you are not running the latest of the v4x Go version, download that from the [releases](https://github.com/knadh/dictpress/releases) page and upgrade your Postgres database schema to the latest version by replacing the old binary and running `./dictpress upgrade`

- Recommended: Use [uv](https://docs.astral.sh/uv/) to run the Python script.
- Download [migrate-db.py](https://github.com/knadh/dictpress/tree/master/scripts)
- Download [schema.sql](https://github.com/knadh/dictpress/blob/master/static/sql/schema.sql)
- Run `python migrade-db.py --config=/path/to/your/old/dictpress/config.toml --sqlite-db=./data.db --sqlite-schema=./schema.sql`

The script will connect the old Postgres dictpress database by reading the `[db]` config from config.toml, create a new SQLite database `./data.db` with the latest schema, and migrate all data from the Postgres database to the new SQLite database file.

