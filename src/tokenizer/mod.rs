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
    #[error("tokenizer not found: {0}")]
    NotFound(String),
}

/// Simple whitespace tokenizer (built-in fallback).
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

pub type TokenizerMap = HashMap<String, Arc<dyn Tokenizer>>;

/// Load tokenizers from directory. Each .lua file becomes a tokenizer.
pub fn load_tokenizers(dir: &Path) -> Result<TokenizerMap, TokenizerError> {
    let mut tokenizers: TokenizerMap = HashMap::new();

    // Always include the simple tokenizer.
    tokenizers.insert("simple".to_string(), Arc::new(SimpleTokenizer));

    // Built-in default stemmers for all supported languages.
    let default_stemmers = [
        ("default.arabic", Algorithm::Arabic),
        ("default.danish", Algorithm::Danish),
        ("default.dutch", Algorithm::Dutch),
        ("default.english", Algorithm::English),
        ("default.finnish", Algorithm::Finnish),
        ("default.french", Algorithm::French),
        ("default.german", Algorithm::German),
        ("default.greek", Algorithm::Greek),
        ("default.hungarian", Algorithm::Hungarian),
        ("default.italian", Algorithm::Italian),
        ("default.norwegian", Algorithm::Norwegian),
        ("default.portuguese", Algorithm::Portuguese),
        ("default.romanian", Algorithm::Romanian),
        ("default.russian", Algorithm::Russian),
        ("default.spanish", Algorithm::Spanish),
        ("default.swedish", Algorithm::Swedish),
        ("default.tamil", Algorithm::Tamil),
        ("default.turkish", Algorithm::Turkish),
    ];

    for (name, algorithm) in default_stemmers {
        tokenizers.insert(name.to_string(), Arc::new(DefaultTokenizer::new(algorithm)));
    }

    if !dir.exists() {
        return Ok(tokenizers);
    }

    for entry in std::fs::read_dir(dir)? {
        let entry = entry?;
        let path = entry.path();

        if path.extension().map(|e| e == "lua").unwrap_or(false) {
            let name = path
                .file_stem()
                .and_then(|s| s.to_str())
                .unwrap_or("unknown")
                .to_string();

            let tk = LuaTokenizer::from_file(&path)?;
            tokenizers.insert(name, Arc::new(tk));
        }
    }

    Ok(tokenizers)
}
