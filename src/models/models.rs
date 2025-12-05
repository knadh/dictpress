use std::collections::HashMap;

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use sqlx::{
    encode::IsNull,
    error::BoxDynError,
    sqlite::{SqliteArgumentValue, SqliteTypeInfo, SqliteValueRef},
    Decode, Encode, FromRow, Sqlite, Type,
};

pub const STATUS_PENDING: &str = "pending";
pub const STATUS_ENABLED: &str = "enabled";
pub const STATUS_DISABLED: &str = "disabled";

/// JSON array wrapper for SQLite TEXT columns storing JSON arrays.
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct StringArray(pub Vec<String>);

impl StringArray {
    pub fn new() -> Self {
        Self(Vec::new())
    }

    pub fn is_empty(&self) -> bool {
        self.0.is_empty()
    }

    pub fn len(&self) -> usize {
        self.0.len()
    }
}

impl From<Vec<String>> for StringArray {
    fn from(v: Vec<String>) -> Self {
        Self(v)
    }
}

impl Type<Sqlite> for StringArray {
    fn type_info() -> SqliteTypeInfo {
        <String as Type<Sqlite>>::type_info()
    }
}

impl<'q> Encode<'q, Sqlite> for StringArray {
    fn encode_by_ref(&self, buf: &mut Vec<SqliteArgumentValue<'q>>) -> Result<IsNull, BoxDynError> {
        let json = serde_json::to_string(&self.0).unwrap_or_else(|_| "[]".to_string());
        <String as Encode<Sqlite>>::encode(json, buf)
    }
}

impl<'r> Decode<'r, Sqlite> for StringArray {
    fn decode(value: SqliteValueRef<'r>) -> Result<Self, BoxDynError> {
        let s = <String as Decode<Sqlite>>::decode(value)?;
        if s.is_empty() {
            return Ok(Self(Vec::new()));
        }
        let v: Vec<String> = serde_json::from_str(&s)?;
        Ok(Self(v))
    }
}

/// Dictionary entry.
#[derive(Debug, Clone, Default, Serialize, Deserialize, FromRow)]
pub struct Entry {
    #[serde(skip_serializing_if = "is_zero")]
    pub id: i64,
    pub guid: String,
    pub content: StringArray,
    pub initial: String,
    pub weight: f64,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub tokens: String,
    pub lang: String,
    pub tags: StringArray,
    pub phones: StringArray,
    pub notes: String,
    #[sqlx(try_from = "String")]
    pub meta: serde_json::Value,
    pub status: String,
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,

    // Populated after loading relations.
    #[sqlx(skip)]
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub relations: Vec<Entry>,

    #[sqlx(default)]
    #[serde(skip_serializing_if = "is_zero_i32")]
    pub total_relations: i32,

    // Pagination total (not serialized).
    #[sqlx(default)]
    #[serde(skip)]
    pub total: i64,

    // Relation data populated during relation loading (not serialized directly).
    #[sqlx(default)]
    #[serde(skip)]
    pub from_id: i64,
    #[sqlx(default)]
    #[serde(skip)]
    pub relation_id: i64,
    #[sqlx(default)]
    #[serde(skip)]
    pub relation_types: StringArray,
    #[sqlx(default)]
    #[serde(skip)]
    pub relation_tags: StringArray,
    #[sqlx(default)]
    #[serde(skip)]
    pub relation_notes: String,
    #[sqlx(default)]
    #[serde(skip)]
    pub relation_weight: f64,
    #[sqlx(default)]
    #[serde(skip)]
    pub relation_status: String,
    #[sqlx(default)]
    #[serde(skip)]
    pub relation_created_at: Option<DateTime<Utc>>,
    #[sqlx(default)]
    #[serde(skip)]
    pub relation_updated_at: Option<DateTime<Utc>>,

    // Relation metadata (populated from relation_* fields).
    #[sqlx(skip)]
    #[serde(skip_serializing_if = "Option::is_none")]
    pub relation: Option<Relation>,
}

fn is_zero(v: &i64) -> bool {
    *v == 0
}

fn is_zero_i32(v: &i32) -> bool {
    *v == 0
}

