use std::{io::Write, path::Path};

use crate::init;

/// Generate sitemap files for entries in the DB.
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

    // Create the output directory.
    fs::create_dir_all(output_dir)?;

    // Get all entries for `from_lang``.
    let db = init::init_db(db_path, 1, true).await?;
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
