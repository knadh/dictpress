# Contributing

To compile the app on your own local system:

0. Make sure you have `go` installed.
1. Clone this repository on to your computer.
2. Switch to the folder.
3. Run `make`
4. `./dictpress` is now available
5. Follow the rest of "Installation" instruction in README.md

To configure the `[db]` section of `config.toml`, you will need a postgres instance running on your computer.

## Setting up a test database

1. Install postgresql.
2. `createuser` and `createdb`. Fill config.toml with these details.
3. `psql -Upostgres dictpress -f sample/sample.sql`

## To run unit tests

1. Prerequisite docker
2.  Run `make test`
