use std::sync::Arc;

use axum::{
    extract::State,
    http::StatusCode,
    response::{Html, IntoResponse},
};

use super::{json, ApiResp, Ctx, Result};
use crate::models::Stats;

/// GET /api/stats - Get database stats.
pub async fn get_stats(State(ctx): State<Arc<Ctx>>) -> Result<ApiResp<Stats>> {
    let stats = ctx.mgr.get_stats().await?;
    Ok(json(stats))
}

/// GET /api/config - Get public config (languages, dicts).
pub async fn get_config(State(ctx): State<Arc<Ctx>>) -> Result<ApiResp<ConfigResp>> {
    let resp = ConfigResp {
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
    Ok(json(resp))
}

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

/// Render admin page with Tera (using embedded admin templates).
fn render_admin(
    ctx: &Ctx,
    template: &str,
    title: &str,
) -> std::result::Result<Html<String>, impl IntoResponse> {
    let mut context = tera::Context::new();
    context.insert("title", title);
    context.insert("asset_ver", &ctx.asset_ver);
    context.insert("consts", &ctx.consts);

    ctx.admin_tpl
        .render(template, &context)
        .map(Html)
        .map_err(|e| {
            log::error!("template error: {}", e);
            (
                StatusCode::INTERNAL_SERVER_ERROR,
                format!("template error: {}", e),
            )
        })
}

/// GET /admin - Admin index.
pub async fn admin_index(State(ctx): State<Arc<Ctx>>) -> impl IntoResponse {
    render_admin(&ctx, "admin/index.html", "")
}

/// GET /admin/search - Admin search page.
pub async fn admin_search(State(ctx): State<Arc<Ctx>>) -> impl IntoResponse {
    render_admin(&ctx, "admin/search.html", "Search")
}

/// GET /admin/pending - Admin pending page.
pub async fn admin_pending(State(ctx): State<Arc<Ctx>>) -> impl IntoResponse {
    render_admin(&ctx, "admin/pending.html", "Pending")
}
