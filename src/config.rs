use std::path::{Path, PathBuf};

use crate::models::Config;

const SAMPLE_CONFIG: &str = include_str!("../config.sample.toml");

/// Load and merge one or more config files.
pub fn load_all(paths: &[PathBuf]) -> Config {
    let mut config: Option<Config> = None;

    for path in paths {
        log::info!("loading config: {}", path.display());
        match read_file(path) {
            Ok(c) => {
                if let Some(ref mut existing) = config {
                    // Merge configs.
                    merge(existing, c);
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

/// Generate sample config file.
pub fn generate_sample(path: &Path) -> Result<(), Box<dyn std::error::Error>> {
    if path.exists() {
        return Err("config file already exists".into());
    }
    std::fs::write(path, SAMPLE_CONFIG)?;
    Ok(())
}

/// Load configuration from a given TOML file.
fn read_file(path: &Path) -> Result<Config, Box<dyn std::error::Error>> {
    let content = std::fs::read_to_string(path)?;
    let cfg: Config = toml::from_str(&content)?;
    Ok(cfg)
}

/// Merge the given src config into the dest config struct.
fn merge(dest: &mut Config, src: Config) {
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
    if !src.app.admin_assets.is_empty() {
        dest.app.admin_assets = src.app.admin_assets;
    }
    dest.app.enable_pages = src.app.enable_pages;
    dest.app.enable_submissions = src.app.enable_submissions;

    // Merge DB config.
    if !src.db.path.is_empty() {
        dest.db.path = src.db.path;
    }
    if src.db.max_conns > 0 {
        dest.db.max_conns = src.db.max_conns;
    }

    // Merge API results config.
    if src.api_results.per_page > 0 {
        dest.api_results.per_page = src.api_results.per_page;
    }
    if src.api_results.max_per_page > 0 {
        dest.api_results.max_per_page = src.api_results.max_per_page;
    }

    // Merge site results config.
    if src.site_results.per_page > 0 {
        dest.site_results.per_page = src.site_results.per_page;
    }
    if src.site_results.max_per_page > 0 {
        dest.site_results.max_per_page = src.site_results.max_per_page;
    }
    if src.site_results.num_page_nums > 0 {
        dest.site_results.num_page_nums = src.site_results.num_page_nums;
    }
    if src.site_results.max_entry_relations_per_type > 0 {
        dest.site_results.max_entry_relations_per_type =
            src.site_results.max_entry_relations_per_type;
    }
    if src.site_results.max_entry_content_items > 0 {
        dest.site_results.max_entry_content_items = src.site_results.max_entry_content_items;
    }

    // Merge glossary config.
    dest.glossary.enabled = src.glossary.enabled;
    if src.glossary.default_per_page > 0 {
        dest.glossary.default_per_page = src.glossary.default_per_page;
    }
    if src.glossary.max_per_page > 0 {
        dest.glossary.max_per_page = src.glossary.max_per_page;
    }
    if src.glossary.num_page_nums > 0 {
        dest.glossary.num_page_nums = src.glossary.num_page_nums;
    }

    // Merge languages.
    for (id, lang) in src.lang {
        dest.lang.insert(id, lang);
    }
}
