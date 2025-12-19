use std::sync::Arc;

use axum::{
    extract::{Path, Query, State},
    http::StatusCode,
    response::{Html, IntoResponse},
};
use axum_extra::extract::Form;

use super::search::do_search;
use super::{clean_query, paginate, Ctx};
use crate::cache::make_glossary_cache_key;
use crate::models::{
    Entry, GlossaryWord, Relation, SearchQuery, SearchResults, StringArray, STATUS_PENDING,
};

#[derive(serde::Deserialize)]
pub struct GlossaryParams {
    page: Option<i32>,
    per_page: Option<i32>,
}

#[derive(serde::Serialize)]
struct GlossaryData {
    words: Vec<GlossaryWord>,
    from_lang: String,
    to_lang: String,
}

/// Cached glossary response (words + total).
#[derive(serde::Serialize, serde::Deserialize)]
struct CachedGlossary {
    words: Vec<GlossaryWord>,
    total: i64,
}

#[derive(serde::Deserialize)]
pub struct MessageParams {
    title: Option<String>,
    message: Option<String>,
}

/// Form data for public entry submission.
#[derive(serde::Deserialize)]
pub struct SubmitForm {
    pub entry_lang: String,
    pub entry_content: String,
    #[serde(default)]
    pub entry_phones: String,
    #[serde(default)]
    pub entry_notes: String,
    #[serde(default)]
    pub relation_lang: Vec<String>,
    #[serde(default)]
    pub relation_content: Vec<String>,
    #[serde(default)]
    pub relation_type: Vec<String>,
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
        None => Err((StatusCode::NOT_FOUND, "no site templates found".to_string())),
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
    // Clean the query for display in the template.
    let query = clean_query(&query);

    let mut tpl_ctx = base_context(&ctx);
    tpl_ctx.insert("page_type", "search");
    tpl_ctx.insert("query", &query);

    // Pagination.
    let (page, per_page, offset) = paginate(
        0,
        0,
        ctx.consts.site_max_per_page,
        ctx.consts.site_default_per_page,
    );

    // Build query with site-specific limits.
    let q = SearchQuery {
        query: query.clone(),
        from_lang: from_lang.clone(),
        to_lang: to_lang.clone(),
        page,
        offset,
        limit: per_page,
        max_relations: ctx.consts.site_max_relations_per_type,
        max_content_items: ctx.consts.site_max_content_items,
        ..Default::default()
    };

    let results = match do_search(ctx.clone(), q, false).await {
        Ok(results) => results,
        Err(e) => {
            log::error!("error searching: {}", e.message);
            SearchResults {
                entries: vec![],
                page: 1,
                per_page,
                total: 0,
                total_pages: 0,
            }
        }
    };

    tpl_ctx.insert("results", &results);
    render(&ctx, "search.html", &tpl_ctx)
}

/// Glossary page.
pub async fn render_glossary_page(
    State(context): State<Arc<Ctx>>,
    Path((from_lang, to_lang, initial)): Path<(String, String, String)>,
    Query(params): Query<GlossaryParams>,
) -> impl IntoResponse {
    // Check if glossary is enabled.
    if !context.consts.enable_glossary {
        return (StatusCode::NOT_FOUND, "glossary is disabled").into_response();
    }

    let mut ctx = base_context(&context);
    ctx.insert("page_type", "glossary");
    ctx.insert("initial", &initial);

    // Get initials.
    match context.mgr.get_initials(&from_lang).await {
        Ok(initials) => ctx.insert("initials", &initials),
        Err(e) => {
            log::error!("initials error: {}", e);
            ctx.insert("initials", &Vec::<String>::new());
        }
    }

    // Use glossary pagination config.
    let page = params.page.unwrap_or(1);
    let per_page = params
        .per_page
        .unwrap_or(context.consts.glossary_default_per_page)
        .min(context.consts.glossary_max_per_page);
    let offset = (page - 1) * per_page;

    // Build base URL for pagination.
    let pg_url = format!(
        "{}/glossary/{}/{}/{}",
        context.consts.root_url, from_lang, to_lang, initial
    );
    ctx.insert("pg_url", &pg_url);

    // Fetch glossary words (from cache or DB).
    let (words, total) =
        match get_glossary_words(&context, &from_lang, &initial, offset, per_page).await {
            Ok(result) => result,
            Err(e) => {
                log::error!("glossary error: {}", e);
                ctx.insert(
                    "glossary",
                    &GlossaryData {
                        words: vec![],
                        from_lang,
                        to_lang,
                    },
                );
                ctx.insert("total_pages", &0);
                return match render(&context, "glossary.html", &ctx) {
                    Ok(html) => html.into_response(),
                    Err(e) => e.into_response(),
                };
            }
        };

    let total_pages = ((total as f64) / (per_page as f64)).ceil() as i32;
    ctx.insert(
        "glossary",
        &GlossaryData {
            words,
            from_lang: from_lang.clone(),
            to_lang: to_lang.clone(),
        },
    );
    ctx.insert("page", &page);
    ctx.insert("per_page", &per_page);
    ctx.insert("total", &total);
    ctx.insert("total_pages", &total_pages);

    match render(&context, "glossary.html", &ctx) {
        Ok(html) => html.into_response(),
        Err(e) => e.into_response(),
    }
}

