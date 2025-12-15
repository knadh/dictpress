use std::path::Path;

use regex::Regex;
use sqlx::Row;

use crate::{
    db,
    models::{LangMap, STATUS_ENABLED},
    tokenizer::{parse_tokenizer_field, Tokenizers},
};

const INSERT_BATCH_SIZE: usize = 5000;
const COL_COUNT: usize = 11;

const TYPE_ENTRY: &str = "-";
const TYPE_DEF: &str = "^";

#[derive(Debug, thiserror::Error)]
pub enum ImportError {
    #[error("csv error: {0}")]
    Csv(#[from] csv::Error),
    #[error("database error: {0}")]
    Db(#[from] sqlx::Error),
    #[error("io error: {0}")]
    Io(#[from] std::io::Error),
    #[error("tokenizer error: {0}")]
    Tokenizer(#[from] crate::tokenizer::TokenizerError),
    #[error("{0}")]
    Validation(String),
}

/// Entry row from CSV.
struct Entry {
    entry_type: String,     // 0: "-" for entry, "^" for definition.
    initial: String,        // 1
    content: String,        // 2
    lang: String,           // 3
    notes: String,          // 4
    tokenizer: String, // 5: "default:name" or "lua:filename.lua" (or if empty, use tokens as-is from the next field)
    tokens: String,    // 6
    tags: Vec<String>, // 7
    phones: Vec<String>, // 8
    def_types: Vec<String>, // 9: Only for definition entries.
    meta: String,      // 10

    definitions: Vec<Entry>,
}

/// Import a CSV file into the database.
pub async fn import_csv(
    file_path: &Path,
    db_path: &str,
    tokenizers: &Tokenizers,
    langs: LangMap,
) -> Result<(), ImportError> {
    let db = db::init(db_path, 1, false).await?;

    log::info!("importing data from {} ...", file_path.display());

    let file = std::fs::File::open(file_path)?;
    let mut reader = csv::ReaderBuilder::new()
        .has_headers(false)
        .flexible(true)
        .from_reader(file);

    let re_spaces = Regex::new(r"\s+").unwrap();

    let mut entries: Vec<Entry> = Vec::new();
    let mut n = 0;
    let mut num_main = 0;
    let mut num_defs = 0;

    for result in reader.records() {
        let record = result?;

        if n == 0 && record.get(0) != Some(TYPE_ENTRY) {
            return Err(ImportError::Validation(
                "line 1: first row should be of type '-'".to_string(),
            ));
        }
        n += 1;

        let entry = read_entry(&record, n, &langs, tokenizers, &re_spaces)?;

        // First entry is always a main entry.
        if entries.is_empty() {
            entries.push(entry);
            continue;
        }

        // Add definitions to last main entry.
        if entry.entry_type == TYPE_DEF {
            if let Some(last) = entries.last_mut() {
                last.definitions.push(entry);
                num_defs += 1;
            }
            continue;
        }

        // Insert batch when reaching size limit.
        if entries.len().is_multiple_of(INSERT_BATCH_SIZE) {
            insert_entries(&db, &entries, num_main).await?;
            num_main += entries.len();
            entries.clear();
            log::info!("imported {} entries and {} definitions", num_main, num_defs);
        }

        // New main entry.
        entries.push(entry);
    }

    // Flush any remaining entries.
    if !entries.is_empty() {
        insert_entries(&db, &entries, num_main).await?;
    }

    log::info!(
        "finished. imported {} entries and {} definitions",
        num_main + entries.len(),
        num_defs
    );

    Ok(())
}

fn read_entry(
    record: &csv::StringRecord,
    line: usize,
    langs: &LangMap,
    tokenizers: &Tokenizers,
    re_spaces: &Regex,
) -> Result<Entry, ImportError> {
    let get = |i: usize| record.get(i).unwrap_or("").to_string();

    let entry_type = clean_string(&get(0), re_spaces);
    if entry_type != TYPE_ENTRY && entry_type != TYPE_DEF {
        return Err(ImportError::Validation(format!(
            "line {}: unknown type '{}' in column 0. Should be '-' or '^'",
            line, entry_type
        )));
    }

    let mut entry = Entry {
        entry_type: entry_type.clone(),
        initial: clean_string(&get(1), re_spaces),
        content: clean_string(&get(2), re_spaces),
        lang: clean_string(&get(3), re_spaces),
        notes: clean_string(&get(4), re_spaces),
        tokenizer: clean_string(&get(5), re_spaces),
        tokens: clean_string(&get(6), re_spaces),
        tags: split_string(&clean_string(&get(7), re_spaces)),
        phones: split_string(&clean_string(&get(8), re_spaces)),
        def_types: Vec::new(),
        meta: get(10).trim().to_string(),
        definitions: Vec::new(),
    };

    if record.len() != COL_COUNT {
        return Err(ImportError::Validation(format!(
            "line {}: every line should have exactly {} columns. Found {}",
            line,
            COL_COUNT,
            record.len()
        )));
    }

    let lang = langs.get(&entry.lang).ok_or_else(|| {
        ImportError::Validation(format!(
            "line {}: unknown language '{}' at column 3",
            line, entry.lang
        ))
    })?;

    if entry.content.is_empty() {
        return Err(ImportError::Validation(format!(
            "line {}: empty content (word) at column 2",
            line
        )));
    }

    // Set initial from first character if not provided.
    if entry.initial.is_empty() {
        entry.initial = entry
            .content
            .chars()
            .next()
            .map(|c| c.to_uppercase().to_string())
            .unwrap_or_default();
    }

    // Generate tokens based on the tokenizer specified.
    if let Some(tk_key) = parse_tokenizer_field(&entry.tokenizer) {
        match tokenizers.get(&tk_key) {
            Some(tk) => match tk.tokenize(&entry.content, &lang.id) {
                Ok(tokens) => entry.tokens = tokens.join(" "),
                Err(e) => log::warn!(
                    "line {}: tokenizer '{}' failed for content '{}': {}",
                    line,
                    entry.tokenizer,
                    entry.content,
                    e
                ),
            },
            None => {
                log::warn!("line {}: tokenizer '{}' not found", line, entry.tokenizer);
            }
        }
    }
    // If tokenizer field is empty, entry.tokens is used as-is (may be empty or pre-provided)

    // Parse definition types.
    let def_type_str = clean_string(&get(9), re_spaces);
    if entry_type == TYPE_DEF {
        let def_types = split_string(&def_type_str);
        for t in &def_types {
            if !lang.types.contains_key(t) {
                log::warn!(
                    "line {}: unknown type '{}' for language '{}'",
                    line,
                    t,
                    entry.lang,
                )
            }
        }
        entry.def_types = def_types;
    } else if !def_type_str.is_empty() {
        return Err(ImportError::Validation(format!(
            "line {}: column 9 (definition type) should only be set for definition entries (^)",
            line
        )));
    }

    // Validate meta JSON.
    if entry.meta.is_empty() {
        entry.meta = "{}".to_string();
    } else {
        match serde_json::from_str::<serde_json::Value>(&entry.meta) {
            Ok(v) => {
                if !v.is_object() {
                    return Err(ImportError::Validation(format!(
                        "line {}: column 10, meta should be a JSON object",
                        line
                    )));
                }
            }
            Err(e) => {
                return Err(ImportError::Validation(format!(
                    "line {}: column 10, invalid JSON: {}",
                    line, e
                )));
            }
        }
    }

    Ok(entry)
}

async fn insert_entries(
    db: &sqlx::SqlitePool,
    entries: &[Entry],
    line_start: usize,
) -> Result<(), ImportError> {
    // Insert main entries.
    let mut ids: Vec<i64> = Vec::with_capacity(entries.len());

    for (i, e) in entries.iter().enumerate() {
        let guid = uuid::Uuid::new_v4().to_string();

        // Encode text arrays to JSON for SQLite.
        let content = serde_json::to_string(&[&e.content]).unwrap_or_else(|_| "[]".to_string());
        let tags = serde_json::to_string(&e.tags).unwrap_or_else(|_| "[]".to_string());
        let phones = serde_json::to_string(&e.phones).unwrap_or_else(|_| "[]".to_string());

        let row = sqlx::query(
            r#"INSERT INTO entries (guid, content, initial, weight, tokens, lang, tags, phones, notes, meta, status)
               VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
               RETURNING id"#,
        )
        .bind(&guid)
        .bind(&content)
        .bind(&e.initial)
        .bind((line_start + i) as i32)
        .bind(&e.tokens)
        .bind(&e.lang)
        .bind(&tags)
        .bind(&phones)
        .bind(&e.notes)
        .bind(&e.meta)
        .bind(STATUS_ENABLED)
        .fetch_one(db)
        .await?;

        ids.push(row.get(0));
    }

    // Insert definition entries and create relations.
    for (i, main_entry) in entries.iter().enumerate() {
        let from_id = ids[i];

        for (j, def) in main_entry.definitions.iter().enumerate() {
            // Insert definition entry.
            let guid = uuid::Uuid::new_v4().to_string();
            let content_json =
                serde_json::to_string(&[&def.content]).unwrap_or_else(|_| "[]".to_string());
            let phones_json =
                serde_json::to_string(&def.phones).unwrap_or_else(|_| "[]".to_string());

            let row = sqlx::query(
                r#"INSERT INTO entries (guid, content, initial, weight, tokens, lang, tags, phones, notes, meta, status)
                   VALUES (?, ?, ?, ?, ?, ?, '[]', ?, '', ?, ?)
                   RETURNING id"#,
            )
            .bind(&guid)
            .bind(&content_json)
            .bind(&def.initial)
            .bind(j as i32)
            .bind(&def.tokens)
            .bind(&def.lang)
            .bind(&phones_json)
            .bind(&def.meta)
            .bind(STATUS_ENABLED)
            .fetch_one(db)
            .await?;

            let to_id: i64 = row.get(0);

            // Create relation.
            let types_json =
                serde_json::to_string(&def.def_types).unwrap_or_else(|_| "[]".to_string());
            let tags_json = serde_json::to_string(&def.tags).unwrap_or_else(|_| "[]".to_string());

            sqlx::query(
                r#"INSERT INTO relations (from_id, to_id, types, tags, notes, weight, status)
                   VALUES (?, ?, ?, ?, ?, ?, ?)"#,
            )
            .bind(from_id)
            .bind(to_id)
            .bind(&types_json)
            .bind(&tags_json)
            .bind(&def.notes)
            .bind(j as i32)
            .bind(STATUS_ENABLED)
            .execute(db)
            .await?;
        }
    }

    Ok(())
}

fn clean_string(s: &str, re_spaces: &Regex) -> String {
    re_spaces.replace_all(s.trim(), " ").to_string()
}

fn split_string(s: &str) -> Vec<String> {
    s.split('|')
        .map(|s| s.trim().to_string())
        .filter(|s| !s.is_empty())
        .collect()
}
