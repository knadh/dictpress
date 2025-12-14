use std::sync::Arc;

use axum::{
    extract::{Path, Query, State},
    http::StatusCode,
};

use super::{json, paginate, total_pages, ApiErr, ApiResp, Ctx, Result};
use crate::models::{RelationsQuery, SearchQuery, SearchResults, STATUS_ENABLED};

/// Search a dictionary with query in path (public API).
pub async fn search(
    State(ctx): State<Arc<Ctx>>,
    Path((from_lang, to_lang, q)): Path<(String, String, String)>,
    Query(mut query): Query<SearchQuery>,
) -> Result<ApiResp<SearchResults>> {
    query.query = q;
    query.from_lang = from_lang;
    query.to_lang = to_lang;

    // Pagination.
    let (page, per_page, offset) = paginate(
        query.page,
        query.per_page,
        ctx.consts.api_max_per_page,
        ctx.consts.api_default_per_page,
    );

    query.page = page;
    query.offset = offset;
    query.limit = per_page;

    do_search(ctx, query, false).await
}

/// Admin search (response includes internal IDs also).
pub async fn search_admin(
    State(ctx): State<Arc<Ctx>>,
    Path((from_lang, to_lang)): Path<(String, String)>,
    Query(mut query): Query<SearchQuery>,
) -> Result<ApiResp<SearchResults>> {
    query.from_lang = from_lang;
    query.to_lang = to_lang;

    // Pagination.
    let (page, per_page, offset) = paginate(
        query.page,
        query.per_page,
        ctx.consts.api_max_per_page,
        ctx.consts.api_default_per_page,
    );
    query.page = page;
    query.offset = offset;
    query.limit = per_page;

    do_search(ctx, query, true).await
}

/// Perform search. Reads offset/limit and max_relations/max_content_items from query.
pub async fn do_search(
    ctx: Arc<Ctx>,
    query: SearchQuery,
    is_admin: bool,
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

    // Search entries in the DB.
    let (mut entries, total) = ctx.mgr.search(&query, query.offset, query.limit).await?;

    // Load relations for results.
    let status = if query.status.is_empty() {
        STATUS_ENABLED
    } else {
        &query.status
    };
    ctx.mgr
        .load_relations(
            &mut entries,
            &RelationsQuery {
                to_lang,
                types: query.types.clone(),
                tags: query.tags.clone(),
                status: status.to_string(),
                max_per_type: query.max_relations,
                max_content_items: query.max_content_items,
            },
        )
        .await?;

    // Hide internal IDs for non-admin requests.
    if !is_admin {
        for entry in &mut entries {
            entry.id = 0;
            for r in &mut entry.relations {
                r.id = 0;
                if let Some(rel) = &mut r.relation {
                    rel.id = 0;
                }
            }
        }
    }

    Ok(json(SearchResults {
        entries,
        page: query.page,
        per_page: query.limit,
        total,
        total_pages: total_pages(total, query.limit),
    }))
}
