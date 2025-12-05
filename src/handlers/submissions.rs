use std::sync::Arc;

use axum::{
    extract::{Path, Query, State},
    http::StatusCode,
    Json,
};

use super::{json, paginate, search::GlossaryQuery, total_pages, ApiErr, ApiResp, Ctx, Result};
use crate::models::{Comment, Entry, Relation, SearchResults, StringArray, STATUS_PENDING};

/// Public submission request.
#[derive(Debug, serde::Deserialize)]
pub struct SubmissionReq {
    /// Main entry content.
    pub content: Vec<String>,
    pub lang: String,
    #[serde(default)]
    pub initial: String,
    #[serde(default)]
    pub tags: Vec<String>,
    #[serde(default)]
    pub phones: Vec<String>,
    #[serde(default)]
    pub notes: String,

    /// Related entry (definition).
    #[serde(default)]
    pub relation_content: Vec<String>,
    #[serde(default)]
    pub relation_lang: String,
    #[serde(default)]
    pub relation_types: Vec<String>,
    #[serde(default)]
    pub relation_tags: Vec<String>,
    #[serde(default)]
    pub relation_notes: String,
}

/// POST /api/submissions - Submit new entry+relation.
pub async fn create_submission(
    State(ctx): State<Arc<Ctx>>,
    Json(req): Json<SubmissionReq>,
) -> Result<ApiResp<bool>> {
    if !ctx.consts.enable_submissions {
        return Err(ApiErr::new("submissions are disabled", StatusCode::BAD_REQUEST));
    }

    if req.content.is_empty() {
        return Err(ApiErr::new("content is required", StatusCode::BAD_REQUEST));
    }
    if req.lang.is_empty() {
        return Err(ApiErr::new("lang is required", StatusCode::BAD_REQUEST));
    }

    // Create main entry.
    let entry = Entry {
        content: req.content.into(),
        initial: req.initial,
        lang: req.lang.clone(),
        tags: req.tags.into(),
        phones: req.phones.into(),
        notes: req.notes,
        status: STATUS_PENDING.to_string(),
        ..Default::default()
    };

    let from_id = ctx
        .mgr
        .insert_submission_entry(&entry)
        .await?
        .ok_or_else(|| ApiErr::new("entry already exists", StatusCode::BAD_REQUEST))?;

    // Create relation entry if provided.
    if !req.relation_content.is_empty() {
        let rel_entry = Entry {
            content: req.relation_content.into(),
            lang: if req.relation_lang.is_empty() {
                req.lang
            } else {
                req.relation_lang
            },
            status: STATUS_PENDING.to_string(),
            ..Default::default()
        };

        if let Some(to_id) = ctx.mgr.insert_submission_entry(&rel_entry).await? {
            let relation = Relation {
                types: StringArray(req.relation_types),
                tags: StringArray(req.relation_tags),
                notes: req.relation_notes,
                status: STATUS_PENDING.to_string(),
                ..Default::default()
            };

            ctx.mgr
                .insert_submission_relation(from_id, to_id, &relation)
                .await?;
        }
    }

    Ok(json(true))
}

/// Comment submission request.
#[derive(Debug, serde::Deserialize)]
pub struct CommentReq {
    pub from_guid: String,
    #[serde(default)]
    pub to_guid: String,
    pub comments: String,
}

/// POST /api/submissions/comments - Submit comment.
pub async fn create_comment(
    State(ctx): State<Arc<Ctx>>,
    Json(req): Json<CommentReq>,
) -> Result<ApiResp<bool>> {
    if !ctx.consts.enable_submissions {
        return Err(ApiErr::new("submissions are disabled", StatusCode::BAD_REQUEST));
    }

    if req.from_guid.is_empty() {
        return Err(ApiErr::new("from_guid is required", StatusCode::BAD_REQUEST));
    }
    if req.comments.is_empty() {
        return Err(ApiErr::new("comments is required", StatusCode::BAD_REQUEST));
    }

    ctx.mgr
        .insert_comment(&req.from_guid, &req.to_guid, &req.comments)
        .await?;

    Ok(json(true))
}

/// GET /api/entries/pending - Get pending entries.
pub async fn get_pending_entries(
    State(ctx): State<Arc<Ctx>>,
    Query(query): Query<GlossaryQuery>,
) -> Result<ApiResp<SearchResults>> {
    let (page, per_page, offset) = paginate(
        query.page,
        query.per_page,
        ctx.consts.max_per_page,
        ctx.consts.default_per_page,
    );

    let (entries, total) = ctx.mgr.get_pending_entries("", offset, per_page).await?;

    Ok(json(SearchResults {
        entries,
        page,
        per_page,
        total,
        total_pages: total_pages(total, per_page),
    }))
}

/// GET /api/entries/comments - Get all comments.
pub async fn get_comments(State(ctx): State<Arc<Ctx>>) -> Result<ApiResp<Vec<Comment>>> {
    let comments = ctx.mgr.get_comments().await?;
    Ok(json(comments))
}

/// DELETE /api/entries/comments/:id - Delete comment.
pub async fn delete_comment(
    State(ctx): State<Arc<Ctx>>,
    Path(id): Path<i64>,
) -> Result<ApiResp<bool>> {
    ctx.mgr.delete_comment(id).await?;
    Ok(json(true))
}

/// DELETE /api/entries/pending - Delete all pending.
pub async fn delete_all_pending(State(ctx): State<Arc<Ctx>>) -> Result<ApiResp<bool>> {
    ctx.mgr.delete_all_pending().await?;
    Ok(json(true))
}

/// PUT /api/entries/:id/submission - Approve submission.
pub async fn approve_submission(
    State(ctx): State<Arc<Ctx>>,
    Path(id): Path<i64>,
) -> Result<ApiResp<bool>> {
    ctx.mgr.approve_submission(id).await?;
    Ok(json(true))
}

/// DELETE /api/entries/:id/submission - Reject submission.
pub async fn reject_submission(
    State(ctx): State<Arc<Ctx>>,
    Path(id): Path<i64>,
) -> Result<ApiResp<bool>> {
    ctx.mgr.reject_submission(id).await?;
    Ok(json(true))
}
