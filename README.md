# dictpress

dictpress is a free and open source, single binary webserver application for building and publishing fast, searchable dictionaries for any language. It stores all data in SQLite DB files and has no dependencies.

Example dictionaries:
- [Alar](https://alar.ink/) — Kannada-English dictionary.
- [Olam](https://olam.in/) — English-Malayalam, Malayalam-Malayalam dictionary.
- [Sourashtra Dictionary](https://dictionary.thinnal.org/) - Sourashtra English, Sourashtra Tamil dictionary.


## Features
- Build dictionaries for any language to any language.
- Supports multiple dictionaries and languages in the same database.
- Custom themes and templates for publishing dictionary websites.
- Paginated A-Z (all alphabets for any language) glossaries.
- HTTP/JSON API for search and everything else.
- Pluggable search tokenizers (Lua scripts) and algorithms for fulltext search, phonetic search etc.
- Admin UI for managing and curating dictionary data.
- Admin moderation UI for crowd sourcing dictionary entries.
- Bulk CSV to database import.

[![image](https://user-images.githubusercontent.com/547147/175945746-575c2cb7-7478-414a-93ae-014196d3385d.png)](https://olam.in)
[![image](https://user-images.githubusercontent.com/547147/175945847-40d3ae1c-c81a-4283-94af-9299476bfd7f.png)](https://dict.press/static/admin.png)

## Getting started
- [Download](https://github.com/knadh/dictpress/releases) the latest version.
- [Read the docs](https://dict.press) for setup and usage instructions.

### Important: v4 -> v5
dictpress until v4.x was written in Go and depended on a Postgres database. dictpress v5 is a complete rewrite in Rust and uses SQLite instead.
