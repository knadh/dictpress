use std::{
    io::{BufRead, Write},
    path::{Path, PathBuf},
};

use sqlx::sqlite::{SqlitePool, SqlitePoolOptions};

use crate::models::{self, schema, Config, Dicts, Lang, LangMap};

const SAMPLE_CONFIG: &str = include_str!("../config.sample.toml");

/// Current schema version.
const CURRENT_VERSION: &str = env!("CARGO_PKG_VERSION");

/// Create a SQLite connection pool.
pub async fn init_db(
    db_path: &str,
    max_conns: u32,
    read_only: bool,
) -> Result<SqlitePool, sqlx::Error> {
    let mode = if read_only { "ro" } else { "rwc" };
    SqlitePoolOptions::new()
        .max_connections(max_conns)
        .connect(&format!("sqlite://{}?mode={}", db_path, mode))
        .await
}

/// Initialize logger.
pub fn init_logger() {
    env_logger::Builder::new()
        .filter_level(log::LevelFilter::Info)
        .parse_env("RUST_LOG")
        .format(|buf, record| {
            use std::io::Write;
            let level = if record.level() != log::Level::Info {
                format!("[{}] ", record.level())
            } else {
                String::new()
            };
            writeln!(
                buf,
                "{} {}:{} {}{}",
                chrono::Local::now().format("%Y-%m-%dT%H:%M:%S%.3f"),
                record.file().unwrap_or("unknown"),
                record.line().unwrap_or(0),
                level,
                record.args()
            )
        })
        .init();
}

/// Load and merge one or more config files.
pub fn init_config(paths: &[PathBuf]) -> models::Config {
    let mut config: Option<models::Config> = None;

    for path in paths {
        log::info!("loading config: {}", path.display());
        match read_config(path) {
            Ok(c) => {
                if let Some(ref mut existing) = config {
                    // Merge configs.
                    merge_config(existing, c);
                } else {
                    config = Some(c);
                }
            }
            Err(e) => {
                log::error!("error loading config {}: {}", path.display(), e);
                std::process::exit(1);
            }
        }
    }

    config.unwrap_or_else(|| {
        log::error!("no config files specified");
        std::process::exit(1);
    })
}

/// Load configuration from TOML file.
fn read_config(path: &Path) -> Result<Config, Box<dyn std::error::Error>> {
    let content = std::fs::read_to_string(path)?;
    let cfg: Config = toml::from_str(&content)?;
    Ok(cfg)
}

/// Initialize languages from config.
pub fn init_langs(config: &Config) -> LangMap {
    let mut langs = LangMap::new();

    for (id, cfg) in &config.lang {
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
            tokenizer_type: if cfg.tokenizer_type.is_empty() {
                "lua".to_string()
            } else {
                cfg.tokenizer_type.clone()
            },
        };

        log::info!("language: {} (tokenizer: {})", id, lang.tokenizer);
        langs.insert(id.clone(), lang);
    }

    if langs.is_empty() {
        log::warn!("no languages configured");
    }

    langs
}

/// Initialize dictionary pairs from config.
pub fn init_dicts(langs: &LangMap, config: &Config) -> Dicts {
    let mut dicts = Dicts::new();

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
        dicts.push((from, to));
    }

    if dicts.is_empty() {
        log::warn!("no dictionary pairs configured");
    }

    dicts
}

