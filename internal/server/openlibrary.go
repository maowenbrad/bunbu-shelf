package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/user/bunbu-shelf/internal/book"
	"github.com/user/bunbu-shelf/internal/openlibrary"
)

// olSearchResult is the template-facing view of an Open Library result.
type olSearchResult struct {
	Key     string
	Title   string
	Author  string
	Year    int
	ISBN    string
	CoverID int
}

func searchOpenLibrary(ctx context.Context, q string) []olSearchResult {
	// Determine cover dir for client (not needed for search, only for add).
	client := openlibrary.New("")
	raw, err := client.SearchByTitle(ctx, q)
	if err != nil {
		return nil
	}
	var out []olSearchResult
	for _, r := range raw {
		author := ""
		if len(r.Authors) > 0 {
			author = r.Authors[0]
		}
		out = append(out, olSearchResult{
			Key:     r.Key,
			Title:   r.Title,
			Author:  author,
			Year:    r.Year,
			ISBN:    r.ISBN,
			CoverID: r.CoverID,
		})
	}
	return out
}

func (s *Server) handleAddFromOpenLibrary(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	author := strings.TrimSpace(r.FormValue("author"))
	isbn := strings.TrimSpace(r.FormValue("isbn"))
	coverIDStr := r.FormValue("cover_id")

	if title == "" {
		http.Error(w, "title required", http.StatusBadRequest)
		return
	}

	slug := book.SlugFromTitle(title)
	booksDir := filepath.Join(s.cfg.LibraryDir, "books")
	if err := os.MkdirAll(booksDir, 0o755); err != nil {
		http.Error(w, "cannot create books dir", http.StatusInternalServerError)
		return
	}

	path := filepath.Join(booksDir, slug+".md")
	if _, err := os.Stat(path); err == nil {
		for i := 2; ; i++ {
			candidate := filepath.Join(booksDir, fmt.Sprintf("%s-%d.md", slug, i))
			if _, err := os.Stat(candidate); os.IsNotExist(err) {
				path = candidate
				slug = fmt.Sprintf("%s-%d", slug, i)
				break
			}
		}
	}

	meta := book.Frontmatter{
		Title:  title,
		Author: author,
		Status: book.StatusAntilibrary,
		ISBN:   isbn,
	}

	// Fetch cover synchronously so the redirected detail page renders it
	// on first paint. FetchCover has a bounded HTTP timeout.
	var coverID int
	fmt.Sscanf(coverIDStr, "%d", &coverID)
	if coverID > 0 {
		coverDir := filepath.Join(s.cfg.LibraryDir, "covers")
		_ = os.MkdirAll(coverDir, 0o755)
		client := openlibrary.New(coverDir)
		if coverPath, err := client.FetchCover(r.Context(), coverID, slug); err == nil && coverPath != "" {
			meta.Cover = coverPath
		}
	}

	b := &book.Book{
		Slug:     slug,
		FilePath: path,
		Meta:     meta,
	}

	if err := book.WriteFile(b); err != nil {
		http.Error(w, "write error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.store.IndexBook(b); err != nil {
		http.Error(w, "index error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/book/"+slug, http.StatusSeeOther)
}
