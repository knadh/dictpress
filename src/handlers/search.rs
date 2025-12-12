use std::sync::Arc;

use axum::{
    extract::{Path, Query, State},
    http::StatusCode,
};

use super::{json, paginate, total_pages, ApiErr, ApiResp, Ctx, Result};
use crate::models::{GlossaryResults, SearchQuery, SearchResults, STATUS_ENABLED};

/// Glossary query params.
#[derive(Debug, serde::Deserialize, Default)]
pub struct GlossaryQuery {
    #[serde(default)]
    pub page: i32,
    #[serde(default)]
    pub per_page: i32,
}

/// Search a dictionary with query in path (public API).
pub async fn search(
    State(ctx): State<Arc<Ctx>>,
    Path((from_lang, to_lang, q)): Path<(String, String, String)>,
    Query(mut query): Query<SearchQuery>,
) -> Result<ApiResp<SearchResults>> {
    query.query = q;
    query.from_lang = from_lang;
    query.to_lang = to_lang.clone();

    do_search(ctx, query, false, 0, 0).await
}

/// Admin search (response includes internal IDs also).
pub async fn search_admin(
    State(ctx): State<Arc<Ctx>>,
    Path((from_lang, to_lang)): Path<(String, String)>,
    Query(mut query): Query<SearchQuery>,
) -> Result<ApiResp<SearchResults>> {
    query.from_lang = from_lang;
    query.to_lang = to_lang.clone();

    do_search(ctx, query, true, 0, 0).await
}

/// Perform search with configurable limits on relations and content items.
/// max_relations: 0 = unlimited, >0 = limit per type.
/// max_content_items: 0 = unlimited, >0 = truncate content array.
pub async fn do_search(
    ctx: Arc<Ctx>,
    query: SearchQuery,
    is_admin: bool,
    max_relations: i32,
    max_content_items: i32,
) -> Result<ApiResp<SearchResults>> {
    if query.query.is_empty() {
        return Err(ApiErr::new("query is required", StatusCode::BAD_REQUEST));
    }

    // Validate languages.
    if !ctx.langs.contains_key(&query.from_lang) {
        return Err(ApiErr::new("unknown `from_lang`", StatusCode::BAD_REQUEST));
    }

    let to_lang = if query.to_lang == "*" {
        String::new()
    } else {
        if !query.to_lang.is_empty() && !ctx.langs.contains_key(&query.to_lang) {
            return Err(ApiErr::new("unknown `to_lang`", StatusCode::BAD_REQUEST));
        }
        query.to_lang.clone()
    };

    // Figure out pagination (use API pagination config for API endpoints).
    let (page, per_page, offset) = paginate(
        query.page,
        query.per_page,
        ctx.consts.api_max_per_page,
        ctx.consts.api_default_per_page,
    );

    // Search entries in the DB.
    let (mut entries, total) = ctx.mgr.search(&query, offset, per_page).await?;

    // Load relations for results.
    let status = if query.status.is_empty() {
        STATUS_ENABLED
    } else {
        &query.status
    };
    ctx.mgr
        .load_relations(
            &mut entries,
            &to_lang,
            &query.types,
            &query.tags,
            status,
            max_relations,
        )
        .await?;

    // Apply content item limit if specified.
    if max_content_items > 0 {
        for entry in &mut entries {
            for rel in &mut entry.relations {
                if rel.content.len() > max_content_items as usize {
                    rel.content.0.truncate(max_content_items as usize);
                }
            }
        }
    }

    // Hide internal IDs for non-admin requests.
    if !is_admin {
        for e in &mut entries {
            e.id = 0;
            for r in &mut e.relations {
                r.id = 0;
                if let Some(rel) = &mut r.relation {
                    rel.id = 0;
                }
            }
        }
    }

    Ok(json(SearchResults {
        entries,
        page,
        per_page,
        total,
        total_pages: total_pages(total, per_page),
    }))
}

/// Get unique initials for a language.
pub async fn get_initials(
    State(ctx): State<Arc<Ctx>>,
    Path(lang): Path<String>,
) -> Result<ApiResp<Vec<String>>> {
    // Check if glossary is enabled.
    if !ctx.consts.enable_glossary {
        return Err(ApiErr::new("glossary disabled", StatusCode::NOT_FOUND));
    }

    if !ctx.langs.contains_key(&lang) {
        return Err(ApiErr::new("unknown language", StatusCode::BAD_REQUEST));
    }

    let initials = ctx.mgr.get_initials(&lang).await?;
    Ok(json(initials))
}

/// Get glossary words.
pub async fn get_glossary_words(
    State(ctx): State<Arc<Ctx>>,
    Path((lang, initial)): Path<(String, String)>,
    Query(query): Query<GlossaryQuery>,
) -> Result<ApiResp<GlossaryResults>> {
    // Check if glossary is enabled.
    if !ctx.consts.enable_glossary {
        return Err(ApiErr::new("glossary disabled", StatusCode::NOT_FOUND));
    }

    if !ctx.langs.contains_key(&lang) {
        return Err(ApiErr::new("unknown language", StatusCode::BAD_REQUEST));
    }

    // Use glossary pagination config.
    let (page, per_page, offset) = paginate(
        query.page,
        query.per_page,
        ctx.consts.glossary_max_per_page,
        ctx.consts.glossary_default_per_page,
    );

    let (words, total) = ctx
        .mgr
        .get_glossary_words(&lang, &initial, offset, per_page)
        .await?;

    Ok(json(GlossaryResults {
        words,
        page,
        per_page,
        total,
        total_pages: total_pages(total, per_page),
    }))
}
