use std::path::PathBuf;

use clap::{Parser, Subcommand};

#[derive(Parser)]
#[command(name = "dictpress")]
#[command(about = "dictpress - Build dictionary websites. https://dict.press")]
#[command(version = env!("VERSION"))]
pub struct Cli {
    /// Path to one or more config files (merged in order).
    #[arg(long, default_value = "config.toml", action = clap::ArgAction::Append)]
    pub config: Vec<PathBuf>,

    /// Path to SQLite database file.
    #[arg(long = "db", default_value = "data.db")]
    pub db_path: PathBuf,

    /// Path to site theme directory. If left empty, only HTTP APIs at /api/* will be available,
    /// and there is no site rendered at /.
    #[arg(long)]
    pub site: Option<PathBuf>,

    #[command(subcommand)]
    pub command: Option<Commands>,
}

#[derive(Subcommand)]
pub enum Commands {
    /// Generate a sample config file.
    NewConfig {
        /// Output path for config file.
        #[arg(short, long, default_value = "config.toml")]
        path: PathBuf,
    },

    /// Run first time DB installation.
    Install {
        /// Assume 'yes' to any manual prompts during installation.
        #[arg(long)]
        yes: bool,
    },

    /// Upgrade database to the current version.
    Upgrade {
        /// Assume 'yes' to any manual prompts during upgrade.
        #[arg(long)]
        yes: bool,
    },

    /// Import a CSV file into the database.
    Import {
        /// CSV file to import.
        #[arg(long)]
        file: PathBuf,
    },

    /// Generate static sitemap files for all dictionary entries.
    Sitemap {
        /// Language to translate from.
        #[arg(long)]
        from_lang: String,

        /// Language to translate to.
        #[arg(long)]
        to_lang: String,

        /// Root URL where sitemaps will be placed (for robots.txt).
        #[arg(long)]
        url: Option<String>,

        /// Maximum number of URL rows per sitemap file.
        #[arg(long, default_value = "49990")]
        max_rows: usize,

        /// Prefix for the sitemap files.
        #[arg(long, default_value = "sitemap")]
        output_prefix: String,

        /// Directory to generate the files in.
        #[arg(long, default_value = "sitemaps")]
        output_dir: PathBuf,

        /// Generate robots.txt.
        #[arg(long)]
        robots: bool,
    },
}
