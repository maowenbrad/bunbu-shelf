package index

import (
	"database/sql"
	"fmt"
	"strings"
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

	if current < schemaVersion {
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
	}

	// Self-healing column check: runs every startup regardless of version,
	// so it also repairs a database that was upgraded before this check
	// existed. Cheap — a PRAGMA read plus a handful of map lookups.
	return addMissingColumns(db)
}

// booksV2Columns are the columns added to the books table in schema v2.
// CREATE TABLE IF NOT EXISTS already includes them for a brand-new
// database; this only fires when upgrading a database created before v2.
var booksV2Columns = []string{
	"publisher    TEXT NOT NULL DEFAULT ''",
	"pages        INTEGER NOT NULL DEFAULT 0",
	"publish_year INTEGER NOT NULL DEFAULT 0",
	"description  TEXT NOT NULL DEFAULT ''",
	"copies       INTEGER NOT NULL DEFAULT 0",
}

func addMissingColumns(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(books)`)
	if err != nil {
		return fmt.Errorf("read books schema: %w", err)
	}
	existing := make(map[string]bool)
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt interface{}
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			rows.Close()
			return fmt.Errorf("scan table_info: %w", err)
		}
		existing[name] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()

	for _, def := range booksV2Columns {
		name := strings.Fields(def)[0]
		if existing[name] {
			continue
		}
		if _, err := db.Exec(`ALTER TABLE books ADD COLUMN ` + def); err != nil {
			return fmt.Errorf("add column %s: %w", name, err)
		}
	}
	return nil
}
