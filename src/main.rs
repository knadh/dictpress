mod cli;
mod handlers;
mod http;
mod importer;
mod init;
mod manager;
mod models;
mod sitemaps;
mod tokenizer;

use std::{path::PathBuf, sync::Arc};

use clap::Parser;

use cli::Commands;
use handlers::{Consts, Ctx};
use manager::{Manager, ManagerConfig};

#[tokio::main]
async fn main() {
    init::init_logger();

    let cli = cli::Cli::parse();

    // DB path from --db flag.
    let db_path = cli.db.to_string_lossy().to_string();

    // Handle CLI flags.
    if let Some(cmd) = cli.command {
        match cmd {
            // Generate a new config file.
            Commands::NewConfig { path } => {
                match init::generate_config(&path) {
                    Ok(_) => {
                        log::info!("config file generated: {}", path.display());
                    }
                    Err(e) => {
                        log::error!("error generating config: {}", e);
                        std::process::exit(1);
                    }
                }
                return;
            }

            // Create a new SQLite database with schema.
            Commands::Install { yes } => {
                if cli.db.exists() {
                    log::error!("database '{}' already exists", cli.db.display());
                    std::process::exit(1);
                }
                if let Err(e) = init::install_schema(&db_path, !yes).await {
                    log::error!("error installing schema: {}", e);
                    std::process::exit(1);
                }
                return;
            }

            // Upgrade existing database schema.
            Commands::Upgrade { yes } => {
                check_db(&cli.db);

                if let Err(e) = init::upgrade_schema(&db_path, !yes).await {
                    log::error!("error upgrading schema: {}", e);
                    std::process::exit(1);
                }
                return;
            }

            // Import entries from a CSV file.
            Commands::Import { file } => {
                check_db(&cli.db);

                let config = init::init_config(&cli.config);
                let langs = init::init_langs(&config);

                let tokenizers_dir = if config.app.tokenizers_dir.is_empty() {
                    "tokenizers".to_string()
                } else {
                    config.app.tokenizers_dir.clone()
                };

                if let Err(e) = importer::import_csv(&file, &db_path, &tokenizers_dir, langs).await
                {
                    log::error!("error importing: {}", e);
                    std::process::exit(1);
                }
                return;
            }

            // Generate sitemaps for entries in the database.
            Commands::Sitemap {
                from_lang,
                to_lang,
                url,
                max_rows,
                output_prefix,
                output_dir,
                robots,
            } => {
                check_db(&cli.db);

                let config = init::init_config(&cli.config);
                if let Err(e) = sitemaps::generate_sitemaps(
                    &db_path,
                    &from_lang,
                    &to_lang,
                    &config.app.root_url,
                    max_rows,
                    &output_prefix,
                    &output_dir,
                    robots,
                    url.as_deref(),
                )
                .await
                {
                    log::error!("error generating sitemaps: {}", e);
                    std::process::exit(1);
                }
                return;
            }
        }
    }

    // For server mode, DB must exist.
    check_db(&cli.db);

    // Load config.
    let config = init::init_config(&cli.config);

    // Initialize languages and dicts config.
    let langs = init::init_langs(&config);
    let dicts = init::init_dicts(&langs, &config);

    // Create database pool.
    let db = match init::init_db(&db_path, config.db.max_conns, false).await {
        Ok(pool) => pool,
        Err(e) => {
            log::error!("error connecting to database: {}", e);
            std::process::exit(1);
        }
    };

    // Check for pending semver DB upgrades.
    if let Err(e) = init::check_upgrade(&db).await {
        log::error!("{}", e);
        std::process::exit(1);
    }

    // Initialize admin templates (embedded).
    let admin_tpl = match init::init_admin_templates() {
        Ok(t) => Arc::new(t),
        Err(e) => {
            log::error!("error loading admin templates: {}", e);
            std::process::exit(1);
        }
    };

    // Initialize site templates (optional, from --site flag).
    let site_tpl = if let Some(site_path) = &cli.site {
        log::info!("loading site theme: {}", site_path.display());

        let templates = init::init_site_templates(site_path).unwrap_or_else(|e| {
            log::error!(
                "error loading site templates from {}: {}",
                site_path.display(),
                e
            );
            std::process::exit(1);
        });

        Some(Arc::new(templates))
    } else {
        None
    };

    // Load i18n strings (only if site is enabled).
    let i18n = if let Some(site_path) = &cli.site {
        init::load_i18n(&site_path.join("lang.json")).unwrap_or_else(|e| {
            log::warn!("failed to load i18n: {}, using empty", e);
            std::collections::HashMap::new()
        })
    } else {
        std::collections::HashMap::new()
    };

    // Initialize manager.
    let tokenizers_dir = if config.app.tokenizers_dir.is_empty() {
        "tokenizers".to_string()
    } else {
        config.app.tokenizers_dir.clone()
    };

    let cfg = ManagerConfig { tokenizers_dir };
    let mgr = match Manager::new(db, cfg, langs.clone(), dicts.clone()).await {
        Ok(m) => Arc::new(m),
        Err(e) => {
            log::error!("error initializing manager: {}", e);
            std::process::exit(1);
        }
    };

    // Preload static files (JS & CSS) for bundling.
    let static_files = http::preload_static_files(&cli.site);

    // Setup the global app context used in HTTP handlers.
    let ctx = Arc::new(Ctx {
        mgr,
        langs,
        dicts,
        admin_tpl,
        site_tpl,
        site_path: cli.site.clone(),
        i18n,
        static_files,

        // Global constants.
        consts: Consts {
            root_url: config.app.root_url,
            enable_submissions: config.app.enable_submissions,
            enable_glossary: true,
            admin_username: config.app.admin_username,
            admin_password: config.app.admin_password,
            default_per_page: 20,
            max_per_page: 100,
            site_max_content_items: 5,
            admin_assets: Vec::new(),
        },

        // Generate a random string for asset version cache busting.
        asset_ver: format!(
            "{:08}",
            chrono::Local::now().timestamp_nanos_opt().unwrap_or(0) % 100_000_000
        ),
    });

    // Start the HTTP server.
    let routes = http::init_handlers(ctx);
    let addr = config.app.address;

    log::info!("starting server on {}", addr);

    let listener = match tokio::net::TcpListener::bind(&addr).await {
        Ok(l) => l,
        Err(e) => {
            log::error!("error listening on {}: {}", addr, e);
            std::process::exit(1);
        }
    };

    if let Err(e) = axum::serve(listener, routes).await {
        log::error!("server error: {}", e);
        std::process::exit(1);
    }
}

/// Check if the DB file exists and exit with error message if not.
fn check_db(path: &PathBuf) {
    if !path.exists() {
        log::error!(
            "database '{}' not found. Run `install` to create a new one.",
            path.display()
        );
        std::process::exit(1);
    }
}
