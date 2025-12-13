use std::path::{Path, PathBuf};

use crate::models::Config;

const SAMPLE_CONFIG: &str = include_str!("../config.sample.toml");

/// Load and merge one or more config files.
pub fn load_all(paths: &[PathBuf]) -> Config {
    let mut config: Option<toml::Value> = None;

    for path in paths {
        log::info!("loading config: {}", path.display());
        match read_file(path) {
            Ok(c) => {
                if let Some(ref mut existing) = config {
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

    let val = config.unwrap_or_else(|| {
        log::error!("no config files specified");
        std::process::exit(1);
    });

    val.try_into().unwrap_or_else(|e| {
        log::error!("invalid config: {}", e);
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
fn read_file(path: &Path) -> Result<toml::Value, Box<dyn std::error::Error>> {
    let content = std::fs::read_to_string(path)?;
    let val: toml::Value = toml::from_str(&content)?;

    Ok(val)
}

/// Recursively merge src TOML value into dest.
fn merge(dest: &mut toml::Value, src: toml::Value) {
    match (dest, src) {
        (toml::Value::Table(dest_tbl), toml::Value::Table(src_tbl)) => {
            for (key, src_val) in src_tbl {
                match dest_tbl.get_mut(&key) {
                    Some(dest_val) => merge(dest_val, src_val),
                    None => {
                        dest_tbl.insert(key, src_val);
                    }
                }
            }
        }
        (dest, src) => *dest = src,
    }
}
