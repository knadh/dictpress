use std::sync::Arc;

use axum::{
    extract::{Path, Query, State},
    http::StatusCode,
    response::{Html, IntoResponse},
};

use super::Ctx;
use crate::models::{SearchQuery, STATUS_ENABLED};

#[derive(serde::Deserialize)]
pub struct GlossaryParams {
    page: Option<i32>,
    per_page: Option<i32>,
}

#[derive(serde::Serialize)]
struct GlossaryData {
    words: Vec<crate::models::GlossaryWord>,
    from_lang: String,
    to_lang: String,
}

#[derive(serde::Deserialize)]
pub struct MessageParams {
    title: Option<String>,
    message: Option<String>,
}

/// Build common template context.
fn base_context(ctx: &Ctx) -> tera::Context {
    let mut context = tera::Context::new();
    context.insert("asset_ver", &ctx.asset_ver);
    context.insert("consts", &ctx.consts);
    context.insert("i18n", &ctx.i18n);
    context.insert("langs", &ctx.langs);
    // Convert dicts to serializable format.
    let dicts: Vec<_> = ctx.dicts.iter().map(|(from, to)| (from, to)).collect();
    context.insert("dicts", &dicts);
    context
}

/// Render site page using site templates (loaded from --site directory).
fn render(
    ctx: &Ctx,
    template: &str,
    context: &tera::Context,
) -> std::result::Result<Html<String>, impl IntoResponse> {
    match &ctx.site_tpl {
        Some(tpl) => tpl.render(template, context).map(Html).map_err(|e| {
            // Log full error chain for debugging.
            let mut msg = e.to_string();
            let mut source = std::error::Error::source(&e);
            while let Some(cause) = source {
                msg.push_str(&format!(": {}", cause));
                source = std::error::Error::source(cause);
            }
            log::error!("template error: {}", msg);
            (
                StatusCode::INTERNAL_SERVER_ERROR,
                format!("template error: {}", msg),
            )
        }),
        None => Err((
            StatusCode::NOT_FOUND,
            "site templates not loaded".to_string(),
        )),
    }
}

/// Site index.
pub async fn index(State(ctx): State<Arc<Ctx>>) -> impl IntoResponse {
    let mut context = base_context(&ctx);
    context.insert("page_type", "/");
    render(&ctx, "index.html", &context)
}

/// Search page.
pub async fn search(
    State(ctx): State<Arc<Ctx>>,
    Path((from_lang, to_lang, query)): Path<(String, String, String)>,
) -> impl IntoResponse {
    let mut context = base_context(&ctx);
    context.insert("page_type", "search");
    context.insert("query", &query);

    // Perform search
    let sq = SearchQuery {
        query: query.clone(),
        from_lang: from_lang.clone(),
        to_lang: to_lang.clone(),
        ..Default::default()
    };

    let per_page = ctx.consts.site_default_per_page;
    let max_relations = ctx.consts.site_max_relations_per_type;
    let max_content_items = ctx.consts.site_max_content_items;

    match ctx.mgr.search(&sq, 0, per_page).await {
        Ok((mut entries, total)) => {
            // Load relations for all entries with site-specific limits.
            if let Err(e) = ctx
                .mgr
                .load_relations(
                    &mut entries,
                    &to_lang,
                    &[],
                    &[],
                    STATUS_ENABLED,
                    max_relations,
                )
                .await
            {
                log::error!("error loading relations: {}", e);
            }

            // Apply content item limit.
            if max_content_items > 0 {
                for entry in &mut entries {
                    for rel in &mut entry.relations {
                        if rel.content.len() > max_content_items as usize {
                            rel.content.0.truncate(max_content_items as usize);
                        }
                    }
                }
            }

            // Hide internal IDs for public site.
            for e in &mut entries {
                e.id = 0;
                for r in &mut e.relations {
                    r.id = 0;
                    if let Some(rel) = &mut r.relation {
                        rel.id = 0;
                    }
                }
            }

            let results = crate::models::SearchResults {
                entries,
                page: 1,
                per_page,
                total,
                total_pages: ((total as f64) / (per_page as f64)).ceil() as i32,
            };
            context.insert("results", &results);
        }
        Err(e) => {
            log::error!("search error: {}", e);
            context.insert(
                "results",
                &crate::models::SearchResults {
                    entries: vec![],
                    page: 1,
                    per_page,
                    total: 0,
                    total_pages: 0,
                },
            );
        }
    }

    render(&ctx, "search.html", &context)
}