/// Submit new entry page.
pub async fn render_submit_page(State(context): State<Arc<Ctx>>) -> impl IntoResponse {
    if !context.consts.enable_submissions {
        return (StatusCode::NOT_FOUND, "submissions disabled").into_response();
    }

    let mut ctx = base_context(&context);
    ctx.insert("page_type", "submit");

    match render(&context, "submit-new.html", &ctx) {
        Ok(html) => html.into_response(),
        Err(e) => e.into_response(),
    }
}

/// Custom pages.
pub async fn render_custom_page(
    State(context): State<Arc<Ctx>>,
    Path(page): Path<String>,
) -> impl IntoResponse {
    // Check if custom pages are enabled.
    if !context.consts.enable_pages {
        return (StatusCode::NOT_FOUND, "custom pages are disabled").into_response();
    }

    let template = format!("pages/{}.html", page);
    let mut ctx = base_context(&context);
    ctx.insert("page_type", "page");
    ctx.insert("page_id", &page);

    // Check if template exists.
    match &context.site_tpl {
        Some(tpl) => {
            if tpl.get_template(&template).is_err() {
                return (StatusCode::NOT_FOUND, "page not found").into_response();
            }
        }
        None => return (StatusCode::NOT_FOUND, "site templates not loaded").into_response(),
    }

    match render(&context, &template, &ctx) {
        Ok(html) => html.into_response(),
        Err(e) => e.into_response(),
    }
}

/// Generic message page.
pub async fn message(
    State(context): State<Arc<Ctx>>,
    Query(params): Query<MessageParams>,
) -> impl IntoResponse {
    let mut ctx = base_context(&context);
    ctx.insert("page_type", "message");
    ctx.insert("title", &params.title.unwrap_or_default());
    ctx.insert("description", &params.message.unwrap_or_default());

    render(&context, "message.html", &ctx)
}

