use std::{collections::HashMap, sync::Arc};

use sqlx::{sqlite::SqlitePool, Row};

use crate::{
    models::{
        q, Comment, Dicts, Entry, GlossaryWord, LangMap, Relation, RelationsQuery, SearchQuery,
        Stats, STATUS_ENABLED,
    },
    tokenizer::{Tokenizer, TokenizerError, Tokenizers},
};

#[derive(Debug, thiserror::Error)]
pub enum Error {
    #[error("database error: {0}")]
    Db(#[from] sqlx::Error),
    #[error("tokenizer error: {0}")]
    Tokenizer(#[from] TokenizerError),
    #[error("unknown language: {0}")]
    UnknownLang(String),
    #[error("not found")]
    NotFound,
    #[error("{0}")]
    Validation(String),
}

/// Manager handles all database operations and business logic.
pub struct Manager {
    db: SqlitePool,
    tokenizers: Tokenizers,
    pub langs: LangMap,
    pub dicts: Dicts,
}

impl Manager {
    pub async fn new(
        db: SqlitePool,
        tokenizers: Tokenizers,
        langs: LangMap,
        dicts: Dicts,
    ) -> Result<Self, Error> {
        Ok(Self {
            db,
            tokenizers,
            langs,
            dicts,
        })
    }

    /// Get tokenizer for a language.
    fn get_tokenizer(&self, lang_id: &str) -> Option<&Arc<dyn Tokenizer>> {
        let lang = self.langs.get(lang_id)?;
        self.tokenizers.get(&lang.tokenizer)
    }

    /// Tokenize content for a given language.
    pub fn tokenize(&self, content: &[String], lang_id: &str) -> Result<String, Error> {
        let text = content.join(" ");
        if let Some(tk) = self.get_tokenizer(lang_id) {
            let tokens = tk.tokenize(&text, lang_id)?;
            Ok(tokens.join(" "))
        } else {
            // Fallback to simple whitespace tokenization.
            Ok(text.to_lowercase())
        }
    }

    /// Convert search query to FTS5 query string.
    pub fn to_fts_query(&self, query: &str, lang_id: &str) -> Result<String, Error> {
        let tk = self
            .get_tokenizer(lang_id)
            .ok_or_else(|| Error::UnknownLang(lang_id.to_string()))?;

        Ok(tk.to_query(query, lang_id)?)
    }

    // #########################
    // Search

    /// Search entries based on a search query.
    pub async fn search(
        &self,
        sq: &SearchQuery,
        offset: i32,
        limit: i32,
    ) -> Result<(Vec<Entry>, i64), Error> {
        if !self.langs.contains_key(&sq.from_lang) {
            return Err(Error::UnknownLang(sq.from_lang.clone()));
        }

        // Generate FTS query.
        let fts_query = self.to_fts_query(&sq.query, &sq.from_lang)?;

        // If FTS query is empty, return an error.
        if fts_query.trim().is_empty() {
            return Err(Error::Validation("invalid search query".to_string()));
        }

        let status = if sq.status.is_empty() {
            STATUS_ENABLED.to_string()
        } else {
            sq.status.clone()
        };

        let results: Vec<Entry> = sqlx::query_as(&q.search.query)
            .bind(&sq.from_lang)
            .bind(&sq.query)
            .bind(&fts_query)
            .bind(&status)
            .bind(offset)
            .bind(limit)
            .fetch_all(&self.db)
            .await?;

        let total = results.first().map(|e| e.total).unwrap_or(0);
        Ok((results, total))
    }

    /// Load relations for a set of entries.
    /// rel_query.max_per_type: 0 = load all relations, >0 = limit relations per type per entry.
    /// rel_query.max_content_items: 0 = no truncation, >0 = truncate content array.
    pub async fn load_relations(
        &self,
        entries: &mut [Entry],
        rel_query: &RelationsQuery,
    ) -> Result<(), Error> {
        if entries.is_empty() {
            return Ok(());
        }

        // Build ID list and index map.
        let ids: Vec<i64> = entries.iter().map(|e| e.id).collect();

        entries.iter_mut().for_each(|e| e.relations = Vec::new());
        let id_map: HashMap<i64, usize> =
            entries.iter().enumerate().map(|(i, e)| (e.id, i)).collect();

        // Serialize array types for SQLite query.
        let id_json = serde_json::to_string(&ids).unwrap_or_default();
        let types_json = serde_json::to_string(&rel_query.types).unwrap_or_default();
        let tags_json = serde_json::to_string(&rel_query.tags).unwrap_or_default();

        let rel_entries: Vec<Entry> = sqlx::query_as(&q.search_relations.query)
            .bind(&id_json)
            .bind(&rel_query.to_lang)
            .bind(&types_json)
            .bind(&tags_json)
            .bind(&rel_query.status)
            .bind(rel_query.max_per_type)
            .bind(rel_query.max_content_items)
            .fetch_all(&self.db)
            .await?;

        // Attach relations to their parent entries.
        for mut r in rel_entries {
            // Build Relation struct from flat fields.
            r.relation = Some(Relation {
                id: r.relation_id,
                types: std::mem::take(&mut r.relation_types),
                tags: std::mem::take(&mut r.relation_tags),
                notes: std::mem::take(&mut r.relation_notes),
                weight: r.relation_weight,
                status: std::mem::take(&mut r.relation_status),
                created_at: r.relation_created_at.take(),
                updated_at: r.relation_updated_at.take(),
            });

            if let Some(&idx) = id_map.get(&r.from_id) {
                let entry = &mut entries[idx];
                if entry.relations.is_empty() {
                    entry.total_relations = r.total_relations;
                }
                entry.relations.push(r);
            }
        }

        Ok(())
    }

    // #########################
    // Entry CRUD

    /// Get an entry by ID or GUID.
    pub async fn get_entry(&self, id: i64, guid: &str) -> Result<Entry, Error> {
        let entry: Entry = sqlx::query_as(&q.get_entry.query)
            .bind(id)
            .bind(guid)
            .fetch_optional(&self.db)
            .await?
            .ok_or(Error::NotFound)?;
        Ok(entry)
    }

    /// Get parent entries for a given entry ID.
    pub async fn get_parent_entries(&self, id: i64) -> Result<Vec<Entry>, Error> {
        let entries: Vec<Entry> = sqlx::query_as(&q.get_parent_relations.query)
            .bind(id)
            .fetch_all(&self.db)
            .await?;
        Ok(entries)
    }

    /// Insert a new entry into the database.
    pub async fn insert_entry(&self, e: &Entry) -> Result<i64, Error> {
        if !self.langs.contains_key(&e.lang) {
            return Err(Error::UnknownLang(e.lang.clone()));
        }

        // Generate tokens if not provided.
        let tokens = if e.tokens.is_empty() {
            self.tokenize(&e.content.0, &e.lang)?
        } else {
            e.tokens.clone()
        };

        let guid = if e.guid.is_empty() {
            uuid::Uuid::new_v4().to_string()
        } else {
            e.guid.clone()
        };

        let status = if e.status.is_empty() {
            STATUS_ENABLED.to_string()
        } else {
            e.status.clone()
        };

        let content_json = serde_json::to_string(&e.content.0).unwrap_or_else(|_| "[]".to_string());
        let tags_json = serde_json::to_string(&e.tags.0).unwrap_or_else(|_| "[]".to_string());
        let phones_json = serde_json::to_string(&e.phones.0).unwrap_or_else(|_| "[]".to_string());
        let meta_json = serde_json::to_string(&e.meta).unwrap_or_else(|_| "{}".to_string());

        let row = sqlx::query(&q.insert_entry.query)
            .bind(&guid)
            .bind(&content_json)
            .bind(&e.initial)
            .bind(e.weight)
            .bind(&tokens)
            .bind(&e.lang)
            .bind(&tags_json)
            .bind(&phones_json)
            .bind(&e.notes)
            .bind(&meta_json)
            .bind(&status)
            .fetch_one(&self.db)
            .await?;

        Ok(row.get(0))
    }

    /// Update an existing entry in the database.
    pub async fn update_entry(&self, id: i64, e: &Entry) -> Result<(), Error> {
        // Regenerate tokens if content changed.
        let tokens = if !e.content.is_empty() && e.tokens.is_empty() {
            self.tokenize(&e.content.0, &e.lang)?
        } else {
            e.tokens.clone()
        };

        let content_json = serde_json::to_string(&e.content.0).unwrap_or_else(|_| "[]".to_string());
        let tags_json = serde_json::to_string(&e.tags.0).unwrap_or_else(|_| "[]".to_string());
        let phones_json = serde_json::to_string(&e.phones.0).unwrap_or_else(|_| "[]".to_string());
        let meta_json = serde_json::to_string(&e.meta).unwrap_or_else(|_| "{}".to_string());

        sqlx::query(&q.update_entry.query)
            .bind(id)
            .bind(&content_json)
            .bind(&e.initial)
            .bind(e.weight)
            .bind(&tokens)
            .bind(&e.lang)
            .bind(&tags_json)
            .bind(&phones_json)
            .bind(&e.notes)
            .bind(&meta_json)
            .bind(&e.status)
            .execute(&self.db)
            .await?;

        Ok(())
    }

    pub async fn delete_entry(&self, id: i64) -> Result<(), Error> {
        sqlx::query(&q.delete_entry.query)
            .bind(id)
            .execute(&self.db)
            .await?;
        Ok(())
    }

    // #########################
    // Relations.

    /// Insert a new relation into the database.
    pub async fn insert_relation(
        &self,
        from_id: i64,
        to_id: i64,
        r: &Relation,
    ) -> Result<i64, Error> {
        let types_json = serde_json::to_string(&r.types.0).unwrap_or_else(|_| "[]".to_string());
        let tags_json = serde_json::to_string(&r.tags.0).unwrap_or_else(|_| "[]".to_string());

        let status = if r.status.is_empty() {
            STATUS_ENABLED.to_string()
        } else {
            r.status.clone()
        };

        let row = sqlx::query(&q.insert_relation.query)
            .bind(from_id)
            .bind(to_id)
            .bind(&types_json)
            .bind(&tags_json)
            .bind(&r.notes)
            .bind(r.weight)
            .bind(&status)
            .fetch_one(&self.db)
            .await?;

        Ok(row.get(0))
    }

    /// Update an existing relation.
    pub async fn update_relation(&self, id: i64, r: &Relation) -> Result<(), Error> {
        let types_json = serde_json::to_string(&r.types.0).unwrap_or_else(|_| "[]".to_string());
        let tags_json = serde_json::to_string(&r.tags.0).unwrap_or_else(|_| "[]".to_string());

        sqlx::query(&q.update_relation.query)
            .bind(id)
            .bind(&types_json)
            .bind(&tags_json)
            .bind(&r.notes)
            .bind(r.weight)
            .bind(&r.status)
            .execute(&self.db)
            .await?;

        Ok(())
    }

    /// Delete a relation.
    pub async fn delete_relation(&self, id: i64) -> Result<(), Error> {
        sqlx::query(&q.delete_relation.query)
            .bind(id)
            .execute(&self.db)
            .await?;
        Ok(())
    }

    /// Reorder relations based on provided IDs.
    pub async fn reorder_relations(&self, ids: &[i64]) -> Result<(), Error> {
        let ids_json = serde_json::to_string(ids).unwrap_or_else(|_| "[]".to_string());
        sqlx::query(&q.reorder_relations.query)
            .bind(&ids_json)
            .execute(&self.db)
            .await?;
        Ok(())
    }

    // #########################
    // Glossary

    /// Get initials for glossary words for a given language.
    pub async fn get_initials(&self, lang: &str) -> Result<Vec<String>, Error> {
        let rows: Vec<(String,)> = sqlx::query_as(&q.get_initials.query)
            .bind(lang)
            .fetch_all(&self.db)
            .await?;
        Ok(rows.into_iter().map(|(s,)| s).collect())
    }

    /// Get glossary words for a given language and initial.
    pub async fn get_glossary_words(
        &self,
        lang: &str,
        initial: &str,
        offset: i32,
        limit: i32,
    ) -> Result<(Vec<GlossaryWord>, i64), Error> {
        let words: Vec<GlossaryWord> = sqlx::query_as(&q.get_glossary_words.query)
            .bind(lang)
            .bind(initial)
            .bind(offset)
            .bind(limit)
            .fetch_all(&self.db)
            .await?;

        let total = words.first().map(|w| w.total).unwrap_or(0);
        Ok((words, total))
    }

    // #########################
    // Submissions

    /// Get pending entries for a given language (with pagination).
    pub async fn get_pending_entries(
        &self,
        lang: &str,
        offset: i32,
        limit: i32,
    ) -> Result<(Vec<Entry>, i64), Error> {
        let entries: Vec<Entry> = sqlx::query_as(&q.get_pending_entries.query)
            .bind(lang)
            .bind(offset)
            .bind(limit)
            .fetch_all(&self.db)
            .await?;

        let total = entries.first().map(|e| e.total).unwrap_or(0);
        Ok((entries, total))
    }

    /// Insert a new submission entry in the DB.
    pub async fn insert_submission_entry(&self, e: &Entry) -> Result<Option<i64>, Error> {
        if !self.langs.contains_key(&e.lang) {
            return Err(Error::UnknownLang(e.lang.clone()));
        }

        let tokens = if e.tokens.is_empty() {
            self.tokenize(&e.content.0, &e.lang)?
        } else {
            e.tokens.clone()
        };

        let guid = if e.guid.is_empty() {
            uuid::Uuid::new_v4().to_string()
        } else {
            e.guid.clone()
        };

        let content_json = serde_json::to_string(&e.content.0).unwrap_or_else(|_| "[]".to_string());
        let tags_json = serde_json::to_string(&e.tags.0).unwrap_or_else(|_| "[]".to_string());
        let phones_json = serde_json::to_string(&e.phones.0).unwrap_or_else(|_| "[]".to_string());
        let meta_json = serde_json::to_string(&e.meta).unwrap_or_else(|_| "{}".to_string());

        let row: Option<(i64,)> = sqlx::query_as(&q.insert_submission_entry.query)
            .bind(&guid)
            .bind(&content_json)
            .bind(&e.initial)
            .bind(e.weight)
            .bind(&tokens)
            .bind(&e.lang)
            .bind(&tags_json)
            .bind(&phones_json)
            .bind(&e.notes)
            .bind(&meta_json)
            .bind(&e.status)
            .fetch_optional(&self.db)
            .await?;

        Ok(row.map(|(id,)| id))
    }

    /// Insert a new submission relation to the DB.
    pub async fn insert_submission_relation(
        &self,
        from_id: i64,
        to_id: i64,
        r: &Relation,
    ) -> Result<Option<i64>, Error> {
        let types_json = serde_json::to_string(&r.types.0).unwrap_or_else(|_| "[]".to_string());
        let tags_json = serde_json::to_string(&r.tags.0).unwrap_or_else(|_| "[]".to_string());

        let row: Option<(i64,)> = sqlx::query_as(&q.insert_submission_relation.query)
            .bind(from_id)
            .bind(to_id)
            .bind(&types_json)
            .bind(&tags_json)
            .bind(&r.notes)
            .bind(r.weight)
            .bind(&r.status)
            .fetch_optional(&self.db)
            .await?;

        Ok(row.map(|(id,)| id))
    }

    // Approve a user submission.
    pub async fn approve_submission(&self, id: i64) -> Result<(), Error> {
        sqlx::query(&q.approve_submission.query)
            .bind(id)
            .execute(&self.db)
            .await?;
        sqlx::query(&q.approve_submission_relations.query)
            .bind(id)
            .execute(&self.db)
            .await?;
        sqlx::query(&q.approve_submission_to_entries.query)
            .bind(id)
            .execute(&self.db)
            .await?;
        Ok(())
    }

    // Reject a user submission.
    pub async fn reject_submission(&self, id: i64) -> Result<(), Error> {
        sqlx::query(&q.reject_submission_to_entries.query)
            .bind(id)
            .execute(&self.db)
            .await?;
        sqlx::query(&q.reject_submission_relations.query)
            .bind(id)
            .execute(&self.db)
            .await?;
        sqlx::query(&q.reject_submission.query)
            .bind(id)
            .execute(&self.db)
            .await?;
        Ok(())
    }

    // #########################
    // Comments

    /// Insert a new comment.
    pub async fn insert_comment(
        &self,
        from_guid: &str,
        to_guid: &str,
        comments: &str,
    ) -> Result<(), Error> {
        sqlx::query(&q.insert_comment.query)
            .bind(from_guid)
            .bind(to_guid)
            .bind(comments)
            .execute(&self.db)
            .await?;
        Ok(())
    }

    /// Get all comments.
    pub async fn get_comments(&self) -> Result<Vec<Comment>, Error> {
        let comments: Vec<Comment> = sqlx::query_as(&q.get_comments.query)
            .fetch_all(&self.db)
            .await?;
        Ok(comments)
    }

    /// Delete a comment by ID.
    pub async fn delete_comment(&self, id: i64) -> Result<(), Error> {
        sqlx::query(&q.delete_comment.query)
            .bind(id)
            .execute(&self.db)
            .await?;
        Ok(())
    }

    /// Delete all pending entries, relations, and comments.
    pub async fn delete_all_pending(&self) -> Result<(), Error> {
        sqlx::query(&q.delete_all_pending_relations.query)
            .execute(&self.db)
            .await?;
        sqlx::query(&q.delete_all_pending.query)
            .execute(&self.db)
            .await?;
        sqlx::query(&q.delete_all_comments.query)
            .execute(&self.db)
            .await?;
        Ok(())
    }

    // #########################
    // Misc

    pub async fn get_stats(&self) -> Result<Stats, Error> {
        let row: (String,) = sqlx::query_as(&q.get_stats.query)
            .fetch_one(&self.db)
            .await?;
        let stats: Stats = serde_json::from_str(&row.0).unwrap_or_default();
        Ok(stats)
    }
}
