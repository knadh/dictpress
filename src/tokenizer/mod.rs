mod lua;

pub use lua::LuaTokenizer;

use rust_stemmers::{Algorithm, Stemmer};
use std::{collections::HashMap, path::Path, sync::Arc};

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

/// Load tokenizers from directory. Each .lua file becomes a tokenizer.
pub fn load_all(dir: &Path) -> Result<Tokenizers, TokenizerError> {
    let mut out: Tokenizers = HashMap::new();

    // Always include the simple tokenizer.
    out.insert("simple".to_string(), Arc::new(SimpleTokenizer));

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
        out.insert(name.to_string(), Arc::new(DefaultTokenizer::new(algorithm)));
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
        let filename = path
            .file_name()
            .and_then(|s| s.to_str())
            .unwrap_or("<invalid>");

        // Skip non-.lua files with logging.
        if path.extension().is_none_or(|e| e != "lua") {
            log::info!("skipping '{}'", filename);
            continue;
        }

        // Get tokenizer name - skip invalid filenames.
        let name = match path.file_stem().and_then(|s| s.to_str()) {
            Some(stem) if !stem.is_empty() => stem.to_string(),
            _ => {
                log::warn!("skipping invalid file '{}'", filename);
                continue;
            }
        };

        // Load Lua tokenizer.
        let tokenizer = match LuaTokenizer::from_file(&path) {
            Ok(t) => t,
            Err(e) => {
                log::error!("error reading '{}': {}", filename, e);
                continue;
            }
        };

        // Validate by calling tokenize() and to_query().
        if let Err(e) = tokenizer.tokenize("test", "test") {
            log::error!("error validating '{}': {}", filename, e);
            continue;
        }
        if let Err(e) = tokenizer.to_query("test", "test") {
            log::error!("error validating '{}': {}", filename, e);
            continue;
        }

        log::info!("loaded '{}'", filename);
        out.insert(name, Arc::new(tokenizer));
    }

    Ok(out)
}
