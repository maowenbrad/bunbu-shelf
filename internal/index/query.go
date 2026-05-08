package index

import (
	"encoding/json"
	"fmt"

	"github.com/user/bunbu-shelf/internal/book"
)

// SearchResult is a lightweight result type used for lists and search.
type SearchResult struct {
	Slug    string
	Title   string
	Author  string
	Status  book.Status
	Track   string
	Rating  *int
	Cover   string
	Themes  []string
	Snippet string // populated by FTS highlight()
}

// ThemeCount pairs a theme with the number of books tagged with it.
type ThemeCount struct {
	Theme string
	Count int
}

// ListOpts controls filtering and sorting for List.
type ListOpts struct {
	Status  book.Status
	Track   string
	OrderBy string // "title" | "author" | "finished" | "rating"
	Desc    bool
}

// Search performs full-text search using FTS5.
func (s *Store) Search(query string, limit int) ([]SearchResult, error) {
	rows, err := s.db.Query(`
		SELECT
			b.slug, b.title, b.author, b.status, b.track, b.rating, b.cover, b.themes,
			snippet(books_fts, 3, '<mark>', '</mark>', '…', 32) AS snippet
		FROM books_fts
		JOIN books b ON b.rowid = books_fts.rowid
		WHERE books_fts MATCH ?
		ORDER BY rank
		LIMIT ?
	`, query, limit)
	if err != nil {
		return nil, fmt.Errorf("fts search: %w", err)
	}
	defer rows.Close()
	return scanResults(rows, true)
}

// Get returns the full book record for a given slug, or nil if not found.
func (s *Store) Get(slug string) (*book.Book, error) {
	row := s.db.QueryRow(`
		SELECT slug, file_path, title, author, status, track,
		       started, finished, rating, themes, isbn, cover,
		       acquired, source, body_md, mod_time
		FROM books WHERE slug = ?
	`, slug)

	var b book.Book
	var started, finished, acquired *string
	var rating *int
	var themesJSON string
	var modTimeStr string

	err := row.Scan(
		&b.Slug, &b.FilePath, &b.Meta.Title, &b.Meta.Author,
		&b.Meta.Status, &b.Meta.Track,
		&started, &finished, &rating,
		&themesJSON, &b.Meta.ISBN, &b.Meta.Cover,
		&acquired, &b.Meta.Source, &b.Body, &modTimeStr,
	)
	if err != nil {
		return nil, fmt.Errorf("get book %s: %w", slug, err)
	}

	b.Meta.Rating = rating

	if started != nil {
		d := &book.Date{}
		if err := scanDate(d, *started); err == nil {
			b.Meta.Started = d
		}
	}
	if finished != nil {
		d := &book.Date{}
		if err := scanDate(d, *finished); err == nil {
			b.Meta.Finished = d
		}
	}
	if acquired != nil {
		d := &book.Date{}
		if err := scanDate(d, *acquired); err == nil {
			b.Meta.Acquired = d
		}
	}

	var themes []string
	if err := json.Unmarshal([]byte(themesJSON), &themes); err == nil {
		b.Meta.Themes = themes
	}

	return &b, nil
}

// List returns books matching the given options.
func (s *Store) List(opts ListOpts) ([]SearchResult, error) {
	q := `SELECT slug, title, author, status, track, rating, cover, themes, '' AS snippet FROM books`
	var args []interface{}
	var where []string

	if opts.Status != "" {
		where = append(where, "status = ?")
		args = append(args, string(opts.Status))
	}
	if opts.Track != "" {
		where = append(where, "track = ?")
		args = append(args, opts.Track)
	}

	if len(where) > 0 {
		q += " WHERE " + joinAnd(where)
	}

	order := "title"
	switch opts.OrderBy {
	case "author", "finished", "rating":
		order = opts.OrderBy
	}
	if opts.Desc {
		q += " ORDER BY " + order + " DESC"
	} else {
		q += " ORDER BY " + order + " ASC"
	}

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanResults(rows, false)
}

// ListByStatus returns all books with the given status, ordered by title.
func (s *Store) ListByStatus(status book.Status) ([]SearchResult, error) {
	return s.List(ListOpts{Status: status})
}

// ListByTheme returns books that contain a specific theme tag.
func (s *Store) ListByTheme(theme string) ([]SearchResult, error) {
	rows, err := s.db.Query(`
		SELECT DISTINCT b.slug, b.title, b.author, b.status, b.track, b.rating, b.cover, b.themes, '' AS snippet
		FROM books b, json_each(b.themes) t
		WHERE t.value = ?
		ORDER BY b.title ASC
	`, theme)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanResults(rows, false)
}

// AllThemes returns all distinct theme values with their book counts.
func (s *Store) AllThemes() ([]ThemeCount, error) {
	rows, err := s.db.Query(`
		SELECT t.value AS theme, COUNT(*) AS cnt
		FROM books b, json_each(b.themes) t
		GROUP BY t.value
		ORDER BY cnt DESC, t.value ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ThemeCount
	for rows.Next() {
		var tc ThemeCount
		if err := rows.Scan(&tc.Theme, &tc.Count); err != nil {
			return nil, err
		}
		out = append(out, tc)
	}
	return out, rows.Err()
}

// RecentFinishes returns finished books ordered by finished date descending.
func (s *Store) RecentFinishes(limit int) ([]SearchResult, error) {
	rows, err := s.db.Query(`
		SELECT slug, title, author, status, track, rating, cover, themes, '' AS snippet
		FROM books
		WHERE status = 'finished' AND finished IS NOT NULL
		ORDER BY finished DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanResults(rows, false)
}

// CurrentlyReading returns books with status=reading, optionally filtered by track.
func (s *Store) CurrentlyReading() ([]SearchResult, error) {
	rows, err := s.db.Query(`
		SELECT slug, title, author, status, track, rating, cover, themes, '' AS snippet
		FROM books
		WHERE status = 'reading'
		ORDER BY started DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanResults(rows, false)
}

// Queue returns books with status=queued, ordered by title.
func (s *Store) Queue() ([]SearchResult, error) {
	return s.ListByStatus(book.StatusQueued)
}

func scanResults(rows interface {
	Next() bool
	Scan(dest ...interface{}) error
	Err() error
}, hasSnippet bool) ([]SearchResult, error) {
	var out []SearchResult
	for rows.Next() {
		var r SearchResult
		var rating *int
		var themesJSON string
		err := rows.Scan(
			&r.Slug, &r.Title, &r.Author, &r.Status, &r.Track,
			&rating, &r.Cover, &themesJSON, &r.Snippet,
		)
		if err != nil {
			return nil, err
		}
		r.Rating = rating
		var themes []string
		if err := json.Unmarshal([]byte(themesJSON), &themes); err == nil {
			r.Themes = themes
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func scanDate(d *book.Date, s string) error {
	_, err := fmt.Sscanf(s, "%d-%d-%d", &d.Year, &d.Month, &d.Day)
	return err
}

func joinAnd(parts []string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += " AND "
		}
		result += p
	}
	return result
}
