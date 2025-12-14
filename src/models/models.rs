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
#[allow(dead_code)]
pub const STATUS_DISABLED: &str = "disabled";

/// JSON array wrapper for SQLite TEXT columns storing JSON arrays.
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct StringArray(pub Vec<String>);

impl StringArray {
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

    #[sqlx(default)]
    pub content_length: i32,

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

    // Internal fields (not from HTTP query).
    #[serde(skip)]
    pub offset: i32,

    #[serde(skip)]
    pub limit: i32,

    #[serde(skip)]
    pub max_relations: i32, // 0 = no limit.

    #[serde(skip)]
    pub max_content_items: i32, // 0 = no limit.
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
    pub api_results: ApiResultsConfig,

    #[serde(default)]
    pub site_results: SiteResultsConfig,

    #[serde(default)]
    pub glossary: GlossaryConfig,

    #[serde(default)]
    pub lang: HashMap<String, LangConfig>,
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
    pub admin_assets: Vec<String>,

    #[serde(default = "default_true")]
    pub enable_pages: bool,

    #[serde(default)]
    pub enable_submissions: bool,

    #[serde(default)]
    pub dicts: Vec<Vec<String>>,

    #[serde(default)]
    pub tokenizers_dir: String,
}

fn default_true() -> bool {
    true
}

#[derive(Debug, Clone, Deserialize)]
pub struct ApiResultsConfig {
    #[serde(default = "default_api_per_page")]
    pub per_page: i32,
    #[serde(default = "default_api_max_per_page")]
    pub max_per_page: i32,
}

fn default_api_per_page() -> i32 {
    10
}

fn default_api_max_per_page() -> i32 {
    20
}

impl Default for ApiResultsConfig {
    fn default() -> Self {
        Self {
            per_page: default_api_per_page(),
            max_per_page: default_api_max_per_page(),
        }
    }
}

#[derive(Debug, Clone, Deserialize)]
pub struct SiteResultsConfig {
    #[serde(default = "default_site_per_page")]
    pub per_page: i32,
    #[serde(default = "default_site_max_per_page")]
    pub max_per_page: i32,
    #[serde(default = "default_num_page_nums")]
    pub num_page_nums: i32,
    #[serde(default = "default_max_entry_relations_per_type")]
    pub max_entry_relations_per_type: i32,
    #[serde(default = "default_max_entry_content_items")]
    pub max_entry_content_items: i32,
}

fn default_site_per_page() -> i32 {
    10
}

fn default_site_max_per_page() -> i32 {
    20
}

fn default_num_page_nums() -> i32 {
    10
}

fn default_max_entry_relations_per_type() -> i32 {
    5
}

fn default_max_entry_content_items() -> i32 {
    5
}

impl Default for SiteResultsConfig {
    fn default() -> Self {
        Self {
            per_page: default_site_per_page(),
            max_per_page: default_site_max_per_page(),
            num_page_nums: default_num_page_nums(),
            max_entry_relations_per_type: default_max_entry_relations_per_type(),
            max_entry_content_items: default_max_entry_content_items(),
        }
    }
}

#[derive(Debug, Clone, Deserialize)]
pub struct GlossaryConfig {
    #[serde(default = "default_true")]
    pub enabled: bool,
    #[serde(default = "default_glossary_per_page")]
    pub default_per_page: i32,
    #[serde(default = "default_glossary_max_per_page")]
    pub max_per_page: i32,
    #[serde(default = "default_num_page_nums")]
    pub num_page_nums: i32,
}

fn default_glossary_per_page() -> i32 {
    100
}

fn default_glossary_max_per_page() -> i32 {
    100
}

fn default_max_conns() -> u32 {
    5
}

impl Default for GlossaryConfig {
    fn default() -> Self {
        Self {
            enabled: true,
            default_per_page: default_glossary_per_page(),
            max_per_page: default_glossary_max_per_page(),
            num_page_nums: default_num_page_nums(),
        }
    }
}

#[derive(Debug, Clone, Default, Deserialize)]
pub struct DbConfig {
    #[serde(default = "default_max_conns")]
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
