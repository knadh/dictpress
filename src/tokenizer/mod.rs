mod lua;

pub use lua::LuaTokenizer;

use rust_stemmers::{Algorithm, Stemmer};
use std::{collections::HashMap, path::Path, sync::Arc};

use crate::models::DEFAULT_TOKENIZER;

/// Tokenizer trait for converting text to searchable tokens.
pub trait Tokenizer: Send + Sync {
    /// Convert text to tokens for indexing.
    fn tokenize(&self, text: &str, lang: &str) -> Result<Vec<String>, TokenizerError>;

    /// Convert search query to FTS5 query string.
    fn to_query(&self, text: &str, lang: &str) -> Result<String, TokenizerError>;
}

#[derive(Debug, thiserror::Error)]
pub enum TokenizerError {
    #[error("lua error: {0}")]
    Lua(#[from] mlua::Error),
    #[error("io error: {0}")]
    Io(#[from] std::io::Error),
}

/// Simple whitespace tokenizer (default fallback).
pub struct SimpleTokenizer;

impl Tokenizer for SimpleTokenizer {
    fn tokenize(&self, text: &str, _lang: &str) -> Result<Vec<String>, TokenizerError> {
        Ok(text.split_whitespace().map(|s| s.to_lowercase()).collect())
    }

    fn to_query(&self, text: &str, _lang: &str) -> Result<String, TokenizerError> {
        let terms: Vec<String> = text.split_whitespace().map(|s| s.to_lowercase()).collect();
        Ok(terms.join(" "))
    }
}

/// Default tokenizer using Snowball stemmer for a specific language.
pub struct DefaultTokenizer {
    stemmer: Stemmer,
}

impl DefaultTokenizer {
    pub fn new(algorithm: Algorithm) -> Self {
        Self {
            stemmer: Stemmer::create(algorithm),
        }
    }
}

impl Tokenizer for DefaultTokenizer {
    fn tokenize(&self, text: &str, _lang: &str) -> Result<Vec<String>, TokenizerError> {
        Ok(text
            .split_whitespace()
            .map(|word| self.stemmer.stem(&word.to_lowercase()).to_string())
            .collect())
    }

    fn to_query(&self, text: &str, _lang: &str) -> Result<String, TokenizerError> {
        let terms: Vec<String> = text
            .split_whitespace()
            .map(|word| self.stemmer.stem(&word.to_lowercase()).to_string())
            .collect();
        Ok(terms.join(" "))
    }
}

pub type Tokenizers = HashMap<String, Arc<dyn Tokenizer>>;

/// Load all tokenizers into a map, the default bundled ones and the Lua
/// ones from the given directory. Each .lua file becomes a tokenizer.
pub fn load_all(dir: &Path) -> Result<Tokenizers, TokenizerError> {
    let mut out: Tokenizers = HashMap::new();

    // Always include the simple tokenizer.
    out.insert(DEFAULT_TOKENIZER.to_string(), Arc::new(SimpleTokenizer));

    // Add built-in default stemmers.
    let default_stemmers = [
        ("arabic", Algorithm::Arabic),
        ("danish", Algorithm::Danish),
        ("dutch", Algorithm::Dutch),
        ("english", Algorithm::English),
        ("finnish", Algorithm::Finnish),
        ("french", Algorithm::French),
        ("german", Algorithm::German),
        ("greek", Algorithm::Greek),
        ("hungarian", Algorithm::Hungarian),
        ("italian", Algorithm::Italian),
        ("norwegian", Algorithm::Norwegian),
        ("portuguese", Algorithm::Portuguese),
        ("romanian", Algorithm::Romanian),
        ("russian", Algorithm::Russian),
        ("spanish", Algorithm::Spanish),
        ("swedish", Algorithm::Swedish),
        ("tamil", Algorithm::Tamil),
        ("turkish", Algorithm::Turkish),
    ];
    for (name, algorithm) in default_stemmers {
        out.insert(
            format!("default:{}", name),
            Arc::new(DefaultTokenizer::new(algorithm)),
        );
    }

    // If no dir has been specified, skip loading from disk.
    if dir == Path::new("") {
        log::info!("no tokenizers directory to load");
        return Ok(out);
    }

    if !dir.exists() {
        log::info!("'{}' tokenizers directory does not exist", dir.display());
        return Ok(out);
    }

    log::info!("loading tokenizers from '{}'", dir.display());

    // Add .lua tokenizers from the directory.
    for entry in std::fs::read_dir(dir)?.flatten() {
        let path = entry.path();
        let fname = path
            .file_name()
            .and_then(|s| s.to_str())
            .unwrap_or("<invalid>");

        // Skip non-.lua files with logging.
        if path.extension().is_none_or(|e| e != "lua") {
            log::info!("skipping '{}'", fname);
            continue;
        }

        // Get tokenizer name (full filename including .lua extension).
        let name = match path.file_name().and_then(|s| s.to_str()) {
            Some(n) if !n.is_empty() => n.to_string(),
            _ => {
                log::warn!("skipping invalid file '{}'", fname);
                continue;
            }
        };

        // Load Lua tokenizer.
        let tk = match LuaTokenizer::from_file(&path) {
            Ok(t) => t,
            Err(e) => {
                log::error!("error reading '{}': {}", fname, e);
                continue;
            }
        };

        // Validate by calling tokenize() and to_query().
        if let Err(e) = tk.tokenize("test", "test") {
            log::error!("error validating '{}': {}", fname, e);
            continue;
        }
        if let Err(e) = tk.to_query("test", "test") {
            log::error!("error validating '{}': {}", fname, e);
            continue;
        }

        log::info!("loaded '{}'", fname);
        out.insert(format!("lua:{}", name), Arc::new(tk));
    }

    Ok(out)
}

/// Parse and validate tokenizer field in format "default:name" or "lua:filename.lua".
/// Returns the validated tokenizer string for lookup in the tokenizers map.
pub fn parse_tokenizer_field(tokenizer: &str) -> Option<String> {
    if tokenizer.is_empty() {
        return None;
    }

    if tokenizer.starts_with("default:") && tokenizer.len() > 8 {
        Some(tokenizer.to_string())
    } else if tokenizer.starts_with("lua:") && tokenizer.len() > 4 {
        Some(tokenizer.to_string())
    } else {
        log::warn!(
            "unknown tokenizer format '{}'. expected 'default:name' or 'lua:filename.lua'",
            tokenizer
        );
        None
    }
}
