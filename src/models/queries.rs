use lazy_static::lazy_static;
use yesqlr_macros::ScanQueries;

const SQL_SCHEMA: &[u8] = include_bytes!("../../static/sql/schema.sql");
const SQL_QUERIES: &[u8] = include_bytes!("../../static/sql/queries.sql");

/// Parsed SQL schema.
#[derive(Default, ScanQueries)]
pub struct Schema {
    pub pragma: yesqlr::Query,
    pub schema: yesqlr::Query,
}

/// Parsed SQL queries.
#[derive(Default, ScanQueries)]
pub struct Queries {
    pub search: yesqlr::Query,
    #[name = "search-relations"]
    pub search_relations: yesqlr::Query,
    #[name = "get-entry"]
    pub get_entry: yesqlr::Query,
    #[name = "get-parent-relations"]
    pub get_parent_relations: yesqlr::Query,
    #[name = "get-initials"]
    pub get_initials: yesqlr::Query,
    #[name = "get-glossary-words"]
    pub get_glossary_words: yesqlr::Query,
    #[name = "insert-entry"]
    pub insert_entry: yesqlr::Query,
    #[name = "update-entry"]
    pub update_entry: yesqlr::Query,
    #[name = "delete-entry"]
    pub delete_entry: yesqlr::Query,
    #[name = "insert-relation"]
    pub insert_relation: yesqlr::Query,
    #[name = "update-relation"]
    pub update_relation: yesqlr::Query,
    #[name = "delete-relation"]
    pub delete_relation: yesqlr::Query,
    #[name = "reorder-relations"]
    pub reorder_relations: yesqlr::Query,
    #[name = "get-stats"]
    pub get_stats: yesqlr::Query,
    #[name = "get-pending-entries"]
    pub get_pending_entries: yesqlr::Query,
    #[name = "insert-submission-entry"]
    pub insert_submission_entry: yesqlr::Query,
    #[name = "insert-submission-relation"]
    pub insert_submission_relation: yesqlr::Query,
    #[name = "approve-submission"]
    pub approve_submission: yesqlr::Query,
    #[name = "approve-submission-relations"]
    pub approve_submission_relations: yesqlr::Query,
    #[name = "approve-submission-to-entries"]
    pub approve_submission_to_entries: yesqlr::Query,
    #[name = "reject-submission"]
    pub reject_submission: yesqlr::Query,
    #[name = "reject-submission-relations"]
    pub reject_submission_relations: yesqlr::Query,
    #[name = "reject-submission-to-entries"]
    pub reject_submission_to_entries: yesqlr::Query,
    #[name = "insert-comment"]
    pub insert_comment: yesqlr::Query,
    #[name = "get-comments"]
    pub get_comments: yesqlr::Query,
    #[name = "delete-comment"]
    pub delete_comment: yesqlr::Query,
    #[name = "delete-all-pending"]
    pub delete_all_pending: yesqlr::Query,
    #[name = "delete-all-pending-relations"]
    pub delete_all_pending_relations: yesqlr::Query,
    #[name = "delete-all-comments"]
    pub delete_all_comments: yesqlr::Query,
}

lazy_static! {
    pub static ref schema: Schema = {
        let result = yesqlr::parse(SQL_SCHEMA).expect("error parsing schema.sql");
        Schema::try_from(result).expect("error reading SQL schema")
    };
    pub static ref q: Queries = {
        let result = yesqlr::parse(SQL_QUERIES).expect("error parsing queries.sql");
        Queries::try_from(result).expect("error reading SQL queries")
    };
}
