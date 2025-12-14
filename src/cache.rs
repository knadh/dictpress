use std::time::{Duration, SystemTime, UNIX_EPOCH};

use bytes::{BufMut, Bytes, BytesMut};
use foyer::{
    BlockEngineBuilder, Cache as FoyerCache, CacheBuilder, Compression, DeviceBuilder,
    FsDeviceBuilder, HybridCache, HybridCacheBuilder, RecoverMode,
};
use serde::Deserialize;
use thiserror::Error;

use crate::models::SearchQuery;

const MODE_MEMORY: &str = "memory";
const MODE_HYBRID: &str = "hybrid";

/// Size of TTL prefix (u64 timestamp).
const TTL_PREFIX_SIZE: usize = 8;

/// Cache configuration.
#[derive(Debug, Clone, Deserialize)]
pub struct CacheConfig {
    #[serde(default)]
    pub enabled: bool,

    /// TTL duration string (e.g., "72h", "30m", "1d").
    #[serde(default = "default_cache_ttl")]
    pub ttl: String,

    /// Cache mode: "memory" or "hybrid".
    #[serde(default = "default_cache_mode")]
    pub mode: String,

    /// Maximum memory in MB for in-memory cache.
    #[serde(default = "default_cache_memory")]
    pub max_memory_mb: u64,

    /// Maximum disk size in MB for hybrid mode.
    #[serde(default = "default_cache_disk")]
    pub max_disk_mb: u64,

    /// Directory for disk cache (hybrid mode only).
    #[serde(default = "default_cache_dir")]
    pub dir: String,
}

fn default_cache_ttl() -> String {
    "72h".to_string()
}

fn default_cache_mode() -> String {
    "hybrid".to_string()
}

fn default_cache_memory() -> u64 {
    128
}

fn default_cache_disk() -> u64 {
    512
}

fn default_cache_dir() -> String {
    "/tmp/dictpress-cache".to_string()
}

impl Default for CacheConfig {
    fn default() -> Self {
        Self {
            enabled: false,
            ttl: default_cache_ttl(),
            mode: default_cache_mode(),
            max_memory_mb: default_cache_memory(),
            max_disk_mb: default_cache_disk(),
            dir: default_cache_dir(),
        }
    }
}

#[derive(Debug, Error)]
pub enum CacheError {
    #[error("cache build error: {0}")]
    Build(String),

    #[error("invalid TTL format: {0}")]
    InvalidTtl(String),

    #[error("invalid cache mode: {0}")]
    InvalidMode(String),
}

/// Cache backend abstraction.
enum CacheBackend {
    Memory(FoyerCache<String, Bytes>),
    Hybrid(HybridCache<String, Bytes>),
}

/// Cache wrapper with TTL support.
pub struct Cache {
    backend: CacheBackend,
    ttl: Duration,
}

impl Cache {
    /// Create a new cache instance.
    pub async fn new(cfg: &CacheConfig) -> Result<Self, CacheError> {
        let ttl = parse_duration(&cfg.ttl)?;
        let memory_bytes = (cfg.max_memory_mb * 1024 * 1024) as usize;

        let backend = match cfg.mode.as_str() {
            MODE_MEMORY => {
                let cache = CacheBuilder::new(memory_bytes)
                    .with_weighter(|_key, value: &Bytes| value.len())
                    .build();
                CacheBackend::Memory(cache)
            }

            MODE_HYBRID => {
                let disk_bytes = (cfg.max_disk_mb * 1024 * 1024) as usize;

                // Build filesystem device.
                let device = FsDeviceBuilder::new(&cfg.dir)
                    .with_capacity(disk_bytes)
                    .build()
                    .map_err(|e| CacheError::Build(e.to_string()))?;

                // Build hybrid cache.
                let cache = HybridCacheBuilder::new()
                    .memory(memory_bytes)
                    .storage()
                    .with_compression(Compression::None)
                    .with_engine_config(BlockEngineBuilder::new(device))
                    .with_recover_mode(RecoverMode::Quiet)
                    .build()
                    .await
                    .map_err(|e| CacheError::Build(e.to_string()))?;

                CacheBackend::Hybrid(cache)
            }
            _ => return Err(CacheError::InvalidMode(cfg.mode.clone())),
        };

        Ok(Self { backend, ttl })
    }

