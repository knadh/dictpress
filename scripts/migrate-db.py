#!/usr/bin/env python3
"""Migrate Go dictpress PostgreSQL data to Rust dictpress SQLite."""

import argparse
import json
import os
import sys

import psycopg2
import psycopg2.extras
import toml

BATCH_SIZE = 5000


def parse_args():
    p = argparse.ArgumentParser(
        description="Migrate Go dictpress (v4) data from PostgreSQL to Rust dictpress SQLite")
    p.add_argument("--config", required=True,
                   help="Path to the old Go dictpress config.toml")
    p.add_argument("--sqlite-db", required=True,
                   help="Path to the new output SQLite database path (must not exist)")
    p.add_argument("--sqlite-schema", required=True,
                   help="Path to the SQLite schema file (copy from static/sql/schema.sql from the Rust dictpress repo)")
    return p.parse_args()


def _load_pg_config(config_path: str) -> dict:
    """Get PostgreSQL config"""
    cfg = toml.load(config_path)
    db = cfg.get("db", {})

    return {
        "host": db.get("host", "localhost"),
        "port": db.get("port", 5432),
        "dbname": db.get("db", ""),
        "user": db.get("user", ""),
        "password": db.get("password", ""),
    }


def _pg_array_to_json(val) -> str:
    """Convert PostgreSQL array to JSON string."""
    if val is None:
        return "[]"
    return json.dumps(list(val), ensure_ascii=False)


def _tsvector_to_text(val) -> str:
    """Extract tokens from tsvector as a space separated string."""
    if not val:
        return ""

    # tsvector comes as string like "'word1':1 'word2':2" - extract just the tokens/lexemes.
    tokens = []
    for part in val.split():
        if part.startswith("'") and ":" in part:
            lexeme = part.split(":")[0].strip("'")
            tokens.append(lexeme)

    return " ".join(tokens)


def _timestamp_to_iso(val) -> str:
    """Convert timestamp to ISO string."""
    if val is None:
        return None
    return val.isoformat().replace("+00:00", "Z")


def _migrate(pg_cfg: dict, sqlite_path: str, schema_path: str):
    """Run the Postgres to SQLite3 migration."""
    import sqlite3

    # Connect to PostgreSQL
    print(
        f"connecting to PostgreSQL at {pg_cfg['host']}:{pg_cfg['port']}/{pg_cfg['dbname']}...")
    pg = psycopg2.connect(**pg_cfg)
    pg_cur = pg.cursor(cursor_factory=psycopg2.extras.DictCursor)

    # Create SQLite database and apply schema
    print(f"creating new SQLite database '{sqlite_path}'")
    sq = sqlite3.connect(sqlite_path)
    sq_cur = sq.cursor()

    with open(schema_path) as f:
        schema = f.read()

    # Split by "-- name:" comments (yesql structure) and execute each block.
    for block in schema.split("-- name:"):
        block = block.strip()
        if block:
            # Skip the name line, execute the rest.
            lines = block.split("\n", 1)
            if len(lines) > 1:
                sq_cur.executescript(lines[1])
    sq.commit()

    # Migrate entries in batches.
    print("migrating entries")
    last_id = 0
    total_entries = 0

    while True:
        pg_cur.execute(
            """SELECT id, guid, content, initial, weight, tokens, lang, tags, phones,
                      notes, meta, status, created_at, updated_at
               FROM entries
               WHERE id > %s
               ORDER BY id
               LIMIT %s""",
            (last_id, BATCH_SIZE),
        )

        rows = pg_cur.fetchall()
        if not rows:
            break

        entries = []
        for r in rows:
            entries.append((
                r["id"],
                str(r["guid"]),
                _pg_array_to_json(r["content"]),
                r["initial"] or "",
                float(r["weight"] or 0),
                _tsvector_to_text(r["tokens"]),
                r["lang"],
                _pg_array_to_json(r["tags"]),
                _pg_array_to_json(r["phones"]),
                r["notes"] or "",
                json.dumps(
                    r["meta"], ensure_ascii=False) if r["meta"] else "{}",
                r["status"],
                _timestamp_to_iso(r["created_at"]),
                _timestamp_to_iso(r["updated_at"]),
            ))

        sq_cur.executemany(
            """INSERT INTO entries (id, guid, content, initial, weight, tokens, lang, tags, phones, notes, meta, status, created_at, updated_at)
               VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)""",
            entries,
        )
        sq.commit()

        last_id = rows[-1]["id"]
        total_entries += len(rows)

        print(f"  entries: {total_entries} (last_id={last_id})")

    print(f"migrated {total_entries} entries")

    # Migrate relations in batches
    print("migrating relations (defs)")
    last_id = 0
    total_rels = 0

    while True:
        pg_cur.execute(
            """SELECT id, from_id, to_id, types, tags, notes, weight, status, created_at, updated_at
               FROM relations
               WHERE id > %s ORDER BY id LIMIT %s""",
            (last_id, BATCH_SIZE),
        )
        rows = pg_cur.fetchall()
        if not rows:
            break

        rels = []
        for r in rows:
            rels.append((
                r["id"],
                r["from_id"],
                r["to_id"],
                _pg_array_to_json(r["types"]),
                _pg_array_to_json(r["tags"]),
                r["notes"] or "",
                float(r["weight"] or 0),
                r["status"],
                _timestamp_to_iso(r["created_at"]),
                _timestamp_to_iso(r["updated_at"]),
            ))

        sq_cur.executemany(
            """INSERT INTO relations (id, from_id, to_id, types, tags, notes, weight, status, created_at, updated_at)
               VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)""",
            rels,
        )
        sq.commit()

        last_id = rows[-1]["id"]
        total_rels += len(rows)
        print(f"  relations: {total_rels} (last_id={last_id})")

    print(f"migrated {total_rels} relations")

    pg_cur.close()
    pg.close()
    sq.close()

    print("finished")


def main():
    args = parse_args()

    if not os.path.exists(args.config):
        sys.exit(f"Config file not found: {args.config}")

    if os.path.exists(args.sqlite_db):
        sys.exit(f"SQLite database already exists: {args.sqlite_db}")

    if not os.path.exists(args.sqlite_schema):
        sys.exit(f"Schema file not found: {args.sqlite_schema}")

    pg_cfg = _load_pg_config(args.config)
    _migrate(pg_cfg, args.sqlite_db, args.sqlite_schema)


if __name__ == "__main__":
    main()
