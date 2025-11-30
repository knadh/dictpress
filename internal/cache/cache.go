package cache

import (
	"log"
	"time"

	"github.com/dgraph-io/badger/v4"
)

const (
	CacheTypeMemory = "memory"
	CacheTypeHybrid = "hybrid"
)

// Config holds cache configuration.
type Config struct {
	TTL time.Duration
	// "memory" or "hybrid"
	Mode     string
	CacheDir string

	// MBs
	MaxMemory int64
}

// Cache is a simple abstraction on top of Badger for caching results.
type Cache struct {
	defaultTTL time.Duration

	db *badger.DB
	lo *log.Logger
}

// New creates and returns a new Cache instance.
func New(cfg Config, lo *log.Logger) (*Cache, error) {
	opts := badger.DefaultOptions(cfg.CacheDir)

	// Suppress Badger's default logging.
	opts.Logger = nil

	if cfg.Mode == CacheTypeMemory {
		opts.InMemory = true
		opts.Dir = ""
		opts.ValueDir = ""
	}

	// Apply memory limits if specified.
	if cfg.MaxMemory > 0 {
		maxBytes := cfg.MaxMemory << 20 // Convert MB to bytes

		// Distribute memory to 50% block cache, 25% memtables, and 25% index cache.
		// Opinionated!
		opts.BlockCacheSize = maxBytes / 2
		opts.IndexCacheSize = maxBytes / 4

		// MemTableSize * NumMemtables should fit in remaining ~25%.
		// Use smaller memtables with fewer of them.
		opts.MemTableSize = maxBytes / 8
		opts.NumMemtables = 2
	}

	db, err := badger.Open(opts)
	if err != nil {
		return nil, err
	}

	return &Cache{
		defaultTTL: cfg.TTL,
		db:         db,
		lo:         lo,
	}, nil
}

// Get retrieves a value by key. Doesn't return an error if key is not found.
func (c *Cache) Get(key string) ([]byte, error) {
	var val []byte

	err := c.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err != nil {
			return err
		}

		val, err = item.ValueCopy(nil)
		return err
	})

	if err == badger.ErrKeyNotFound {
		return nil, nil
	}

	return val, err
}

// Put stores a value with the given key and an optional TTL.
// If ttl is nil, the default TTL from config is used.
func (c *Cache) Put(key string, val []byte, ttl *time.Duration) error {
	t := c.defaultTTL
	if ttl != nil {
		t = *ttl
	}

	return c.db.Update(func(txn *badger.Txn) error {
		e := badger.NewEntry([]byte(key), val).WithTTL(t)
		return txn.SetEntry(e)
	})
}

// Delete deletes a key from the cache.
func (c *Cache) Delete(key string) error {
	return c.db.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(key))
	})
}

// Purge deletes all entries from the cache.
func (c *Cache) Purge() error {
	return c.db.DropAll()
}

// Close closes the underlying Badger database.
func (c *Cache) Close() error {
	return c.db.Close()
}
