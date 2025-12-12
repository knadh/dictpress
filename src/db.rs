use std::{
    io::{BufRead, Write},
    path::PathBuf,
};

use sqlx::sqlite::{SqlitePool, SqlitePoolOptions};

use crate::models::schema;

/// Current schema version.
const CURRENT_VERSION: &str = env!("CARGO_PKG_VERSION");

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
    let db = init(db_path, 1, false).await?;

    // Exec pragma and schema.
    sqlx::query(&schema.pragma.query).execute(&db).await?;
    sqlx::query(&schema.schema.query).execute(&db).await?;

    // Record the migration version.
    record_migration_version(&db, CURRENT_VERSION).await?;

    log::info!("successfully installed schema");
    Ok(())
}

/// Check if the DB file exists and exit with error message if not.
pub fn exists(path: &PathBuf) {
    if !path.exists() {
        log::error!(
            "database '{}' not found. Run `install` to create a new one.",
            path.display()
        );
        std::process::exit(1);
    }
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
    let db = init(db_path, 1, false).await?;

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
pub async fn init(
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

/// Do a simple gt/lt semver comparison.
fn compare_semver(a: &str, b: &str) -> i32 {
    let parse = |s: &str| -> Vec<u32> {
        s.trim_start_matches('v')
            .split('.')
            .filter_map(|p| p.parse().ok())
            .collect()
    };

    let av = parse(a);
    let bv = parse(b);

    // -1 = a < b
    // 0 = a == b
    // 1 = a > b
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
