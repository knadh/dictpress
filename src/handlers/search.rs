use std::sync::Arc;

use axum::{
    extract::{Path, Query, State},
    http::StatusCode,
};

use super::{json, paginate, total_pages, ApiErr, ApiResp, Ctx, Result};
use crate::cache::make_search_cache_key;
use crate::models::{RelationsQuery, SearchQuery, SearchResults, STATUS_ENABLED};

/// Search a dictionary with query in path (public API).
pub async fn search(
    State(ctx): State<Arc<Ctx>>,
    Path((from_lang, to_lang, query)): Path<(String, String, String)>,
    Query(mut q): Query<SearchQuery>,
) -> Result<ApiResp<SearchResults>> {
    q.query = query;
    q.from_lang = from_lang;
    q.to_lang = to_lang;

    // Pagination.
    let (page, per_page, offset) = paginate(
        q.page,
        q.per_page,
        ctx.consts.api_max_per_page,
        ctx.consts.api_default_per_page,
    );

    q.page = page;
    q.offset = offset;
    q.limit = per_page;

    Ok(json(do_search(ctx, q, false).await?))
}

/// Admin search (response includes internal IDs also).
pub async fn search_admin(
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

    Ok(json(do_search(ctx, query, true).await?))
}

/// Perform search. Reads offset/limit and max_relations/max_content_items from query.
pub async fn do_search(ctx: Arc<Ctx>, query: SearchQuery, is_admin: bool) -> Result<SearchResults> {
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

    // Check cache for non-admin requests.
    let cache_key = if !is_admin && ctx.cache.is_some() {
        let key = make_search_cache_key(&query);
        if let Some(cache) = &ctx.cache {
            if let Some(cached) = cache.get(&key).await {
                match rmp_serde::from_slice::<SearchResults>(&cached) {
                    Ok(results) => {
                        log::debug!("cache hit for search key={}", key);
                        return Ok(results);
                    }
                    Err(e) => {
                        log::warn!("failed to deserialize cached search results: {}", e);
                    }
                }
            }
        }
        Some(key)
    } else {
        None
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

    let results = SearchResults {
        entries,
        page: query.page,
        per_page: query.limit,
        total,
        total_pages: total_pages(total, query.limit),
    };

    // Cache the results for non-admin requests.
    if let Some(key) = cache_key {
        if let Some(cache) = &ctx.cache {
            match rmp_serde::to_vec_named(&results) {
                Ok(encoded) => {
                    cache.put(&key, &encoded);
                }
                Err(e) => {
                    log::warn!("failed to serialize search results for caching: {}", e);
                }
            }
        }
    }

    Ok(results)
}
