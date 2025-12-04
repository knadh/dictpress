use std::sync::Arc;

use axum::{
    extract::{Path, State},
    Json,
};

use super::{json, ApiErr, ApiResp, Ctx, Result};
use crate::models::{Relation, StringArray};

/// Relation create/update request.
#[derive(Debug, serde::Deserialize)]
pub struct RelationReq {
    #[serde(default)]
    pub types: Vec<String>,
    #[serde(default)]
    pub tags: Vec<String>,
    #[serde(default)]
    pub notes: String,
    #[serde(default)]
    pub weight: f64,
    #[serde(default)]
    pub status: String,
}

impl From<RelationReq> for Relation {
    fn from(req: RelationReq) -> Self {
        Relation {
            types: StringArray(req.types),
            tags: StringArray(req.tags),
            notes: req.notes,
            weight: req.weight,
            status: req.status,
            ..Default::default()
        }
    }
}

/// POST /api/relations/:fromId/:toId - Create relation.
pub async fn create_relation(
    State(ctx): State<Arc<Ctx>>,
    Path((from_id, to_id)): Path<(i64, i64)>,
    Json(req): Json<RelationReq>,
) -> Result<ApiResp<i64>> {
    if from_id == to_id {
        return Err(ApiErr::bad_request("from_id and to_id cannot be the same"));
    }

    let relation: Relation = req.into();
    let id = ctx.mgr.insert_relation(from_id, to_id, &relation).await?;

    Ok(json(id))
}

/// PUT /api/relations/:relId - Update relation.
pub async fn update_relation(
    State(ctx): State<Arc<Ctx>>,
    Path(rel_id): Path<i64>,
    Json(req): Json<RelationReq>,
) -> Result<ApiResp<bool>> {
    let relation: Relation = req.into();
    ctx.mgr.update_relation(rel_id, &relation).await?;

    Ok(json(true))
}

/// DELETE /api/relations/:relId - Delete relation.
pub async fn delete_relation(
    State(ctx): State<Arc<Ctx>>,
    Path(rel_id): Path<i64>,
) -> Result<ApiResp<bool>> {
    ctx.mgr.delete_relation(rel_id).await?;
    Ok(json(true))
}

/// Reorder relations request.
#[derive(Debug, serde::Deserialize)]
pub struct ReorderReq {
    pub ids: Vec<i64>,
}

/// PUT /api/entries/:id/relations/weights - Reorder relations.
pub async fn reorder_relations(
    State(ctx): State<Arc<Ctx>>,
    Path(_entry_id): Path<i64>,
    Json(req): Json<ReorderReq>,
) -> Result<ApiResp<bool>> {
    ctx.mgr.reorder_relations(&req.ids).await?;
    Ok(json(true))
}
