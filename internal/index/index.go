package index

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/user/bunbu-shelf/internal/book"
)

// Reindex scans all .md files in libraryDir/books/ and rebuilds the index
// incrementally: only files whose mod_time has changed are re-parsed.
func (s *Store) Reindex(libraryDir string) error {
	booksDir := filepath.Join(libraryDir, "books")

	entries, err := os.ReadDir(booksDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read books dir %s: %w", booksDir, err)
	}

	existing, err := s.existingModTimes()
	if err != nil {
		return err
	}

	seen := make(map[string]bool)

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	var committed bool
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		path := filepath.Join(booksDir, entry.Name())
		slug := book.SlugFromPath(path)
		seen[slug] = true

		info, err := entry.Info()
		if err != nil {
			continue
		}
		mtime := info.ModTime()

		if prev, ok := existing[slug]; ok && !mtime.After(prev) {
			continue
		}

		b, err := book.ParseFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "skipping %s: %v\n", path, err)
			continue
		}
		if err := indexBookTx(tx, b); err != nil {
			return fmt.Errorf("index %s: %w", path, err)
		}
	}

	for slug := range existing {
		if !seen[slug] {
			if _, err := tx.Exec(`DELETE FROM books WHERE slug = ?`, slug); err != nil {
				return fmt.Errorf("delete %s: %w", slug, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	committed = true
	return nil
}

// IndexBook upserts a single book into the index.
func (s *Store) IndexBook(b *book.Book) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	var committed bool
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if err := indexBookTx(tx, b); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

// DeleteBook removes a book from the index by slug.
func (s *Store) DeleteBook(slug string) error {
	_, err := s.db.Exec(`DELETE FROM books WHERE slug = ?`, slug)
	return err
}

func (s *Store) existingModTimes() (map[string]time.Time, error) {
	rows, err := s.db.Query(`SELECT slug, mod_time FROM books`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	m := make(map[string]time.Time)
	for rows.Next() {
		var slug string
		var modTime time.Time
		if err := rows.Scan(&slug, &modTime); err != nil {
			return nil, err
		}
		m[slug] = modTime
	}
	return m, rows.Err()
}

func indexBookTx(tx *sql.Tx, b *book.Book) error {
	themesJSON, err := json.Marshal(b.Meta.Themes)
	if err != nil {
		return err
	}

	var started, finished, acquired *string
	if b.Meta.Started != nil {
		s := b.Meta.Started.String()
		started = &s
	}
	if b.Meta.Finished != nil {
		f := b.Meta.Finished.String()
		finished = &f
	}
	if b.Meta.Acquired != nil {
		a := b.Meta.Acquired.String()
		acquired = &a
	}

	_, err = tx.Exec(`
		INSERT INTO books (
			slug, file_path, title, author, status, track,
			started, finished, rating, themes, isbn, cover,
			acquired, source, publisher, pages, publish_year,
			description, copies, body_md, mod_time
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(slug) DO UPDATE SET
			file_path    = excluded.file_path,
			title        = excluded.title,
			author       = excluded.author,
			status       = excluded.status,
			track        = excluded.track,
			started      = excluded.started,
			finished     = excluded.finished,
			rating       = excluded.rating,
			themes       = excluded.themes,
			isbn         = excluded.isbn,
			cover        = excluded.cover,
			acquired     = excluded.acquired,
			source       = excluded.source,
			publisher    = excluded.publisher,
			pages        = excluded.pages,
			publish_year = excluded.publish_year,
			description  = excluded.description,
			copies       = excluded.copies,
			body_md      = excluded.body_md,
			mod_time     = excluded.mod_time,
			indexed_at   = CURRENT_TIMESTAMP
	`,
		b.Slug, b.FilePath, b.Meta.Title, b.Meta.Author,
		string(b.Meta.Status), b.Meta.Track,
		started, finished, b.Meta.Rating,
		string(themesJSON), b.Meta.ISBN, b.Meta.Cover,
		acquired, b.Meta.Source, b.Meta.Publisher, b.Meta.Pages, b.Meta.PublishYear,
		b.Meta.Description, b.Meta.Copies, b.Body,
		b.ModTime,
	)
	return err
}