/// Glossary page.
pub async fn render_glossary_page(
    State(ctx): State<Arc<Ctx>>,
    Path((from_lang, to_lang, initial)): Path<(String, String, String)>,
    Query(params): Query<GlossaryParams>,
) -> impl IntoResponse {
    // Check if glossary is enabled.
    if !ctx.consts.enable_glossary {
        return (StatusCode::NOT_FOUND, "glossary is disabled").into_response();
    }

    let mut context = base_context(&ctx);
    context.insert("page_type", "glossary");
    context.insert("initial", &initial);

    // Get initials.
    match ctx.mgr.get_initials(&from_lang).await {
        Ok(initials) => context.insert("initials", &initials),
        Err(e) => {
            log::error!("initials error: {}", e);
            context.insert("initials", &Vec::<String>::new());
        }
    }

    // Use glossary pagination config.
    let page = params.page.unwrap_or(1);
    let per_page = params
        .per_page
        .unwrap_or(ctx.consts.glossary_default_per_page)
        .min(ctx.consts.glossary_max_per_page);
    let offset = (page - 1) * per_page;

    // Build base URL for pagination.
    let pg_url = format!(
        "{}/glossary/{}/{}/{}",
        ctx.consts.root_url, from_lang, to_lang, initial
    );
    context.insert("pg_url", &pg_url);

    match ctx
        .mgr
        .get_glossary_words(&from_lang, &initial, offset, per_page)
        .await
    {
        Ok((words, total)) => {
            let g = GlossaryData {
                words,
                from_lang: from_lang.clone(),
                to_lang: to_lang.clone(),
            };
            context.insert("glossary", &g);
            context.insert("page", &page);
            context.insert("per_page", &per_page);
            context.insert("total", &total);
            let total_pages = ((total as f64) / (per_page as f64)).ceil() as i32;
            context.insert("total_pages", &total_pages);
        }
        Err(e) => {
            log::error!("glossary error: {}", e);
            context.insert(
                "glossary",
                &GlossaryData {
                    words: vec![],
                    from_lang,
                    to_lang,
                },
            );
            context.insert("total_pages", &0);
        }
    }

    match render(&ctx, "glossary.html", &context) {
        Ok(html) => html.into_response(),
        Err(e) => e.into_response(),
    }
}

/// Submit new entry page.
pub async fn submit_form(State(ctx): State<Arc<Ctx>>) -> impl IntoResponse {
    if !ctx.consts.enable_submissions {
        return (StatusCode::NOT_FOUND, "submissions disabled").into_response();
    }
    let mut context = base_context(&ctx);
    context.insert("page_type", "submit");
    match render(&ctx, "submit-new.html", &context) {
        Ok(html) => html.into_response(),
        Err(e) => e.into_response(),
    }
}

/// Custom pages.
pub async fn render_custom_page(
    State(ctx): State<Arc<Ctx>>,
    Path(page): Path<String>,
) -> impl IntoResponse {
    // Check if custom pages are enabled.
    if !ctx.consts.enable_pages {
        return (StatusCode::NOT_FOUND, "custom pages are disabled").into_response();
    }

    let template = format!("pages/{}.html", page);
    let mut context = base_context(&ctx);
    context.insert("page_type", "page");
    context.insert("page_id", &page);

    // Check if template exists.
    match &ctx.site_tpl {
        Some(tpl) => {
            if tpl.get_template(&template).is_err() {
                return (StatusCode::NOT_FOUND, "page not found").into_response();
            }
        }
        None => return (StatusCode::NOT_FOUND, "site templates not loaded").into_response(),
    }

    match render(&ctx, &template, &context) {
        Ok(html) => html.into_response(),
        Err(e) => e.into_response(),
    }
}

/// Generic message page.
pub async fn message(
    State(ctx): State<Arc<Ctx>>,
    Query(params): Query<MessageParams>,
) -> impl IntoResponse {
    let mut context = base_context(&ctx);
    context.insert("page_type", "message");
    context.insert("title", &params.title.unwrap_or_default());
    context.insert("description", &params.message.unwrap_or_default());
    render(&ctx, "message.html", &context)
}
