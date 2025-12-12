use std::io::{BufRead, Write};

use sqlx::sqlite::{SqlitePool, SqlitePoolOptions};

use crate::models::{schema, Config, Dicts, Lang, LangMap};

/// Current schema version.
const CURRENT_VERSION: &str = env!("CARGO_PKG_VERSION");

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
    if compare_semver(&last_ver, CURRENT_VERSION) < 0 {
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

    if compare_semver(&last_ver, CURRENT_VERSION) >= 0 {
        log::info!("no upgrades to run. Database is up to date.");
        return Ok(());
    }

    // Record new version.
    record_migration_version(&db, CURRENT_VERSION).await?;

    log::info!("upgrade complete");
    Ok(())
}

/// Create a SQLite connection pool.
pub async fn init_db(
    db_path: &str,
    max_conns: u32,
    read_only: bool,
) -> Result<SqlitePool, sqlx::Error> {
    let mode = if read_only { "ro" } else { "rwc" };
    let db = SqlitePoolOptions::new()
        .max_connections(max_conns)
        .connect(&format!("sqlite://{}?mode={}", db_path, mode))
        .await?;

    // Apply SQLite DB pragmas.
    if let Err(e) = sqlx::query(&schema.pragma.query).execute(&db).await {
        log::error!("error applying pragmas: {}", e);
        std::process::exit(1);
    }

    Ok(db)
}

/// Simple semver comparison (-1 = a < b, 0 = a == b, 1 = a > b).
fn compare_semver(a: &str, b: &str) -> i32 {
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
