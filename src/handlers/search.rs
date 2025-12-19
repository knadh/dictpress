use std::sync::Arc;

use axum::{
    extract::{Path, Query, State},
    http::StatusCode,
};

use super::{clean_query, json, paginate, total_pages, ApiErr, ApiResp, Ctx, Result};
use crate::cache::make_search_cache_key;
use crate::models::{
    RelationsQuery, SearchQuery, SearchResults, StringArray, Suggestion, STATUS_ENABLED,
};

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

    Ok(json(do_search(ctx, query, true).await?))
}

/// Perform search. Reads offset/limit and max_relations/max_content_items from query.
pub async fn do_search(ctx: Arc<Ctx>, mut query: SearchQuery, is_admin: bool) -> Result<SearchResults> {
    // Clean and normalize the query string.
    query.query = clean_query(&query.query);

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

/// Suggestions endpoint for search word autocomplete.
pub async fn get_suggestions(
    State(ctx): State<Arc<Ctx>>,
    Path((lang, q)): Path<(String, String)>,
) -> Result<ApiResp<Vec<Suggestion>>> {
    // Clean and normalize the query string.
    let q = clean_query(&q);

    if q.is_empty() {
        return Err(ApiErr::new("`q` is required", StatusCode::BAD_REQUEST));
    }
    if !ctx.langs.contains_key(&lang) {
        return Err(ApiErr::new("unknown language", StatusCode::BAD_REQUEST));
    }

    // If suggestions are disable, return an empty array.
    if !ctx.consts.suggestions_enabled {
        return Ok(json(Vec::new()));
    }

    let limit = ctx.consts.num_suggestions;

    // Try trie search first if suggestions are enabled.
    let mut out: Vec<Suggestion> = if let Some(sugg) = &ctx.suggestions {
        sugg.query(&lang, &q, limit as usize)
            .into_iter()
            .map(|w| Suggestion {
                content: StringArray(vec![w]),
            })
            .collect()
    } else {
        Vec::new()
    };

    // If there are fewer than limit results, supplement with DB FTS search.
    if out.len() < limit as usize {
        let remaining = limit - out.len() as i32;
        if let Ok(res) = ctx.mgr.get_suggestions(&lang, &q, remaining).await {
            for s in res {
                if !out.iter().any(|r| r.content.0 == s.content.0) {
                    out.push(s);
                    if out.len() >= limit as usize {
                        break;
                    }
                }
            }
        }
    }

    Ok(json(out))
}
