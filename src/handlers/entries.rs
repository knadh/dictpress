use std::sync::Arc;

use axum::{
    extract::{Path, State},
    Json,
};

use super::{json, ApiErr, ApiResp, Ctx, Result};
use crate::models::Entry;

/// GET /api/entries/:id - Get entry by ID.
pub async fn get_entry(State(ctx): State<Arc<Ctx>>, Path(id): Path<i64>) -> Result<ApiResp<Entry>> {
    let mut entry = ctx.mgr.get_entry(id, "").await.map_err(|e| {
        if matches!(e, crate::manager::Error::NotFound) {
            ApiErr::not_found("entry not found")
        } else {
            ApiErr::internal(e.to_string())
        }
    })?;

    // Load relations.
    let mut entries = vec![entry];
    ctx.mgr
        .load_relations(&mut entries, "", &[], &[], "")
        .await?;
    entry = entries.remove(0);

    Ok(json(entry))
}

/// GET /api/entries/guid/:guid - Get entry by GUID (public).
pub async fn get_entry_by_guid(
    State(ctx): State<Arc<Ctx>>,
    Path(guid): Path<String>,
) -> Result<ApiResp<Entry>> {
    let mut entry = ctx.mgr.get_entry(0, &guid).await.map_err(|e| {
        if matches!(e, crate::manager::Error::NotFound) {
            ApiErr::not_found("entry not found")
        } else {
            ApiErr::internal(e.to_string())
        }
    })?;

    // Load relations.
    let mut entries = vec![entry];
    ctx.mgr
        .load_relations(&mut entries, "", &[], &[], "")
        .await?;
    entry = entries.remove(0);

    // Hide internal IDs.
    entry.id = 0;
    for r in &mut entry.relations {
        r.id = 0;
        if let Some(rel) = &mut r.relation {
            rel.id = 0;
        }
    }

    Ok(json(entry))
}

/// GET /api/entries/:id/parents - Get parent entries.
pub async fn get_parent_entries(
    State(ctx): State<Arc<Ctx>>,
    Path(id): Path<i64>,
) -> Result<ApiResp<Vec<Entry>>> {
    let entries = ctx.mgr.get_parent_entries(id).await?;
    Ok(json(entries))
}

/// Entry creation/update request.
#[derive(Debug, serde::Deserialize)]
pub struct EntryReq {
    #[serde(default)]
    pub content: Vec<String>,
    #[serde(default)]
    pub initial: String,
    #[serde(default)]
    pub weight: f64,
    #[serde(default)]
    pub tokens: String,
    pub lang: String,
    #[serde(default)]
    pub tags: Vec<String>,
    #[serde(default)]
    pub phones: Vec<String>,
    #[serde(default)]
    pub notes: String,
    #[serde(default)]
    pub meta: serde_json::Value,
    #[serde(default)]
    pub status: String,
}

impl From<EntryReq> for Entry {
    fn from(req: EntryReq) -> Self {
        Entry {
            content: req.content.into(),
            initial: req.initial,
            weight: req.weight,
            tokens: req.tokens,
            lang: req.lang,
            tags: req.tags.into(),
            phones: req.phones.into(),
            notes: req.notes,
            meta: req.meta,
            status: req.status,
            ..Default::default()
        }
    }
}

/// POST /api/entries - Create entry.
pub async fn create_entry(
    State(ctx): State<Arc<Ctx>>,
    Json(req): Json<EntryReq>,
) -> Result<ApiResp<Entry>> {
    if req.content.is_empty() {
        return Err(ApiErr::bad_request("content is required"));
    }
    if req.lang.is_empty() {
        return Err(ApiErr::bad_request("lang is required"));
    }

    let entry: Entry = req.into();
    let id = ctx.mgr.insert_entry(&entry).await?;
    let entry = ctx.mgr.get_entry(id, "").await?;

    Ok(json(entry))
}

/// PUT /api/entries/:id - Update entry.
pub async fn update_entry(
    State(ctx): State<Arc<Ctx>>,
    Path(id): Path<i64>,
    Json(req): Json<EntryReq>,
) -> Result<ApiResp<Entry>> {
    let entry: Entry = req.into();
    ctx.mgr.update_entry(id, &entry).await?;
    let entry = ctx.mgr.get_entry(id, "").await?;

    Ok(json(entry))
}

/// DELETE /api/entries/:id - Delete entry.
pub async fn delete_entry(
    State(ctx): State<Arc<Ctx>>,
    Path(id): Path<i64>,
) -> Result<ApiResp<bool>> {
    ctx.mgr.delete_entry(id).await?;
    Ok(json(true))
}
