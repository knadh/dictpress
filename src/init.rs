use std::path::Path;

use crate::cache::{Cache, CacheConfig, CacheError};
use crate::models::{Config, Dicts, Lang, LangMap};
use crate::tokenizer::{TokenizerError, Tokenizers};

/// Initialize logger.
pub fn logger() {
    env_logger::Builder::new()
        .filter_level(log::LevelFilter::Info)
        .parse_env("RUST_LOG")
        .format(|buf, rec| {
            use std::io::Write;
            let level = if rec.level() != log::Level::Info {
                format!("[{}] ", rec.level())
            } else {
                String::new()
            };
            writeln!(
                buf,
                "{} {}:{} {}{}",
                chrono::Local::now().format("%Y-%m-%dT%H:%M:%S%.3f"),
                rec.file().unwrap_or("unknown"),
                rec.line().unwrap_or(0),
                level,
                rec.args()
            )
        })
        .init();
}

/// Initialize languages from config.
pub fn langs(config: &Config) -> LangMap {
    let mut langs = LangMap::new();

    for (id, cfg) in &config.lang {
        // Validate tokenizer_type.
        let typ = if cfg.tokenizer_type.is_empty() {
            "default".to_string()
        } else {
            cfg.tokenizer_type.clone()
        };

        if typ != "default" && typ != "lua" {
            log::error!(
                "unknown tokenizer_type '{}' for language '{}'. Must be 'default' or 'lua'.",
                typ,
                id
            );
            std::process::exit(1);
        }

        let lang = Lang {
            id: id.clone(),
            name: if cfg.name.is_empty() {
                id.clone()
            } else {
                cfg.name.clone()
            },
            types: cfg.types.clone(),
            tokenizer: if cfg.tokenizer.is_empty() {
                "simple".to_string()
            } else {
                cfg.tokenizer.clone()
            },
            tokenizer_type: typ,
        };

        log::info!(
            "language: {} (tokenizer: {}, type: {})",
            id,
            lang.tokenizer,
            lang.tokenizer_type
        );

        langs.insert(id.clone(), lang);
    }

    if langs.is_empty() {
        log::warn!("no languages configured");
    }

    langs
}

/// Initialize dictionary pairs from config.
pub fn dicts(langs: &LangMap, config: &Config) -> Dicts {
    let mut out = Dicts::new();

    for d in &config.app.dicts {
        if d.len() != 2 {
            log::warn!("invalid dict pair: {:?}", d);
            continue;
        }

        let from_id = &d[0];
        let to_id = &d[1];

        let from = match langs.get(from_id) {
            Some(l) => l.clone(),
            None => {
                log::warn!("unknown language in dict pair: {}", from_id);
                continue;
            }
        };

        let to = match langs.get(to_id) {
            Some(l) => l.clone(),
            None => {
                log::warn!("unknown language in dict pair: {}", to_id);
                continue;
            }
        };

        log::info!("dict: {} -> {}", from_id, to_id);
        out.push((from, to));
    }

    if out.is_empty() {
        log::warn!("no dictionary pairs configured");
    }

    out
}

/// Initialize admin templates from embedded files.
pub fn admin_tpls() -> Result<tera::Tera, Box<dyn std::error::Error>> {
    use crate::http::AdminTemplates;

    let mut tera = tera::Tera::default();
    tera.autoescape_on(vec![".html"]);

    // Load all embedded admin templates.
    for file in AdminTemplates::iter() {
        let path = file.as_ref();
        if path.ends_with(".html") {
            if let Some(content) = AdminTemplates::get(path) {
                let template_name = format!("admin/{}", path);
                if let Ok(s) = std::str::from_utf8(&content.data) {
                    tera.add_raw_template(&template_name, s)?;
                }
            }
        }
    }

    Ok(tera)
}

/// Initialize site templates from disk.
pub fn site_tpls(site_dir: &std::path::Path) -> Result<tera::Tera, Box<dyn std::error::Error>> {
    let glob = format!("{}/**/*.html", site_dir.display());
    let mut tera = tera::Tera::new(&glob)?;
    tera.autoescape_on(vec![".html"]);
    log::info!("loaded site templates from {}", site_dir.display());

    Ok(tera)
}

/// Load i18n strings from the site/JSON file.
pub fn i18n(
    path: &std::path::Path,
) -> Result<std::collections::HashMap<String, String>, Box<dyn std::error::Error>> {
    let content = std::fs::read_to_string(path)?;
    let raw: std::collections::HashMap<String, String> = serde_json::from_str(&content)?;
    // Convert keys: "public.noResults" -> "public_noResults" for Tera compatibility.
    let i18n = raw
        .into_iter()
        .map(|(k, v)| (k.replace('.', "_"), v))
        .collect();

    Ok(i18n)
}

/// Initialize lua tokenizers from a given directory.
pub fn tokenizers(dir: &str) -> Result<Tokenizers, TokenizerError> {
    crate::tokenizer::load_all(Path::new(dir))
}

/// Initialize cache from configuration.
pub async fn cache(cfg: &CacheConfig) -> Result<Cache, CacheError> {
    log::info!(
        "cache: mode={}, memory={}MB, disk={}MB, ttl={}, dir={}",
        cfg.mode,
        cfg.max_memory_mb,
        cfg.max_disk_mb,
        cfg.ttl,
        cfg.dir
    );
    Cache::new(cfg).await
}
