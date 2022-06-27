# Installation

dictpress requires Postgres ⩾ v10.

## Binary
- Download the [latest release](https://github.com/knadh/dictpress/releases) and extract the binary.
- `./dictpress --new-config` to generate config.toml. Then, edit the file.
- `./dictpress --install` to install the tables in the Postgres DB.
- Run `./dictpress` and visit `http://localhost:9000/admin`.

See [Importing data](import.md) to populate the dictionary database from CSVs.


## Compiling from source

To compile the latest unreleased version (`master` branch):

1. Make sure `go`, `nodejs`, and `yarn` are installed on your system.
2. `git clone git@github.com:knadh/dictpress.git`
3. `cd dictpress && make dist`. This will generate the `dictpress` binary.
