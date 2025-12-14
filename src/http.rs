use std::{collections::HashMap, fs, path::PathBuf, sync::Arc};

use axum::{
    body::{Body, Bytes},
    extract::State,
    http::{header, Request, StatusCode},
    middleware::{self, Next},
    response::{IntoResponse, Response},
    routing::{delete, get, post, put},
    Router,
};
use base64::{engine::general_purpose::STANDARD, Engine};
use rust_embed::Embed;

use crate::handlers::{admin, entries, relations, search, site, submissions, Ctx};

// Embedded admin templates and static files.
#[derive(Embed)]
#[folder = "static/admin/"]
struct AdminStaticFiles;

#[derive(Embed)]
#[folder = "admin/"]
pub struct AdminTemplates;

/// Initialize HTTP routes.
pub fn init_handlers(ctx: Arc<Ctx>) -> Router {
    // Public API routes.
    let pub_routes = Router::new()
        .route("/api/config", get(admin::get_config))
        .route(
            "/api/dictionary/{fromLang}/{toLang}/{q}",
            get(search::search),
        )
        .route(
            "/api/dictionary/entries/{guid}",
            get(entries::get_entry_by_guid),
        );

    // Public submission routes (if enabled).
    let submit_routes = Router::new()
        .route("/api/submissions", post(submissions::insert_submission))
        .route(
            "/api/submissions/comments",
            post(submissions::insert_comment),
        );

    // Admin static (no auth required).
    let admin_static_routes = Router::new().route("/admin/static/{*path}", get(serve_admin_static));

    // Admin (requires auth).
    let admin_routes = Router::new()
        // Admin pages.
        .route("/admin", get(admin::render_index_page))
        .route("/admin/search", get(admin::render_search_page))
        .route("/admin/pending", get(admin::render_pending_page))
        // Admin API.
        .route("/api/stats", get(admin::get_stats))
        .route(
            "/api/entries/{fromLang}/{toLang}",
            get(search::search_admin),
        )
        .route(
            "/api/entries/pending",
            get(submissions::get_pending_entries),
        )
        .route(
            "/api/entries/pending",
            delete(submissions::delete_all_pending),
        )
        .route("/api/entries/comments", get(submissions::get_comments))
        .route(
            "/api/entries/comments/{id}",
            delete(submissions::delete_comment),
        )
        .route("/api/entries/{id}", get(entries::get_entry))
        .route(
            "/api/entries/{id}/parents",
            get(entries::get_parent_entries),
        )
        .route("/api/entries", post(entries::insert_entry))
        .route("/api/entries/{id}", put(entries::update_entry))
        .route("/api/entries/{id}", delete(entries::delete_entry))
        // Relation routes with separate path to avoid conflicts.
        .route(
            "/api/relations/{fromId}/{toId}",
            post(relations::insert_relation),
        )
        .route("/api/relations/{relId}", put(relations::update_relation))
        .route("/api/relations/{relId}", delete(relations::delete_relation))
        .route(
            "/api/entries/{id}/relations/weights",
            put(relations::reorder_relations),
        )
        .route(
            "/api/entries/{id}/submission",
            put(submissions::approve_submission),
        )
        .route(
            "/api/entries/{id}/submission",
            delete(submissions::reject_submission),
        )
        .route_layer(middleware::from_fn_with_state(ctx.clone(), auth_middleware));

    // Setup the router.
    let mut router = Router::new()
        .merge(pub_routes)
        .merge(submit_routes)
        .merge(admin_static_routes)
        .merge(admin_routes);

    // Add public site routes if site templates are loaded via the --site flag.
    if ctx.site_tpl.is_some() {
        let site_routes = Router::new()
            .route("/", get(site::index))
            .route("/dictionary/{from}/{to}/{q}", get(site::search))
            .route("/submit", get(site::render_submit_page))
            .route("/submit", post(site::submit_entry))
            .route("/message", get(site::message))
            .route("/static/_bundle.js", get(serve_bundle))
            .route("/static/_bundle.css", get(serve_bundle))
            .route("/static/{*path}", get(serve_site_static))
            .route(
                "/glossary/{from}/{to}/{initial}",
                get(site::render_glossary_page),
            )
            .route("/page/{page}", get(site::render_custom_page));

        router = router.merge(site_routes);
        log::info!("site routes enabled");
    } else {
        log::info!("site routes disabled (no --site flag, API-only mode)");
    }

    router.with_state(ctx)
}

/// BasicAuth middleware checks for admin username & password defined in ctx constants.
async fn auth_middleware(
    State(ctx): State<Arc<Ctx>>,
    request: Request<Body>,
    next: Next,
) -> Response {
    if validate_basic_auth(
        request.headers(),
        &ctx.consts.admin_username,
        &ctx.consts.admin_password,
    ) {
        return next.run(request).await;
    }

    (
        StatusCode::UNAUTHORIZED,
        [(header::WWW_AUTHENTICATE, "Basic realm=\"dictpress\"")],
        "unauthorized",
    )
        .into_response()
}

