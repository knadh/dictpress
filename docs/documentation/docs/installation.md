# Installation

dictpress requires Postgres ⩾ v10.

## Pre-built binary
- Download the [latest release](https://github.com/knadh/dictpress/releases) and extract the binary.
- `./dictpress new-config` to generate config.toml. Then, edit the file.
- `./dictpress install` to install the tables in the Postgres DB.
- Run `./dictpress` and visit `http://localhost:9000/admin`.

See [Importing data](import.md) to populate the dictionary database from CSVs.

## Compiling from source

Make sure `go` is installed on your system.

1. Download or `git clone` the latest tagged release or the bleeding edge `master` branch from the [repository](https://github.com/knadh/dictpress).
Eg: `git clone git@github.com:knadh/dictpress.git`

1. `cd` into the dictpress directory and run `make dist`. This compiles the `dictpress` binary.

## Upgrading
Some releases may require schema changes to the existing database. If a release prompts for an upgrade, run `dictpress upgrade` to apply the necessary database migrations.