    /// Get a value from the cache. Returns None if not found or expired.
    pub async fn get(&self, key: &str) -> Option<Bytes> {
        let raw = match &self.backend {
            CacheBackend::Memory(c) => c.get(key).map(|e| e.value().clone()),
            CacheBackend::Hybrid(c) => match c.get(key).await {
                Ok(Some(entry)) => Some(entry.value().clone()),
                Ok(None) => None,
                Err(e) => {
                    log::warn!("cache get (hybrid) key={}: error: {}", key, e);
                    None
                }
            },
        }?;

        // Need at least TTL prefix.
        if raw.len() < TTL_PREFIX_SIZE {
            return None;
        }

        // Read TTL from first 8 bytes.
        let created_at = u64::from_le_bytes(raw[..TTL_PREFIX_SIZE].try_into().ok()?);

        let now = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap_or_default()
            .as_secs();

        if now.saturating_sub(created_at) > self.ttl.as_secs() {
            return None;
        }

        // Return data slice.
        Some(raw.slice(TTL_PREFIX_SIZE..))
    }

    /// Store a value in the cache with current timestamp prefix.
    pub fn put(&self, key: &str, value: &[u8]) {
        let now = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap_or_default()
            .as_secs();

        // Prepend TTL timestamp (first 8 bytes).
        let mut buf = BytesMut::with_capacity(TTL_PREFIX_SIZE + value.len());
        buf.put_u64_le(now);
        buf.extend_from_slice(value);
        let data = buf.freeze();

        match &self.backend {
            CacheBackend::Memory(c) => {
                c.insert(key.to_string(), data);
            }
            CacheBackend::Hybrid(c) => {
                c.insert(key.to_string(), data);
            }
        }
    }

    /// Close the cache and flush pending writes.
    pub async fn close(&self) {
        if let CacheBackend::Hybrid(c) = &self.backend {
            c.close().await.ok();
        }
    }
}

/// Generate a cache key for search queries.
/// Matches the Go implementation's makeQueryCacheKey().
pub fn make_search_cache_key(q: &SearchQuery) -> String {
    let mut types = q.types.clone();
    types.sort();
    let mut tags = q.tags.clone();
    tags.sort();

    let key = format!(
        "s:{}:{}:{}:{}:{}:{}:{}:{}",
        q.from_lang,
        q.to_lang,
        q.query.to_lowercase().trim(),
        types.join(","),
        tags.join(","),
        q.status,
        q.page,
        q.per_page
    );

    let digest = md5::compute(key.as_bytes());
    format!("s:{:x}", digest)
}

/// Generate a cache key for glossary queries.
/// Matches the Go implementation's makeGlossaryCacheKey().
pub fn make_glossary_cache_key(lang: &str, initial: &str, offset: i32, limit: i32) -> String {
    let key = format!("g:{}:{}:{}:{}", lang, initial, offset, limit);
    let digest = md5::compute(key.as_bytes());
    format!("g:{:x}", digest)
}

/// Parse a duration string like "72h", "30m", "1d" into Duration.
fn parse_duration(s: &str) -> Result<Duration, CacheError> {
    let s = s.trim();
    if s.is_empty() {
        return Err(CacheError::InvalidTtl("empty duration".to_string()));
    }

    let (num_str, unit) = s.split_at(s.len() - 1);
    let num: u64 = num_str
        .parse()
        .map_err(|_| CacheError::InvalidTtl(s.to_string()))?;

    let secs = match unit {
        "s" => num,
        "m" => num * 60,
        "h" => num * 3600,
        "d" => num * 86400,
        _ => return Err(CacheError::InvalidTtl(s.to_string())),
    };

    Ok(Duration::from_secs(secs))
}
