use crate::autocomplete::Autocomplete;
use crate::cache::{Cache, CacheConfig, CacheError};
use crate::manager::Manager;
use crate::models::{Config, Dicts, Lang, LangMap, DEFAULT_TOKENIZER};
use crate::tokenizer::Tokenizers;

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

/// Initialize languages from config, validating tokenizers against loaded tokenizers.
pub fn langs(config: &Config, tokenizers: &Tokenizers) -> LangMap {
    let mut langs = LangMap::new();

    for (id, cfg) in &config.lang {
        let tokenizer = if cfg.tokenizer.is_empty() {
            // If the tokenizer is not specified, use default.
            DEFAULT_TOKENIZER.to_string()
        } else if (!cfg.tokenizer.starts_with("default:")) && (!cfg.tokenizer.starts_with("lua:")) {
            // Tokenizer name must start with "default:" or "lua:".
            log::error!(
                "invalid tokenizer format '{}' for language '{}'. defaulting to '{}'",
                cfg.tokenizer,
                id,
                DEFAULT_TOKENIZER
            );
            DEFAULT_TOKENIZER.to_string()
        } else if tokenizers.contains_key(&cfg.tokenizer) {
            // Yep, it's valid.
            cfg.tokenizer.clone()
        } else {
            // Unknown tokenizer.
            log::error!(
                "tokenizer '{}' not found for language '{}'. defaulting to '{}'",
                cfg.tokenizer,
                id,
                DEFAULT_TOKENIZER
            );
            DEFAULT_TOKENIZER.to_string()
        };

        // Create the language instance.
        let lang = Lang {
            id: id.clone(),
            name: if cfg.name.is_empty() {
                id.clone()
            } else {
                cfg.name.clone()
            },
            types: cfg.types.clone(),
            tokenizer: tokenizer.clone(),
        };

        log::info!("language: {} (tokenizer: {})", id, tokenizer);

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

/// Load i18n from the site/JSON file.
pub fn i18n(path: &std::path::Path) -> Result<tinyi18n_rs::I18n, Box<dyn std::error::Error>> {
    Ok(tinyi18n_rs::I18n::from_file(path, None, None)?)
}

/// Register i18n functions (t, ts, tc) with Tera.
///
/// Usage in templates:
/// - `{{ t(key="public.hello") }}` - simple translation
/// - `{{ ts(key="public.greeting", name="World", count="5") }}` - with substitution
/// - `{{ tc(key="public.items", count=5) }}` - singular/plural based on count
pub fn register_i18n_functions(tera: &mut tera::Tera, i18n: std::sync::Arc<tinyi18n_rs::I18n>) {
    // t(key) - simple translation
    let i18n_t = i18n.clone();
    tera.register_function(
        "t",
        move |args: &std::collections::HashMap<String, tera::Value>| {
            let key = args
                .get("key")
                .and_then(|v| v.as_str())
                .ok_or_else(|| tera::Error::msg("t() requires 'key' argument"))?;
            Ok(tera::Value::String(i18n_t.t(key)))
        },
    );

    // ts(key, ...params) - translation with parameter substitution
    // All arguments except "key" are treated as substitution parameters.
    // Example: ts(key="greeting", name="World", count="5")
    let i18n_ts = i18n.clone();
    tera.register_function(
        "ts",
        move |args: &std::collections::HashMap<String, tera::Value>| {
            let key = args
                .get("key")
                .and_then(|v| v.as_str())
                .ok_or_else(|| tera::Error::msg("ts() requires 'key' argument"))?;

            // Collect all arguments except "key" as substitution parameters.
            let mut params: Vec<(String, String)> = Vec::new();
            for (k, v) in args {
                if k == "key" {
                    continue;
                }
                let val = match v {
                    tera::Value::String(s) => s.clone(),
                    tera::Value::Number(n) => n.to_string(),
                    tera::Value::Bool(b) => b.to_string(),
                    tera::Value::Null => String::new(),
                    _ => v.to_string().trim_matches('"').to_string(),
                };
                params.push((k.clone(), val));
            }

            // Convert to the format expected by I18n::ts
            let params_ref: Vec<(&str, String)> =
                params.iter().map(|(k, v)| (k.as_str(), v.clone())).collect();
            Ok(tera::Value::String(i18n_ts.ts(key, &params_ref)))
        },
    );

    // tc(key, count) - count-based singular/plural
    let i18n_tc = i18n.clone();
    tera.register_function(
        "tc",
        move |args: &std::collections::HashMap<String, tera::Value>| {
            let key = args
                .get("key")
                .and_then(|v| v.as_str())
                .ok_or_else(|| tera::Error::msg("tc() requires 'key' argument"))?;
            let count = args.get("count").and_then(|v| v.as_i64()).unwrap_or(1);
            Ok(tera::Value::String(i18n_tc.tc(key, count)))
        },
    );
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

/// Initialize autocomplete trie from database.
pub async fn autocomplete(
    mgr: &Manager,
    langs: &LangMap,
) -> Result<Autocomplete, Box<dyn std::error::Error>> {
    let mut ac = Autocomplete::new();

    for lang_id in langs.keys() {
        let words = mgr.get_all_words(lang_id).await?;
        let num = words.len();
        ac.build(lang_id, words);
        log::info!("autocomplete: loaded {} words for {}", num, lang_id);
    }

    Ok(ac)
}
