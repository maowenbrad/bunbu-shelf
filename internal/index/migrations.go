package index

import (
	"database/sql"
	"fmt"
)

func migrate(db *sql.DB) error {
	// Ensure the version table exists so we can read the current version.
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`); err != nil {
		return fmt.Errorf("create schema_version: %w", err)
	}

	var current int
	row := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_version`)
	if err := row.Scan(&current); err != nil {
		return fmt.Errorf("read schema_version: %w", err)
	}

	if current >= schemaVersion {
		return nil
	}

	// Apply the full DDL (all statements are IF NOT EXISTS, safe to re-apply).
	if _, err := db.Exec(schemaDDL); err != nil {
		return fmt.Errorf("apply schema DDL: %w", err)
	}

	if _, err := db.Exec(`DELETE FROM schema_version`); err != nil {
		return err
	}
	if _, err := db.Exec(`INSERT INTO schema_version (version) VALUES (?)`, schemaVersion); err != nil {
		return fmt.Errorf("update schema_version: %w", err)
	}
	return nil
}
