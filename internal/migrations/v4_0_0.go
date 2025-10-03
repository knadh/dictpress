package migrations

import (
	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
)

// V4_0_0 performs the DB migrations.
func V4_0_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	if _, err := db.Exec(`
		DO $$
		BEGIN
			-- Check if column is not already an array type
			IF EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_name = 'entries'
				AND column_name = 'content'
				AND UPPER(data_type) = 'TEXT'
			) THEN
				-- Add temporary column.
				ALTER TABLE entries ADD COLUMN IF NOT EXISTS content_array TEXT[];

				-- Convert existing data to single-element arrays.
				UPDATE entries SET content_array = ARRAY[content];

				-- Drop old column and rename new one.
				ALTER TABLE entries DROP COLUMN content;
				ALTER TABLE entries RENAME COLUMN content_array TO content;

				-- Add constraints.
				ALTER TABLE entries ALTER COLUMN content SET NOT NULL;
				ALTER TABLE entries ADD CONSTRAINT check_content_not_empty CHECK (ARRAY_LENGTH(content, 1) > 0);

				-- Recreate index with new array syntax.
				DROP INDEX IF EXISTS idx_entries_content;
				CREATE INDEX idx_entries_content ON entries((LOWER(SUBSTRING(content[1], 0, 50))));
			END IF;
		END
		$$;
	`); err != nil {
		return err
	}

	return nil
}