/// Relation between two entries.
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct Relation {
    #[serde(skip_serializing_if = "is_zero")]
    pub id: i64,
    pub types: StringArray,
    pub tags: StringArray,
    pub notes: String,
    pub weight: f64,
    pub status: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub created_at: Option<DateTime<Utc>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub updated_at: Option<DateTime<Utc>>,
}

/// Glossary word for index pages.
#[derive(Debug, Clone, Serialize, Deserialize, FromRow)]
pub struct GlossaryWord {
    pub id: i64,
    pub guid: String,
    pub content: StringArray,
    #[serde(skip)]
    pub total: i64,
}

/// Database statistics.
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct Stats {
    pub entries: i64,
    pub relations: i64,
    pub languages: HashMap<String, i64>,
}

/// Public comment/suggestion.
#[derive(Debug, Clone, Serialize, Deserialize, FromRow)]
pub struct Comment {
    pub id: i64,
    pub from_id: i64,
    pub to_id: Option<i64>,
    pub comments: String,
    pub created_at: DateTime<Utc>,
    #[sqlx(default)]
    pub from_guid: Option<String>,
    #[sqlx(default)]
    pub to_guid: Option<String>,
}

/// Search query parameters.
#[derive(Debug, Clone, Default, Deserialize)]
pub struct SearchQuery {
    #[serde(rename = "q", default)]
    pub query: String,
    #[serde(default)]
    pub from_lang: String,
    #[serde(default)]
    pub to_lang: String,
    #[serde(rename = "type", default)]
    pub types: Vec<String>,
    #[serde(rename = "tag", default)]
    pub tags: Vec<String>,
    #[serde(default)]
    pub status: String,
    #[serde(default)]
    pub page: i32,
    #[serde(default)]
    pub per_page: i32,
}

/// Search results wrapper.
#[derive(Debug, Clone, Serialize)]
pub struct SearchResults {
    pub entries: Vec<Entry>,
    pub page: i32,
    pub per_page: i32,
    pub total: i64,
    pub total_pages: i32,
}

/// Glossary results wrapper.
#[derive(Debug, Clone, Serialize)]
pub struct GlossaryResults {
    pub words: Vec<GlossaryWord>,
    pub page: i32,
    pub per_page: i32,
    pub total: i64,
    pub total_pages: i32,
}

/// Language configuration.
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct Lang {
    #[serde(default)]
    pub id: String,
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub types: HashMap<String, String>,
    #[serde(default)]
    pub tokenizer: String,
    #[serde(default)]
    pub tokenizer_type: String,
}

pub type LangMap = HashMap<String, Lang>;

/// Dictionary pair (from_lang, to_lang).
pub type DictPair = (Lang, Lang);
pub type Dicts = Vec<DictPair>;

/// Application configuration.
#[derive(Debug, Clone, Default, Deserialize)]
pub struct Config {
    pub app: AppConfig,
    pub db: DbConfig,
    #[serde(default)]
    pub lang: HashMap<String, LangConfig>,
    #[serde(default)]
    pub tokenizer: HashMap<String, toml::Value>,
}

#[derive(Debug, Clone, Default, Deserialize)]
pub struct AppConfig {
    #[serde(default)]
    pub address: String,
    #[serde(default)]
    pub admin_username: String,
    #[serde(default)]
    pub admin_password: String,
    #[serde(default)]
    pub root_url: String,
    #[serde(default)]
    pub site: String,
    #[serde(default)]
    pub enable_submissions: bool,
    #[serde(default)]
    pub dicts: Vec<Vec<String>>,
    #[serde(default)]
    pub tokenizers_dir: String,
}

#[derive(Debug, Clone, Default, Deserialize)]
pub struct DbConfig {
    #[serde(default)]
    pub path: String,
    #[serde(default)]
    pub max_conns: u32,
}

#[derive(Debug, Clone, Default, Deserialize)]
pub struct LangConfig {
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub tokenizer: String,
    #[serde(default)]
    pub tokenizer_type: String,
    #[serde(default)]
    pub types: HashMap<String, String>,
}