/// Handle public entry submission.
pub async fn submit_entry(
    State(ctx): State<Arc<Ctx>>,
    Form(form): Form<SubmitForm>,
) -> impl IntoResponse {
    if !ctx.consts.enable_submissions {
        return render_message(&ctx, "Error", "Submissions are disabled.");
    }

    // Validate entry content.
    let entry_content = form.entry_content.trim();
    if entry_content.is_empty() {
        return render_message(&ctx, "Error", "Entry content is required.");
    }

    let rel_len = form.relation_lang.len();
    if rel_len == 0 || rel_len != form.relation_content.len() || rel_len != form.relation_type.len()
    {
        return render_message(&ctx, "Error", "Invalid submission fields.");
    }

    if !ctx.langs.contains_key(&form.entry_lang) {
        return render_message(&ctx, "Error", "Unknown entry language.");
    }

    // Validate relation languages and types.
    for i in 0..rel_len {
        if !ctx.langs.contains_key(&form.relation_lang[i]) {
            return render_message(&ctx, "Error", "Unknown relation language.");
        }
        let rel_content = form.relation_content[i].trim();
        if rel_content.is_empty() {
            return render_message(&ctx, "Error", "Relation content is required.");
        }
    }

    // Parse phones (comma-separated).
    let phones: Vec<String> = form
        .entry_phones
        .split(',')
        .map(|s| s.trim().to_string())
        .filter(|s| !s.is_empty())
        .collect();

    // Compute initial from first character.
    let initial = entry_content
        .chars()
        .next()
        .map(|c| c.to_uppercase().to_string())
        .unwrap_or_default();

    // Create main entry.
    let entry = Entry {
        content: vec![entry_content.to_string()].into(),
        initial,
        lang: form.entry_lang.clone(),
        phones: phones.into(),
        notes: form.entry_notes.clone(),
        status: STATUS_PENDING.to_string(),
        ..Default::default()
    };

    // Insert main entry.
    let from_id = match ctx.mgr.insert_submission_entry(&entry).await {
        Ok(Some(id)) => id,
        Ok(None) => {
            return render_message(&ctx, "Error", "Entry already exists.");
        }
        Err(e) => {
            log::error!("error inserting submission entry: {}", e);
            return render_message(&ctx, "Error", "Error saving entry.");
        }
    };

    // Insert relations.
    for i in 0..rel_len {
        let rel_content = form.relation_content[i].trim();
        let rel_initial = rel_content
            .chars()
            .next()
            .map(|c| c.to_uppercase().to_string())
            .unwrap_or_default();

        let rel_entry = Entry {
            content: vec![rel_content.to_string()].into(),
            initial: rel_initial,
            lang: form.relation_lang[i].clone(),
            status: STATUS_PENDING.to_string(),
            ..Default::default()
        };

        let to_id = match ctx.mgr.insert_submission_entry(&rel_entry).await {
            Ok(Some(id)) => id,
            Ok(None) => continue, // Entry exists, skip relation
            Err(e) => {
                log::error!("error inserting submission definition: {}", e);
                return render_message(&ctx, "Error", "Error saving definition.");
            }
        };

        let relation = Relation {
            types: StringArray(vec![form.relation_type[i].clone()]),
            status: STATUS_PENDING.to_string(),
            ..Default::default()
        };

        if let Err(e) = ctx
            .mgr
            .insert_submission_relation(from_id, to_id, &relation)
            .await
        {
            log::error!("error inserting submission relation: {}", e);
            return render_message(&ctx, "Error", "Error saving relation.");
        }
    }

    render_message(
        &ctx,
        "Submitted",
        "Your entry has been submitted for review.",
    )
}

/// Fetch glossary words from cache or DB. Caches result if cache is enabled.
async fn get_glossary_words(
    ctx: &Ctx,
    lang: &str,
    initial: &str,
    offset: i32,
    limit: i32,
) -> Result<(Vec<GlossaryWord>, i64), Box<dyn std::error::Error + Send + Sync>> {
    // Try cache first if it's enabled.
    if let Some(cache) = &ctx.cache {
        let key = make_glossary_cache_key(lang, initial, offset, limit);
        if let Some(data) = cache.get(&key).await {
            if let Ok(cached) = rmp_serde::from_slice::<CachedGlossary>(&data) {
                return Ok((cached.words, cached.total));
            }
        }
    }

    // Fetch from DB.
    let (words, total) = ctx
        .mgr
        .get_glossary_words(lang, initial, offset, limit)
        .await?;

    // Cache the result if caching is enabled.
    if let Some(cache) = &ctx.cache {
        let key = make_glossary_cache_key(lang, initial, offset, limit);
        let cached = CachedGlossary {
            words: words.clone(),
            total,
        };
        if let Ok(encoded) = rmp_serde::to_vec_named(&cached) {
            cache.put(&key, &encoded);
        }
    }

    Ok((words, total))
}

/// Helper to render the message template.
fn render_message(context: &Ctx, title: &str, description: &str) -> impl IntoResponse {
    let mut ctx = base_context(context);
    ctx.insert("page_type", "message");
    ctx.insert("title", title);
    ctx.insert("description", description);

    match render(context, "message.html", &ctx) {
        Ok(html) => html.into_response(),
        Err(e) => e.into_response(),
    }
}
