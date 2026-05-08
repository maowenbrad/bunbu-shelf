package index_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/user/bunbu-shelf/internal/book"
	"github.com/user/bunbu-shelf/internal/index"
)

func openTestStore(t *testing.T) *index.Store {
	t.Helper()
	tmp := filepath.Join(t.TempDir(), "test.db")
	s, err := index.Open(tmp)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func makeBook(slug, title, author string, status book.Status) *book.Book {
	return &book.Book{
		Slug:     slug,
		FilePath: "/tmp/" + slug + ".md",
		Meta: book.Frontmatter{
			Title:  title,
			Author: author,
			Status: status,
			Themes: []string{"test"},
		},
		Body:    "## Quotes\n\n> A test quote about " + title,
		ModTime: time.Now(),
	}
}

func TestReindex(t *testing.T) {
	s := openTestStore(t)

	// Set up a temp library dir with two book files.
	libDir := t.TempDir()
	booksDir := filepath.Join(libDir, "books")
	if err := os.MkdirAll(booksDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Copy test fixtures.
	for _, name := range []string{"blood-meridian.md", "minimal.md"} {
		src := filepath.Join("../../testdata/books", name)
		data, err := os.ReadFile(src)
		if err != nil {
			t.Fatalf("read fixture %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(booksDir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := s.Reindex(libDir); err != nil {
		t.Fatalf("Reindex: %v", err)
	}

	results, err := s.List(index.ListOpts{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 books, got %d", len(results))
	}
}

func TestReindexIncremental(t *testing.T) {
	s := openTestStore(t)

	libDir := t.TempDir()
	booksDir := filepath.Join(libDir, "books")
	if err := os.MkdirAll(booksDir, 0o755); err != nil {
		t.Fatal(err)
	}

	src := "../../testdata/books/blood-meridian.md"
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	bookPath := filepath.Join(booksDir, "blood-meridian.md")
	if err := os.WriteFile(bookPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := s.Reindex(libDir); err != nil {
		t.Fatalf("first Reindex: %v", err)
	}

	// Get the first indexed_at timestamp.
	var firstIndexedAt string
	row := s.DB().QueryRow(`SELECT indexed_at FROM books WHERE slug = 'blood-meridian'`)
	if err := row.Scan(&firstIndexedAt); err != nil {
		t.Fatalf("scan indexed_at: %v", err)
	}

	// Second reindex without file change — indexed_at should not update.
	if err := s.Reindex(libDir); err != nil {
		t.Fatalf("second Reindex: %v", err)
	}

	var secondIndexedAt string
	row = s.DB().QueryRow(`SELECT indexed_at FROM books WHERE slug = 'blood-meridian'`)
	if err := row.Scan(&secondIndexedAt); err != nil {
		t.Fatalf("scan indexed_at: %v", err)
	}

	if firstIndexedAt != secondIndexedAt {
		t.Errorf("file unchanged but re-indexed: %s → %s", firstIndexedAt, secondIndexedAt)
	}
}

func TestSearch(t *testing.T) {
	s := openTestStore(t)

	b := makeBook("blood-meridian", "Blood Meridian", "Cormac McCarthy", book.StatusReading)
	b.Body = "## Reflections\n\nA book about violence and american mythology on the frontier."
	if err := s.IndexBook(b); err != nil {
		t.Fatalf("IndexBook: %v", err)
	}

	results, err := s.Search("violence", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected at least one result for 'violence'")
	}
	if results[0].Slug != "blood-meridian" {
		t.Errorf("expected blood-meridian, got %s", results[0].Slug)
	}
}

func TestSearchNoResults(t *testing.T) {
	s := openTestStore(t)
	results, err := s.Search("xyzzy", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestDeleteBook(t *testing.T) {
	s := openTestStore(t)

	b := makeBook("test-book", "Test Book", "Test Author", book.StatusQueued)
	if err := s.IndexBook(b); err != nil {
		t.Fatalf("IndexBook: %v", err)
	}

	if err := s.DeleteBook("test-book"); err != nil {
		t.Fatalf("DeleteBook: %v", err)
	}

	results, err := s.List(index.ListOpts{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, r := range results {
		if r.Slug == "test-book" {
			t.Error("deleted book still appears in List results")
		}
	}
}

func TestListByStatus(t *testing.T) {
	s := openTestStore(t)

	if err := s.IndexBook(makeBook("a", "Book A", "Author", book.StatusReading)); err != nil {
		t.Fatal(err)
	}
	if err := s.IndexBook(makeBook("b", "Book B", "Author", book.StatusFinished)); err != nil {
		t.Fatal(err)
	}
	if err := s.IndexBook(makeBook("c", "Book C", "Author", book.StatusQueued)); err != nil {
		t.Fatal(err)
	}

	reading, err := s.ListByStatus(book.StatusReading)
	if err != nil {
		t.Fatalf("ListByStatus: %v", err)
	}
	if len(reading) != 1 || reading[0].Slug != "a" {
		t.Errorf("expected 1 reading book 'a', got %v", reading)
	}
}

func TestAllThemes(t *testing.T) {
	s := openTestStore(t)

	b1 := makeBook("a", "Book A", "Author", book.StatusReading)
	b1.Meta.Themes = []string{"fiction", "history"}
	b2 := makeBook("b", "Book B", "Author", book.StatusReading)
	b2.Meta.Themes = []string{"fiction", "philosophy"}

	if err := s.IndexBook(b1); err != nil {
		t.Fatal(err)
	}
	if err := s.IndexBook(b2); err != nil {
		t.Fatal(err)
	}

	themes, err := s.AllThemes()
	if err != nil {
		t.Fatalf("AllThemes: %v", err)
	}

	// "fiction" should appear twice
	found := false
	for _, tc := range themes {
		if tc.Theme == "fiction" && tc.Count == 2 {
			found = true
		}
	}
	if !found {
		t.Errorf("expected fiction count=2, got %v", themes)
	}
}

func TestGet(t *testing.T) {
	s := openTestStore(t)

	b := makeBook("test-get", "Get Me", "Author", book.StatusFinished)
	if err := s.IndexBook(b); err != nil {
		t.Fatal(err)
	}

	got, err := s.Get("test-get")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Meta.Title != "Get Me" {
		t.Errorf("title: got %q", got.Meta.Title)
	}
}
