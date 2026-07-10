package index_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/user/bunbu-shelf/internal/book"
	"github.com/user/bunbu-shelf/internal/index"
)

// v1BooksSchema recreates the pre-v2 schema in full — including the FTS5
// shadow table and triggers, which every real v1 database has had since
// creation. (An earlier version of this fixture omitted books_fts, which
// doesn't reflect any real upgrade path: FTS5 external-content tables track
// a consistency checksum per row, and a row that predates the FTS triggers
// trips "database disk image is malformed" on its first UPDATE — a fixture
// bug, not a migration bug.)
const v1BooksSchema = `
CREATE TABLE schema_version (version INTEGER NOT NULL);
INSERT INTO schema_version (version) VALUES (1);

CREATE TABLE books (
    slug        TEXT PRIMARY KEY,
    file_path   TEXT NOT NULL,
    title       TEXT NOT NULL,
    author      TEXT NOT NULL,
    status      TEXT NOT NULL,
    track       TEXT NOT NULL DEFAULT '',
    started     TEXT,
    finished    TEXT,
    rating      INTEGER,
    themes      TEXT NOT NULL DEFAULT '[]',
    isbn        TEXT NOT NULL DEFAULT '',
    cover       TEXT NOT NULL DEFAULT '',
    acquired    TEXT,
    source      TEXT NOT NULL DEFAULT '',
    body_md     TEXT NOT NULL DEFAULT '',
    mod_time    DATETIME NOT NULL,
    indexed_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE VIRTUAL TABLE books_fts USING fts5(
    title,
    author,
    themes,
    body_md,
    content=books,
    content_rowid=rowid,
    tokenize='porter unicode61'
);

CREATE TRIGGER books_ai AFTER INSERT ON books BEGIN
    INSERT INTO books_fts(rowid, title, author, themes, body_md)
    VALUES (new.rowid, new.title, new.author, new.themes, new.body_md);
END;

CREATE TRIGGER books_ad AFTER DELETE ON books BEGIN
    INSERT INTO books_fts(books_fts, rowid, title, author, themes, body_md)
    VALUES ('delete', old.rowid, old.title, old.author, old.themes, old.body_md);
END;

CREATE TRIGGER books_au AFTER UPDATE ON books BEGIN
    INSERT INTO books_fts(books_fts, rowid, title, author, themes, body_md)
    VALUES ('delete', old.rowid, old.title, old.author, old.themes, old.body_md);
    INSERT INTO books_fts(rowid, title, author, themes, body_md)
    VALUES (new.rowid, new.title, new.author, new.themes, new.body_md);
END;
`

func TestMigrateAddsV2ColumnsToExistingDB(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "v1.db")

	// Seed a v1-shaped database directly, bypassing index.Open. Use the same
	// DSN pragmas Open() itself uses so the file is left in a state that a
	// second connection can safely reopen.
	dsn := "file:" + tmp + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=synchronous(NORMAL)"
	seed, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open seed db: %v", err)
	}
	if _, err := seed.Exec(v1BooksSchema); err != nil {
		t.Fatalf("seed v1 schema: %v", err)
	}
	if _, err := seed.Exec(`
		INSERT INTO books (slug, file_path, title, author, status, mod_time)
		VALUES ('old-book', '/tmp/old-book.md', 'Old Book', 'Someone', 'queued', CURRENT_TIMESTAMP)
	`); err != nil {
		t.Fatalf("seed row: %v", err)
	}
	if err := seed.Close(); err != nil {
		t.Fatalf("close seed db: %v", err)
	}

	// Opening the store should add the missing v2 columns without error and
	// without losing the pre-existing row.
	s, err := index.Open(tmp)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	got, err := s.Get("old-book")
	if err != nil {
		t.Fatalf("Get old-book after migration: %v", err)
	}
	if got.Meta.Title != "Old Book" {
		t.Errorf("title: got %q, want %q", got.Meta.Title, "Old Book")
	}

	// New columns should be writable/readable now.
	b := got
	b.Meta.Publisher = "Test Press"
	b.Meta.Pages = 321
	b.Meta.PublishYear = 1999
	b.Meta.Description = "A test description."
	b.Meta.Copies = 2
	if err := s.IndexBook(b); err != nil {
		t.Fatalf("IndexBook after migration: %v", err)
	}

	updated, err := s.Get("old-book")
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if updated.Meta.Publisher != "Test Press" {
		t.Errorf("publisher: got %q, want %q", updated.Meta.Publisher, "Test Press")
	}
	if updated.Meta.Pages != 321 {
		t.Errorf("pages: got %d, want 321", updated.Meta.Pages)
	}
	if updated.Meta.PublishYear != 1999 {
		t.Errorf("publish year: got %d, want 1999", updated.Meta.PublishYear)
	}
	if updated.Meta.Description != "A test description." {
		t.Errorf("description: got %q", updated.Meta.Description)
	}
	if updated.Meta.Copies != 2 {
		t.Errorf("copies: got %d, want 2", updated.Meta.Copies)
	}
}

func TestMigrateIsIdempotentOnFreshDB(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "fresh.db")

	// Opening a brand-new database twice should not error even though
	// addMissingColumns runs on every startup.
	s1, err := index.Open(tmp)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	s2, err := index.Open(tmp)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer s2.Close()

	b := &book.Book{
		Slug:     "fresh-book",
		FilePath: "/tmp/fresh-book.md",
		Meta: book.Frontmatter{
			Title:  "Fresh Book",
			Author: "Someone",
			Status: book.StatusQueued,
		},
	}
	if err := s2.IndexBook(b); err != nil {
		t.Fatalf("IndexBook: %v", err)
	}
}
