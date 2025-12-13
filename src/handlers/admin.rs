use std::sync::Arc;

use axum::{
    extract::State,
    http::StatusCode,
    response::{Html, IntoResponse},
};

use super::{json, ApiResp, Ctx, Result};
use crate::models::Stats;

#[derive(serde::Serialize)]
pub struct ConfigResp {
    pub langs: Vec<LangResp>,
    pub dicts: Vec<[String; 2]>,
}

#[derive(serde::Serialize)]
pub struct LangResp {
    pub id: String,
    pub name: String,
    pub types: std::collections::HashMap<String, String>,
}

/// Get database stats.
pub async fn get_stats(State(ctx): State<Arc<Ctx>>) -> Result<ApiResp<Stats>> {
    let stats = ctx.mgr.get_stats().await?;
    Ok(json(stats))
}

/// Get public config.
pub async fn get_config(State(ctx): State<Arc<Ctx>>) -> Result<ApiResp<ConfigResp>> {
    let out = ConfigResp {
        langs: ctx
            .langs
            .iter()
            .map(|(id, l)| LangResp {
                id: id.clone(),
                name: l.name.clone(),
                types: l.types.clone(),
            })
            .collect(),
        dicts: ctx
            .dicts
            .iter()
            .map(|(from, to)| [from.id.clone(), to.id.clone()])
            .collect(),
    };

    Ok(json(out))
}

/// Admin index.
pub async fn render_index_page(State(ctx): State<Arc<Ctx>>) -> impl IntoResponse {
    render_admin(&ctx, "admin/index.html", "")
}

/// Admin search page.
pub async fn render_search_page(State(ctx): State<Arc<Ctx>>) -> impl IntoResponse {
    render_admin(&ctx, "admin/search.html", "Search")
}

/// Admin pending page.
pub async fn render_pending_page(State(ctx): State<Arc<Ctx>>) -> impl IntoResponse {
    render_admin(&ctx, "admin/pending.html", "Pending")
}

/// Render admin page with Tera (using embedded admin templates).
fn render_admin(
    context: &Ctx,
    template: &str,
    title: &str,
) -> std::result::Result<Html<String>, impl IntoResponse> {
    let mut ctx = tera::Context::new();
    ctx.insert("title", title);
    ctx.insert("asset_ver", &context.asset_ver);
    ctx.insert("consts", &context.consts);

    context
        .admin_tpl
        .render(template, &ctx)
        .map(Html)
        .map_err(|e| {
            log::error!("template error: {}", e);
            (
                StatusCode::INTERNAL_SERVER_ERROR,
                format!("template error: {}", e),
            )
        })
}