/// Initialize admin templates from embedded files.
pub fn init_admin_templates() -> Result<tera::Tera, Box<dyn std::error::Error>> {
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
pub fn init_site_templates(
    site_dir: &std::path::Path,
) -> Result<tera::Tera, Box<dyn std::error::Error>> {
    let glob = format!("{}/**/*.html", site_dir.display());
    let mut tera = tera::Tera::new(&glob)?;
    tera.autoescape_on(vec![".html"]);
    log::info!("loaded site templates from {}", site_dir.display());
    Ok(tera)
}

/// Load i18n strings from the site/JSON file.
pub fn load_i18n(
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

/// Generate sample config file.
pub fn generate_config(path: &Path) -> Result<(), Box<dyn std::error::Error>> {
    if path.exists() {
        return Err("config file already exists".into());
    }
    std::fs::write(path, SAMPLE_CONFIG)?;
    Ok(())
}

/// Merge the given src config into the dest config struct.
fn merge_config(dest: &mut Config, src: Config) {
    // Merge app config.
    if !src.app.address.is_empty() {
        dest.app.address = src.app.address;
    }
    if !src.app.admin_username.is_empty() {
        dest.app.admin_username = src.app.admin_username;
    }
    if !src.app.admin_password.is_empty() {
        dest.app.admin_password = src.app.admin_password;
    }
    if !src.app.root_url.is_empty() {
        dest.app.root_url = src.app.root_url;
    }
    if !src.app.dicts.is_empty() {
        dest.app.dicts = src.app.dicts;
    }
    if !src.app.tokenizers_dir.is_empty() {
        dest.app.tokenizers_dir = src.app.tokenizers_dir;
    }
    dest.app.enable_submissions = src.app.enable_submissions;

    // Merge DB config.
    if !src.db.path.is_empty() {
        dest.db.path = src.db.path;
    }
    if src.db.max_conns > 0 {
        dest.db.max_conns = src.db.max_conns;
    }

    // Merge languages.
    for (id, lang) in src.lang {
        dest.lang.insert(id, lang);
    }
}

/// Install database schema.
pub async fn install_schema(db_path: &str, prompt: bool) -> Result<(), Box<dyn std::error::Error>> {
    if prompt {
        println!("\n** Initialize new database at '{}'? **\n", db_path);
        print!("continue (y/n)?  ");
        std::io::stdout().flush()?;

        let mut input = String::new();
        std::io::stdin().lock().read_line(&mut input)?;
        if input.trim().to_lowercase() != "y" {
            println!("install cancelled");
            return Ok(());
        }
    }

    // Create new database.
    let db = init_db(db_path, 1, false).await?;

    // Exec pragma and schema.
    sqlx::query(&schema.pragma.query).execute(&db).await?;
    sqlx::query(&schema.schema.query).execute(&db).await?;

    // Record the migration version.
    record_migration_version(&db, CURRENT_VERSION).await?;

    log::info!("successfully installed schema");
    Ok(())
}

/// Record migration version in the settings table.
async fn record_migration_version(
    db: &sqlx::SqlitePool,
    version: &str,
) -> Result<(), Box<dyn std::error::Error>> {
    let migrations_json = format!(r#"["{}"]"#, version);
    sqlx::query(
        r#"INSERT INTO settings (key, value) VALUES ('migrations', ?)
           ON CONFLICT(key) DO UPDATE SET value = json_insert(settings.value, '$[#]', ?)"#,
    )
    .bind(&migrations_json)
    .bind(version)
    .execute(db)
    .await?;
    Ok(())
}

/// Get last migration version from the database.
async fn get_last_migration_version(
    db: &sqlx::SqlitePool,
) -> Result<String, Box<dyn std::error::Error>> {
    let result: Option<(String,)> =
        sqlx::query_as("SELECT JSON_EXTRACT(value, '$[#-1]') FROM settings WHERE key='migrations'")
            .fetch_optional(db)
            .await?;

    match result {
        Some((ver,)) => Ok(ver),
        None => Ok("v0.0.0".to_string()),
    }
}

/// Check if there are pending database upgrades.
pub async fn check_upgrade(db: &SqlitePool) -> Result<(), Box<dyn std::error::Error>> {
    let last_ver = get_last_migration_version(db).await?;

    // Compare versions.
    if version_compare(&last_ver, CURRENT_VERSION) < 0 {
        return Err(format!(
            "database version ({}) is older than binary ({}). Backup the database and run 'upgrade'",
            last_ver, CURRENT_VERSION
        )
        .into());
    }

    Ok(())
}

/// Upgrade database schema.
pub async fn upgrade_schema(db_path: &str, prompt: bool) -> Result<(), Box<dyn std::error::Error>> {
    if prompt {
        println!("** IMPORTANT: Take a backup of the database before upgrading.");
        print!("continue (y/n)?  ");
        std::io::stdout().flush()?;

        let mut input = String::new();
        std::io::stdin().lock().read_line(&mut input)?;
        if input.trim().to_lowercase() != "y" {
            println!("upgrade cancelled.");
            return Ok(());
        }
    }

    // Connect to database.
    let db = init_db(db_path, 1, false).await?;

    let last_ver = get_last_migration_version(&db).await?;

    if version_compare(&last_ver, CURRENT_VERSION) >= 0 {
        log::info!("no upgrades to run. Database is up to date.");
        return Ok(());
    }

    // Record new version.
    record_migration_version(&db, CURRENT_VERSION).await?;

    log::info!("upgrade complete");
    Ok(())
}

/// Simple semver comparison (-1 = a < b, 0 = a == b, 1 = a > b).
fn version_compare(a: &str, b: &str) -> i32 {
    let parse = |s: &str| -> Vec<u32> {
        s.trim_start_matches('v')
            .split('.')
            .filter_map(|p| p.parse().ok())
            .collect()
    };

    let av = parse(a);
    let bv = parse(b);

    for i in 0..av.len().max(bv.len()) {
        let ai = av.get(i).copied().unwrap_or(0);
        let bi = bv.get(i).copied().unwrap_or(0);
        if ai < bi {
            return -1;
        }
        if ai > bi {
            return 1;
        }
    }
    0
}

/// Generate sitemap files.
pub async fn generate_sitemaps(
    db_path: &str,
    from_lang: &str,
    to_lang: &str,
    root_url: &str,
    max_rows: usize,
    output_prefix: &str,
    output_dir: &Path,
    generate_robots: bool,
    sitemap_url: Option<&str>,
) -> Result<(), Box<dyn std::error::Error>> {
    use regex::Regex;
    use std::fs;

    // Create output directory.
    fs::create_dir_all(output_dir)?;

    // Connect to database (read-only).
    let db = init_db(db_path, 1, true).await?;

    // Get all entries for the from_lang.
    let rows: Vec<(String,)> = sqlx::query_as(
        "SELECT json_extract(content, '$[0]') FROM entries WHERE lang = ? AND status = 'enabled' ORDER BY weight"
    )
    .bind(from_lang)
    .fetch_all(&db)
    .await?;

    let re_spaces = Regex::new(r"\s+")?;
    let mut urls: Vec<String> = Vec::new();
    let mut n = 0;
    let mut file_index = 1;

    log::info!("generating sitemaps for {} -> {}", from_lang, to_lang);

    for (word,) in rows {
        let word = word.to_lowercase().trim().to_string();
        let word = re_spaces.replace_all(&word, "+").to_string();

        let url = format!("{}/dictionary/{}/{}/{}", root_url, from_lang, to_lang, word);
        urls.push(url);

        if urls.len() >= max_rows {
            write_sitemap(&urls, file_index, output_prefix, output_dir)?;
            urls.clear();
            file_index += 1;
        }
        n += 1;
    }

    // Write remaining URLs.
    if !urls.is_empty() {
        write_sitemap(&urls, file_index, output_prefix, output_dir)?;
    }

    log::info!("generated {} URLs in {} sitemap files", n, file_index);

    // Generate robots.txt.
    if generate_robots {
        if let Some(url) = sitemap_url {
            generate_robots_txt(url, output_dir)?;
        }
    }

    Ok(())
}

fn write_sitemap(
    urls: &[String],
    index: usize,
    output_prefix: &str,
    output_dir: &Path,
) -> Result<(), Box<dyn std::error::Error>> {
    use std::fs::File;

    let filepath = output_dir.join(format!("{}{}.txt", output_prefix, index));
    log::info!("writing to {}", filepath.display());

    let mut file = File::create(&filepath)?;
    for url in urls {
        writeln!(file, "{}", url)?;
    }
    Ok(())
}

fn generate_robots_txt(
    sitemap_url: &str,
    output_dir: &Path,
) -> Result<(), Box<dyn std::error::Error>> {
    use std::fs::{self, File};

    let robots_path = output_dir.join("robots.txt");
    log::info!("writing to {}", robots_path.display());

    let mut file = File::create(&robots_path)?;
    writeln!(file, "User-agent: *")?;
    writeln!(file, "Disallow:")?;
    writeln!(file, "Allow: /")?;
    writeln!(file)?;

    // Add sitemap references.
    for entry in fs::read_dir(output_dir)? {
        let entry = entry?;
        let name = entry.file_name().to_string_lossy().to_string();
        if name != "robots.txt" && name.ends_with(".txt") {
            writeln!(file, "Sitemap: {}/{}", sitemap_url, name)?;
        }
    }

    Ok(())
}
