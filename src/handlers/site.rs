use std::sync::Arc;

use axum::{
    extract::{Path, Query, State},
    http::StatusCode,
    response::{Html, IntoResponse},
};

use super::Ctx;
use crate::models::{SearchQuery, STATUS_ENABLED};

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
            log::error!("template error: {}", e);
            (
                StatusCode::INTERNAL_SERVER_ERROR,
                format!("template error: {}", e),
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

    // Perform search.
    let sq = SearchQuery {
        query: query.clone(),
        from_lang: from_lang.clone(),
        to_lang: to_lang.clone(),
        ..Default::default()
    };

    match ctx.mgr.search(&sq, 0, 50).await {
        Ok((mut entries, total)) => {
            // Load relations for all entries.
            if let Err(e) = ctx
                .mgr
                .load_relations(&mut entries, &to_lang, &[], &[], STATUS_ENABLED)
                .await
            {
                log::error!("error loading relations: {}", e);
            }

            let results = crate::models::SearchResults {
                entries,
                page: 1,
                per_page: 50,
                total,
                total_pages: ((total as f64) / 50.0).ceil() as i32,
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
                    per_page: 50,
                    total: 0,
                    total_pages: 0,
                },
            );
        }
    }

    render(&ctx, "search.html", &context)
}

/// Glossary page.
pub async fn glossary(
    State(ctx): State<Arc<Ctx>>,
    Path((from_lang, to_lang, initial)): Path<(String, String, String)>,
    Query(params): Query<GlossaryParams>,
) -> impl IntoResponse {
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

    // Get words for this initial.
    let page = params.page.unwrap_or(1);
    let per_page = params.per_page.unwrap_or(100);
    match ctx
        .mgr
        .get_glossary_words(&from_lang, &initial, page, per_page)
        .await
    {
        Ok((words, _total)) => {
            let g = GlossaryData {
                words,
                from_lang: from_lang.clone(),
                to_lang: to_lang.clone(),
            };
            context.insert("glossary", &g);
            context.insert("pg_bar", ""); // Pagination bar (simplified).
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
        }
    }

    render(&ctx, "glossary.html", &context)
}

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
pub async fn custom_page(
    State(ctx): State<Arc<Ctx>>,
    Path(page): Path<String>,
) -> impl IntoResponse {
    let template = format!("pages/{}.html", page);
    let mut context = base_context(&ctx);
    context.insert("page_type", "page");

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

#[derive(serde::Deserialize)]
pub struct MessageParams {
    title: Option<String>,
    message: Option<String>,
}
