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
    p = argparse.ArgumentParser(description="Migrate dictpress data from PostgreSQL to SQLite")
    p.add_argument("--config", required=True, help="Path to Go dictpress config.toml")
    p.add_argument("--sqlite-db", required=True, help="Output SQLite database path (must not exist)")
    p.add_argument("--sqlite-schema", required=True, help="Path to SQLite schema file")
    return p.parse_args()


def load_pg_config(config_path: str) -> dict:
    """Load PostgreSQL connection params from config.toml."""
    cfg = toml.load(config_path)
    db = cfg.get("db", {})
    return {
        "host": db.get("host", "localhost"),
        "port": db.get("port", 5432),
        "dbname": db.get("db", ""),
        "user": db.get("user", ""),
        "password": db.get("password", ""),
    }


def pg_array_to_json(val) -> str:
    """Convert PostgreSQL array to JSON string."""
    if val is None:
        return "[]"
    return json.dumps(list(val), ensure_ascii=False)


def tsvector_to_text(val) -> str:
    """Extract lexemes from tsvector as space-separated string."""
    if not val:
        return ""
    # tsvector comes as string like "'word1':1 'word2':2" - extract just the lexemes
    tokens = []
    for part in val.split():
        if part.startswith("'") and ":" in part:
            lexeme = part.split(":")[0].strip("'")
            tokens.append(lexeme)
    return " ".join(tokens)


def ts_to_iso(val) -> str:
    """Convert timestamp to ISO string."""
    if val is None:
        return None
    return val.isoformat().replace("+00:00", "Z")


def migrate(pg_cfg: dict, sqlite_path: str, schema_path: str):
    """Run the migration."""
    import sqlite3

    # Connect to PostgreSQL
    print(f"Connecting to PostgreSQL at {pg_cfg['host']}:{pg_cfg['port']}/{pg_cfg['dbname']}...")
    pg = psycopg2.connect(**pg_cfg)
    pg_cur = pg.cursor(cursor_factory=psycopg2.extras.DictCursor)

    # Create SQLite database and apply schema
    print(f"Creating SQLite database: {sqlite_path}")
    sq = sqlite3.connect(sqlite_path)
    sq_cur = sq.cursor()

    with open(schema_path) as f:
        schema = f.read()
    # Split by "-- name:" comments and execute each block
    for block in schema.split("-- name:"):
        block = block.strip()
        if block:
            # Skip the name line, execute the rest
            lines = block.split("\n", 1)
            if len(lines) > 1:
                sq_cur.executescript(lines[1])
    sq.commit()

    # Migrate entries in batches
    print("Migrating entries...")
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
                pg_array_to_json(r["content"]),
                r["initial"] or "",
                float(r["weight"] or 0),
                tsvector_to_text(r["tokens"]),
                r["lang"],
                pg_array_to_json(r["tags"]),
                pg_array_to_json(r["phones"]),
                r["notes"] or "",
                json.dumps(r["meta"], ensure_ascii=False) if r["meta"] else "{}",
                r["status"],
                ts_to_iso(r["created_at"]),
                ts_to_iso(r["updated_at"]),
            ))

        sq_cur.executemany(
            """INSERT INTO entries (id, guid, content, initial, weight, tokens, lang,
                                    tags, phones, notes, meta, status, created_at, updated_at)
               VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)""",
            entries,
        )
        sq.commit()

        last_id = rows[-1]["id"]
        total_entries += len(rows)
        print(f"  entries: {total_entries} (last_id={last_id})")

    print(f"Migrated {total_entries} entries")

    # Migrate relations in batches
    print("Migrating relations...")
    last_id = 0
    total_relations = 0

    while True:
        pg_cur.execute(
            """SELECT id, from_id, to_id, types, tags, notes, weight,
                      status, created_at, updated_at
               FROM relations
               WHERE id > %s
               ORDER BY id
               LIMIT %s""",
            (last_id, BATCH_SIZE),
        )
        rows = pg_cur.fetchall()
        if not rows:
            break

        relations = []
        for r in rows:
            relations.append((
                r["id"],
                r["from_id"],
                r["to_id"],
                pg_array_to_json(r["types"]),
                pg_array_to_json(r["tags"]),
                r["notes"] or "",
                float(r["weight"] or 0),
                r["status"],
                ts_to_iso(r["created_at"]),
                ts_to_iso(r["updated_at"]),
            ))

        sq_cur.executemany(
            """INSERT INTO relations (id, from_id, to_id, types, tags, notes, weight,
                                      status, created_at, updated_at)
               VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)""",
            relations,
        )
        sq.commit()

        last_id = rows[-1]["id"]
        total_relations += len(rows)
        print(f"  relations: {total_relations} (last_id={last_id})")

    print(f"Migrated {total_relations} relations")

    pg_cur.close()
    pg.close()
    sq.close()
    print("Done.")


def main():
    args = parse_args()

    if not os.path.exists(args.config):
        sys.exit(f"Config file not found: {args.config}")

    if os.path.exists(args.sqlite_db):
        sys.exit(f"SQLite database already exists: {args.sqlite_db}")

    if not os.path.exists(args.sqlite_schema):
        sys.exit(f"Schema file not found: {args.sqlite_schema}")

    pg_cfg = load_pg_config(args.config)
    migrate(pg_cfg, args.sqlite_db, args.sqlite_schema)


if __name__ == "__main__":
    main()
