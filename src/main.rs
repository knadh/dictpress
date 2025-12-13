mod cli;
mod config;
mod db;
mod handlers;
mod http;
mod importer;
mod init;
mod manager;
mod models;
mod sitemaps;
mod tokenizer;

use std::sync::Arc;

use clap::Parser;

use cli::Commands;
use handlers::{Consts, Ctx};
use manager::Manager;

#[tokio::main]
async fn main() {
    init::logger();

    let cli = cli::Cli::parse();

    // DB path from --db flag.
    let db_path = cli.db_path.to_string_lossy().to_string();

    // Handle CLI flags.
    if let Some(cmd) = cli.command {
        match cmd {
            // Generate a new config file.
            Commands::NewConfig { path } => {
                match config::generate_sample(&path) {
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
                if cli.db_path.exists() {
                    log::error!("database '{}' already exists", cli.db_path.display());
                    std::process::exit(1);
                }
                if let Err(e) = db::install_schema(&db_path, !yes).await {
                    log::error!("error installing schema: {}", e);
                    std::process::exit(1);
                }
                return;
            }

            // Upgrade existing database schema.
            Commands::Upgrade { yes } => {
                db::exists(&cli.db_path);

                if let Err(e) = db::upgrade_schema(&db_path, !yes).await {
                    log::error!("error upgrading schema: {}", e);
                    std::process::exit(1);
                }
                return;
            }

            // Import entries from a CSV file.
            Commands::Import { file } => {
                db::exists(&cli.db_path);

                let config = config::load_all(&cli.config);
                let langs = init::langs(&config);

                let tokenizers = match init::tokenizers(&config.app.tokenizers_dir) {
                    Ok(t) => t,
                    Err(e) => {
                        log::error!("error loading tokenizers: {}", e);
                        std::process::exit(1);
                    }
                };

                if let Err(e) = importer::import_csv(&file, &db_path, &tokenizers, langs).await {
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
                db::exists(&cli.db_path);

                let config = config::load_all(&cli.config);
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
    db::exists(&cli.db_path);

    // Load config.
    let config = config::load_all(&cli.config);

    // Initialize languages and dicts config.
    let langs = init::langs(&config);
    let dicts = init::dicts(&langs, &config);

    // Create database pool.
    let db = match db::init(&db_path, config.db.max_conns, false).await {
        Ok(pool) => pool,
        Err(e) => {
            log::error!("error connecting to database: {}", e);
            std::process::exit(1);
        }
    };

    // Check for pending semver DB upgrades.
    if let Err(e) = db::check_upgrade(&db).await {
        log::error!("{}", e);
        std::process::exit(1);
    }

    // Initialize admin templates (embedded).
    let admin_tpl = match init::admin_tpls() {
        Ok(t) => Arc::new(t),
        Err(e) => {
            log::error!("error loading admin templates: {}", e);
            std::process::exit(1);
        }
    };

    // Initialize site templates (optional, from --site flag).
    let site_tpl = if let Some(site_path) = &cli.site {
        log::info!("loading site theme: {}", site_path.display());

        let templates = init::site_tpls(site_path).unwrap_or_else(|e| {
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
        init::i18n(&site_path.join("lang.json")).unwrap_or_else(|e| {
            log::warn!("failed to load i18n: {}, using empty", e);
            std::collections::HashMap::new()
        })
    } else {
        std::collections::HashMap::new()
    };

    // Initialize tokenizers.
    let tokenizers = match init::tokenizers(&config.app.tokenizers_dir) {
        Ok(t) => t,
        Err(e) => {
            log::error!("error loading tokenizers: {}", e);
            std::process::exit(1);
        }
    };

    // Initialize manager.
    let mgr = match Manager::new(db, tokenizers, langs.clone(), dicts.clone()).await {
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

        // Global constants populated from config.
        consts: Consts {
            root_url: config.app.root_url,
            enable_pages: config.app.enable_pages,
            enable_submissions: config.app.enable_submissions,
            enable_glossary: config.glossary.enabled,
            admin_username: config.app.admin_username,
            admin_password: config.app.admin_password,

            api_default_per_page: config.api_results.per_page,
            api_max_per_page: config.api_results.max_per_page,

            site_default_per_page: config.site_results.per_page,
            site_max_per_page: config.site_results.max_per_page,
            site_num_page_nums: config.site_results.num_page_nums,
            site_max_relations_per_type: config.site_results.max_entry_relations_per_type,
            site_max_content_items: config.site_results.max_entry_content_items,

            glossary_default_per_page: config.glossary.default_per_page,
            glossary_max_per_page: config.glossary.max_per_page,
            glossary_num_page_nums: config.glossary.num_page_nums,

            // Split admin assets by file extension for template rendering.
            admin_js_assets: config
                .app
                .admin_assets
                .iter()
                .filter(|a| a.ends_with(".js"))
                .cloned()
                .collect(),
            admin_css_assets: config
                .app
                .admin_assets
                .iter()
                .filter(|a| a.ends_with(".css"))
                .cloned()
                .collect(),
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
