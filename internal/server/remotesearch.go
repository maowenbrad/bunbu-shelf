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

// remoteSearchResult is the template-facing view of one hit from a remote
// book-search source (Hardcover or Open Library). CoverID is only set for
// Open Library results; CoverURL is set by both sources.
type remoteSearchResult struct {
	Title     string
	Author    string
	Year      int
	ISBN      string
	CoverID   int
	CoverURL  string
	Publisher string
	Pages     int
}

// searchRemote queries the configured book-search source: Hardcover when an
// API key is configured, Open Library otherwise.
func (s *Server) searchRemote(ctx context.Context, q string) []remoteSearchResult {
	if s.cfg.HardcoverAPIKey != "" {
		return searchHardcover(ctx, s.cfg.HardcoverAPIKey, q)
	}
	return searchOpenLibrary(ctx, q)
}

func (s *Server) handleAddFromSearch(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	author := strings.TrimSpace(r.FormValue("author"))
	isbn := strings.TrimSpace(r.FormValue("isbn"))
	publisher := strings.TrimSpace(r.FormValue("publisher"))
	coverURL := strings.TrimSpace(r.FormValue("cover_url"))
	coverIDStr := r.FormValue("cover_id")
	yearStr := r.FormValue("year")
	pagesStr := r.FormValue("pages")

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

	var year, pages int
	fmt.Sscanf(yearStr, "%d", &year)
	fmt.Sscanf(pagesStr, "%d", &pages)

	meta := book.Frontmatter{
		Title:       title,
		Author:      author,
		Status:      book.StatusAntilibrary,
		ISBN:        isbn,
		Publisher:   publisher,
		PublishYear: year,
		Pages:       pages,
	}

	// Fetch cover synchronously so the redirected detail page renders it on
	// first paint. All fetches have a bounded HTTP timeout.
	var coverID int
	fmt.Sscanf(coverIDStr, "%d", &coverID)
	if coverURL != "" || isbn != "" || coverID > 0 {
		coverDir := filepath.Join(s.cfg.LibraryDir, "covers")
		_ = os.MkdirAll(coverDir, 0o755)
		client := openlibrary.New(coverDir)

		var coverPath string
		if coverID > 0 {
			// Open Library result: cover_url is only the small display
			// thumbnail, so ignore it and prefer the ISBN-keyed cover —
			// cover_id comes from a title search and can belong to a
			// different printing/translation than the exact edition the
			// ISBN identifies.
			if isbn != "" {
				coverPath, _ = client.FetchCoverByISBN(r.Context(), isbn, slug)
			}
			if coverPath == "" {
				coverPath, _ = client.FetchCover(r.Context(), coverID, slug)
			}
		} else {
			// Hardcover result: cover_url is the full-size image the user
			// saw in the results; fall back to Open Library's ISBN cover.
			if coverURL != "" {
				coverPath, _ = downloadCover(r.Context(), coverURL, coverDir, slug)
			}
			if coverPath == "" && isbn != "" {
				coverPath, _ = client.FetchCoverByISBN(r.Context(), isbn, slug)
			}
		}
		meta.Cover = coverPath
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
