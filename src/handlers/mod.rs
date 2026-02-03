pub mod admin;
pub mod entries;
pub mod relations;
pub mod search;
pub mod site;
pub mod submissions;

use std::{collections::HashMap, sync::Arc};

use axum::{
    body::Bytes,
    http::StatusCode,
    response::{IntoResponse, Response},
    Json,
};
use serde::Serialize;
use tera::Tera;

use crate::{
    autocomplete::Autocomplete,
    cache::Cache,
    manager::Manager,
    models::{Dicts, LangMap, Stats},
};

pub type I18n = tinyi18n_rs::I18n;

/// Application context passed to all handlers.
pub struct Ctx {
    pub mgr: Arc<Manager>,
    pub langs: LangMap,
    pub dicts: Dicts,
    pub cache: Option<Arc<Cache>>,
    pub autocomplete: Option<Arc<Autocomplete>>,

    /// Admin templates (always loaded, embedded in binary).
    pub admin_tpl: Arc<Tera>,
    /// Site templates (optional, loaded from --site directory).
    pub site_tpl: Option<Arc<Tera>>,
    pub site_path: Option<std::path::PathBuf>,
    pub i18n: I18n,
    /// Preloaded static files (JS & CSS) for bundling.
    pub static_files: HashMap<String, Bytes>,

    pub consts: Consts,
    pub asset_ver: String,
    pub version: String,
}

/// Application constants.
#[derive(Clone, serde::Serialize)]
pub struct Consts {
    pub root_url: String,
    pub enable_pages: bool,
    pub enable_submissions: bool,
    pub enable_glossary: bool,
    #[serde(skip)]
    pub admin_username: String,
    #[serde(skip)]
    pub admin_password: String,

    // API pagination settings.
    pub api_default_per_page: i32,
    pub api_max_per_page: i32,

    // Site pagination settings.
    pub site_default_per_page: i32,
    pub site_max_per_page: i32,
    pub site_num_page_nums: i32,
    pub site_max_relations_per_type: i32,
    pub site_max_content_items: i32,

    // Glossary pagination settings.
    pub glossary_default_per_page: i32,
    pub glossary_max_per_page: i32,
    pub glossary_num_page_nums: i32,

    // Autocomplete settings.
    pub autocomplete_enabled: bool,
    pub num_autocomplete: i32,

    // Admin assets split by type for easier template rendering.
    pub admin_js_assets: Vec<String>,
    pub admin_css_assets: Vec<String>,

    // Dictionary entry stats for templates (homepage).
    pub stats: Stats,
}

impl Default for Consts {
    fn default() -> Self {
        Self {
            root_url: String::new(),
            enable_pages: true,
            enable_submissions: false,
            enable_glossary: true,
            admin_username: String::new(),
            admin_password: String::new(),

            api_default_per_page: 10,
            api_max_per_page: 20,

            site_default_per_page: 10,
            site_max_per_page: 20,
            site_num_page_nums: 10,
            site_max_relations_per_type: 5,
            site_max_content_items: 5,

            glossary_default_per_page: 100,
            glossary_max_per_page: 100,
            glossary_num_page_nums: 10,

            autocomplete_enabled: false,
            num_autocomplete: 10,

            admin_js_assets: Vec::new(),
            admin_css_assets: Vec::new(),

            stats: Stats::default(),
        }
    }
}

/// API response wrapper.
#[derive(Serialize)]
pub struct ApiResp<T> {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub message: Option<String>,
    pub data: Option<T>,
}

impl<T: Serialize> IntoResponse for ApiResp<T> {
    fn into_response(self) -> Response {
        (StatusCode::OK, Json(self)).into_response()
    }
}

pub fn json<T: Serialize>(data: T) -> ApiResp<T> {
    ApiResp {
        data: Some(data),
        message: None,
    }
}

/// API error type.
#[derive(Debug)]
pub struct ApiErr {
    pub message: String,
    pub status: StatusCode,
}

impl ApiErr {
    pub fn new(message: impl Into<String>, status: StatusCode) -> Self {
        Self {
            message: message.into(),
            status,
        }
    }
}

impl<E: std::fmt::Display> From<E> for ApiErr {
    fn from(err: E) -> Self {
        Self::new(err.to_string(), StatusCode::INTERNAL_SERVER_ERROR)
    }
}

impl IntoResponse for ApiErr {
    fn into_response(self) -> Response {
        let json = Json(ApiResp::<()> {
            data: None,
            message: Some(self.message),
        });
        (self.status, json).into_response()
    }
}

pub type Result<T> = std::result::Result<T, ApiErr>;

/// Pagination helper.
pub fn paginate(
    page: i32,
    per_page: i32,
    max_per_page: i32,
    default_per_page: i32,
) -> (i32, i32, i32) {
    let page = if page < 1 { 1 } else { page };
    let per_page = if per_page < 1 {
        default_per_page
    } else if per_page > max_per_page {
        max_per_page
    } else {
        per_page
    };
    let offset = (page - 1) * per_page;
    (page, per_page, offset)
}

pub fn total_pages(total: i64, per_page: i32) -> i32 {
    ((total as f64) / (per_page as f64)).ceil() as i32
}

/// Clean and normalize a query string by replacing punctuation chars with spaces.
/// Preserves apostrophe for contractions/possessives (one's, don't).
/// Collapses multiple spaces into single spaces.
pub fn clean_query(q: &str) -> String {
    q.chars()
        .map(|c| {
            if c == '\'' {
                c
            } else if c.is_ascii_punctuation() {
                ' '
            } else {
                c
            }
        })
        .collect::<String>()
        .split_whitespace()
        .collect::<Vec<_>>()
        .join(" ")
}

/// Generic pagination query params.
#[derive(Debug, serde::Deserialize, Default)]
pub struct PaginationQuery {
    #[serde(default)]
    pub page: i32,
    #[serde(default)]
    pub per_page: i32,
}