/// Validate BasicAuth credentials from request headers.
fn validate_basic_auth(headers: &header::HeaderMap, username: &str, password: &str) -> bool {
    let check = || {
        let hdr = headers.get(header::AUTHORIZATION)?.to_str().ok()?;
        let decoded = base64_decode(hdr.strip_prefix("Basic ")?).ok()?;
        let (user, pass) = decoded.split_once(':')?;
        Some(user == username && pass == password)
    };
    check().unwrap_or(false)
}

/// Serve embedded admin static files.
async fn serve_admin_static(
    axum::extract::Path(path): axum::extract::Path<String>,
) -> impl IntoResponse {
    let path = path.trim_start_matches('/');
    match AdminStaticFiles::get(path) {
        Some(content) => {
            let mime = mime_guess::from_path(path)
                .first_or_octet_stream()
                .to_string();
            (
                StatusCode::OK,
                [(header::CONTENT_TYPE, mime)],
                content.data.to_vec(),
            )
                .into_response()
        }
        None => (StatusCode::NOT_FOUND, "not found").into_response(),
    }
}

/// Serve a bundle by concatenating multiple files from preloaded static files.
/// Content-Type is determined by route extension (.js or .css).
async fn serve_bundle(
    State(ctx): State<Arc<Ctx>>,
    uri: axum::http::Uri,
    axum::extract::Query(params): axum::extract::Query<Vec<(String, String)>>,
) -> impl IntoResponse {
    let r#type = if uri.path().ends_with(".css") {
        "text/css"
    } else {
        "application/javascript"
    };

    // Get file names from the ?f query param.
    let files: Vec<&str> = params
        .iter()
        .filter(|(k, _)| k == "f")
        .map(|(_, v)| v.as_str())
        .filter(|s| !s.is_empty())
        .collect();

    // Lookup all files first (fail fast if any missing).
    let mut parts = Vec::with_capacity(files.len());
    for name in &files {
        match ctx.static_files.get(*name) {
            Some(b) => parts.push(b.clone()), // Bytes::clone is cheap (refcount bump)
            None => {
                return (
                    StatusCode::NOT_FOUND,
                    [(header::CONTENT_TYPE, "text/plain")],
                    format!("File not found: {}", name),
                )
                    .into_response();
            }
        }
    }

    // Concatenate into a single buffer with exact capacity.
    let len: usize = parts.iter().map(|b| b.len()).sum::<usize>() + parts.len().saturating_sub(1);
    let mut buf = Vec::with_capacity(len);
    for (i, b) in parts.iter().enumerate() {
        buf.extend_from_slice(b);
        if i + 1 < parts.len() {
            buf.push(b'\n');
        }
    }

    (StatusCode::OK, [(header::CONTENT_TYPE, r#type)], buf).into_response()
}

/// Serve site static files from disk (--site directory).
async fn serve_site_static(
    State(ctx): State<Arc<Ctx>>,
    axum::extract::Path(path): axum::extract::Path<String>,
) -> impl IntoResponse {
    let uri = path.trim_start_matches('/');

    // Site static files are only served from disk (--site directory).
    if let Some(ref site_dir) = ctx.site_path {
        let file_path = site_dir.join("static").join(uri);
        if file_path.exists() {
            match std::fs::read(&file_path) {
                Ok(content) => {
                    let mime = mime_guess::from_path(uri)
                        .first_or_octet_stream()
                        .to_string();
                    return (StatusCode::OK, [(header::CONTENT_TYPE, mime)], content)
                        .into_response();
                }
                Err(_) => return (StatusCode::NOT_FOUND, "not found").into_response(),
            }
        }
    }

    (StatusCode::NOT_FOUND, "not found").into_response()
}

/// Preload static files (JS & CSS) for bundling.
pub fn preload_static_files(site_path: &Option<PathBuf>) -> HashMap<String, Bytes> {
    let site_dir = match site_path {
        Some(p) => p,
        None => return HashMap::new(),
    };

    let static_dir = site_dir.join("static");

    // Prealloc for "a few js/css files"
    let mut files = HashMap::with_capacity(8);

    let entries = match fs::read_dir(&static_dir) {
        Ok(e) => e,
        Err(_) => return files,
    };

    for entry in entries.filter_map(|e| e.ok()) {
        let path = entry.path();

        // Only accept .js or .css
        let _ = match path.extension().and_then(|e| e.to_str()) {
            Some(e) if matches!(e, "js" | "css") => e,
            _ => continue,
        };

        let name = match path.file_name().and_then(|n| n.to_str()) {
            Some(n) => n.to_owned(),
            None => continue,
        };

        if let Ok(content) = fs::read(&path) {
            files.insert(name, Bytes::from(content));
        }
    }

    files
}

fn base64_decode(s: &str) -> Result<String, ()> {
    let bytes = STANDARD.decode(s).map_err(|_| ())?;
    String::from_utf8(bytes).map_err(|_| ())
}
