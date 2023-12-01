package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V2_0_0 performs the DB migrations.
func V2_0_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS settings (
			key             TEXT NOT NULL UNIQUE,
			value           JSONB NOT NULL DEFAULT '{}',
			updated_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_settings_key ON settings(key);
	`); err != nil {
		return err
	}

	if _, err := db.Exec(`ALTER TABLE entries ADD COLUMN IF NOT EXISTS meta JSONB NOT NULL DEFAULT '{}'`); err != nil {
		return err
	}

	return nil
}
