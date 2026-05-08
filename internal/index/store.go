package index

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // register "sqlite" driver
)

// Store wraps the SQLite database for the book index.
type Store struct {
	db     *sql.DB
	dbPath string
}

// Open opens (or creates) the SQLite database at dbPath and runs migrations.
func Open(dbPath string) (*Store, error) {
	dsn := "file:" + dbPath + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", dbPath, err)
	}
	// Verify the connection.
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite %s: %w", dbPath, err)
	}
	s := &Store{db: db, dbPath: dbPath}
	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// Close releases the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// DB returns the underlying *sql.DB for use in tests or advanced queries.
func (s *Store) DB() *sql.DB {
	return s.db
}
