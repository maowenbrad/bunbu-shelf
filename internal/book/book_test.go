package book_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/user/bunbu-shelf/internal/book"
)

const (
	fixtureBloodMeridian = "../../testdata/books/blood-meridian.md"
	fixtureMinimal       = "../../testdata/books/minimal.md"
)

// ---- Parse tests ----

func TestParseRoundTrip(t *testing.T) {
	b, err := book.ParseFile(fixtureBloodMeridian)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	// Write to a temp file.
	tmp := filepath.Join(t.TempDir(), "blood-meridian.md")
	b.FilePath = tmp
	if err := book.WriteFile(b); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Re-parse the written file.
	b2, err := book.ParseFile(tmp)
	if err != nil {
		t.Fatalf("ParseFile (re-read): %v", err)
	}

	if b2.Meta.Title != b.Meta.Title {
		t.Errorf("title: got %q, want %q", b2.Meta.Title, b.Meta.Title)
	}
	if b2.Meta.Author != b.Meta.Author {
		t.Errorf("author: got %q, want %q", b2.Meta.Author, b.Meta.Author)
	}
	if b2.Meta.Status != b.Meta.Status {
		t.Errorf("status: got %q, want %q", b2.Meta.Status, b.Meta.Status)
	}
	if b2.Meta.ISBN != b.Meta.ISBN {
		t.Errorf("isbn: got %q, want %q", b2.Meta.ISBN, b.Meta.ISBN)
	}
	if len(b2.Meta.Themes) != len(b.Meta.Themes) {
		t.Errorf("themes len: got %d, want %d", len(b2.Meta.Themes), len(b.Meta.Themes))
	}
	if b2.Body != b.Body {
		t.Errorf("body mismatch\ngot:  %q\nwant: %q", b2.Body, b.Body)
	}
}

func TestParseMinimal(t *testing.T) {
	b, err := book.ParseFile(fixtureMinimal)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if b.Meta.Title != "Minimal Book" {
		t.Errorf("title: got %q", b.Meta.Title)
	}
	if b.Meta.Status != book.StatusAntilibrary {
		t.Errorf("status: got %q", b.Meta.Status)
	}
}

func TestParseNilOptionals(t *testing.T) {
	b, err := book.ParseFile(fixtureMinimal)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if b.Meta.Rating != nil {
		t.Errorf("rating should be nil, got %v", *b.Meta.Rating)
	}
	if b.Meta.Started != nil {
		t.Errorf("started should be nil")
	}
	if b.Meta.Finished != nil {
		t.Errorf("finished should be nil")
	}
}

func TestParseInvalidYAML(t *testing.T) {
	data := []byte("---\n: invalid: yaml: :\n---\n")
	_, err := book.ParseBytes("bad", "/bad.md", data, time.Now())
	if err == nil {
		t.Error("expected error for invalid YAML, got nil")
	}
}

func TestParseMissingFrontmatter(t *testing.T) {
	data := []byte("# Just a markdown file\n\nNo frontmatter.\n")
	_, err := book.ParseBytes("no-fm", "/no-fm.md", data, time.Now())
	if err == nil {
		t.Error("expected error for missing frontmatter, got nil")
	}
}

// ---- Validation tests ----

func TestValidateStatusEnum(t *testing.T) {
	m := book.Frontmatter{Title: "X", Author: "Y", Status: "nonsense"}
	errs := book.ValidateMeta(m)
	if len(errs) == 0 {
		t.Error("expected validation error for unknown status")
	}
}

func TestValidateRatingRange(t *testing.T) {
	for _, r := range []int{0, 6, -1, 100} {
		r := r
		m := book.Frontmatter{Title: "X", Author: "Y", Status: book.StatusQueued, Rating: &r}
		errs := book.ValidateMeta(m)
		found := false
		for _, e := range errs {
			if e.Field == "rating" {
				found = true
			}
		}
		if !found {
			t.Errorf("expected rating error for value %d", r)
		}
	}
	// Valid ratings should not error
	for _, r := range []int{1, 2, 3, 4, 5} {
		r := r
		m := book.Frontmatter{Title: "X", Author: "Y", Status: book.StatusQueued, Rating: &r}
		errs := book.ValidateMeta(m)
		for _, e := range errs {
			if e.Field == "rating" {
				t.Errorf("unexpected rating error for value %d: %s", r, e.Message)
			}
		}
	}
}

func TestValidateDatesOrdered(t *testing.T) {
	started := book.Date{Year: 2026, Month: 5, Day: 10}
	finished := book.Date{Year: 2026, Month: 5, Day: 1} // before started
	m := book.Frontmatter{
		Title:    "X",
		Author:   "Y",
		Status:   book.StatusFinished,
		Started:  &started,
		Finished: &finished,
	}
	errs := book.ValidateMeta(m)
	found := false
	for _, e := range errs {
		if e.Field == "finished" {
			found = true
		}
	}
	if !found {
		t.Error("expected finished-before-started validation error")
	}
}

func TestValidateMissingTitle(t *testing.T) {
	m := book.Frontmatter{Author: "Y", Status: book.StatusQueued}
	errs := book.ValidateMeta(m)
	found := false
	for _, e := range errs {
		if e.Field == "title" {
			found = true
		}
	}
	if !found {
		t.Error("expected title validation error")
	}
}

// ---- Slug tests ----

func TestSlugFromPath(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"/home/user/library/books/blood-meridian.md", "blood-meridian"},
		{"books/hero-with-thousand-faces.md", "hero-with-thousand-faces"},
		{"simple.md", "simple"},
	}
	for _, c := range cases {
		got := book.SlugFromPath(c.path)
		if got != c.want {
			t.Errorf("SlugFromPath(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

func TestSlugFromTitle(t *testing.T) {
	cases := []struct {
		title string
		want  string
	}{
		{"Blood Meridian", "blood-meridian"},
		{"Blood Meridian: Or the Evening Redness in the West", "blood-meridian-or-the-evening-redness-in-the-west"},
		{"1984", "1984"},
		{"The Hero with a Thousand Faces", "the-hero-with-a-thousand-faces"},
		{"  Leading spaces  ", "leading-spaces"},
	}
	for _, c := range cases {
		got := book.SlugFromTitle(c.title)
		if got != c.want {
			t.Errorf("SlugFromTitle(%q) = %q, want %q", c.title, got, c.want)
		}
	}
}

// ---- Atomic write test ----

func TestAtomicWrite(t *testing.T) {
	// Write a book, verify no .tmp files are left behind.
	b, err := book.ParseFile(fixtureBloodMeridian)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	tmp := filepath.Join(t.TempDir(), "blood-meridian.md")
	b.FilePath = tmp
	if err := book.WriteFile(b); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// Verify the .tmp file was cleaned up.
	if _, err := os.Stat(tmp + ".tmp"); !os.IsNotExist(err) {
		t.Error("expected .tmp file to be cleaned up after WriteFile")
	}
	// Verify the output file exists.
	if _, err := os.Stat(tmp); err != nil {
		t.Errorf("output file missing: %v", err)
	}
}

// ---- Date marshaling test ----

func TestDateMarshalRoundTrip(t *testing.T) {
	d := book.Date{Year: 2026, Month: 5, Day: 1}
	if got := d.String(); got != "2026-05-01" {
		t.Errorf("Date.String() = %q, want %q", got, "2026-05-01")
	}
}
