package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/user/bunbu-shelf/internal/book"
)

func (s *Server) handleCreateBook(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	meta := parseFrontmatterForm(r)
	errs := book.ValidateMeta(meta)
	if len(errs) > 0 {
		http.Error(w, "validation error: "+errs[0].Error(), http.StatusUnprocessableEntity)
		return
	}

	slug := book.SlugFromTitle(meta.Title)
	booksDir := filepath.Join(s.cfg.LibraryDir, "books")
	if err := os.MkdirAll(booksDir, 0o755); err != nil {
		http.Error(w, "cannot create books dir", http.StatusInternalServerError)
		return
	}

	path := filepath.Join(booksDir, slug+".md")
	// If slug conflicts, append a counter.
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

func (s *Server) handleUpdateBook(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	b, err := s.store.Get(slug)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	meta := parseFrontmatterForm(r)
	// Preserve the existing cover unless the form submits a replacement URL,
	// so saving other fields (e.g. changing status) doesn't wipe it.
	meta.Cover = b.Meta.Cover
	if coverURL := strings.TrimSpace(r.FormValue("cover_url")); coverURL != "" {
		coverDir := filepath.Join(s.cfg.LibraryDir, "covers")
		if err := os.MkdirAll(coverDir, 0o755); err != nil {
			http.Error(w, "cannot create covers dir", http.StatusInternalServerError)
			return
		}
		coverPath, err := downloadCover(r.Context(), coverURL, coverDir, slug)
		if err != nil {
			http.Error(w, "cover fetch error: "+err.Error(), http.StatusBadRequest)
			return
		}
		meta.Cover = coverPath
	}

	errs := book.ValidateMeta(meta)
	if len(errs) > 0 {
		http.Error(w, "validation error: "+errs[0].Error(), http.StatusUnprocessableEntity)
		return
	}

	if err := book.UpdateMeta(b.FilePath, meta); err != nil {
		http.Error(w, "write error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	b.Meta = meta
	if err := s.store.IndexBook(b); err != nil {
		http.Error(w, "index error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/book/"+slug, http.StatusSeeOther)
}

func (s *Server) handleDeleteBook(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	b, err := s.store.Get(slug)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Delete the markdown file.
	if err := os.Remove(b.FilePath); err != nil && !os.IsNotExist(err) {
		http.Error(w, "delete error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Delete the cover if it's in the library covers dir.
	if b.Meta.Cover != "" {
		coverPath := filepath.Join(s.cfg.LibraryDir, b.Meta.Cover)
		_ = os.Remove(coverPath)
	}

	if err := s.store.DeleteBook(slug); err != nil {
		http.Error(w, "index error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/shelf", http.StatusSeeOther)
}

func (s *Server) handleUpdateStatus(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	b, err := s.store.Get(slug)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	newStatus := book.Status(r.FormValue("status"))
	if !book.IsValidStatus(newStatus) {
		http.Error(w, "invalid status", http.StatusBadRequest)
		return
	}

	b.Meta.Status = newStatus
	if err := book.UpdateMeta(b.FilePath, b.Meta); err != nil {
		http.Error(w, "write error", http.StatusInternalServerError)
		return
	}
	if err := s.store.IndexBook(b); err != nil {
		http.Error(w, "index error", http.StatusInternalServerError)
		return
	}

	s.hub.Broadcast("book-updated", slug)

	// htmx request: return a minimal status badge fragment.
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w,
			`<span class="inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-sm font-semibold bg-mist %s"><span class="inline-block w-1.5 h-1.5 rounded-full %s"></span>%s</span>`,
			statusColorClass(string(newStatus)), statusDotClass(string(newStatus)), statusLabel(string(newStatus)))
		return
	}
	http.Redirect(w, r, "/book/"+slug, http.StatusSeeOther)
}

func (s *Server) handleSaveSettings(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	// Apply changes to in-memory config (LibraryDir change requires restart).
	s.cfg.ServerAddr = r.FormValue("server_addr")
	tracks := r.FormValue("active_tracks")
	s.cfg.ActiveTracks = splitTrimmed(tracks, ",")

	http.Redirect(w, r, "/settings?saved=1", http.StatusSeeOther)
}

// downloadCover fetches an image from a user-supplied URL and saves it to
// coverDir/<slug>.jpg, so the cover displayed for a book can be corrected to
// match the caller's actual physical copy. Bounded by a timeout and a size
// cap since the URL is arbitrary.
func downloadCover(ctx context.Context, rawURL, coverDir, slug string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("invalid cover URL: %w", err)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch cover: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch cover: status %d", resp.StatusCode)
	}

	const maxCoverBytes = 10 << 20 // 10MB
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxCoverBytes))
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", fmt.Errorf("empty response")
	}

	filename := slug + ".jpg"
	if err := os.WriteFile(filepath.Join(coverDir, filename), data, 0o644); err != nil {
		return "", err
	}
	return "covers/" + filename, nil
}

// parseFrontmatterForm extracts a Frontmatter from a form POST.
func parseFrontmatterForm(r *http.Request) book.Frontmatter {
	meta := book.Frontmatter{
		Title:       strings.TrimSpace(r.FormValue("title")),
		Author:      strings.TrimSpace(r.FormValue("author")),
		Status:      book.Status(r.FormValue("status")),
		Track:       strings.TrimSpace(r.FormValue("track")),
		ISBN:        strings.TrimSpace(r.FormValue("isbn")),
		Cover:       strings.TrimSpace(r.FormValue("cover")),
		Source:      strings.TrimSpace(r.FormValue("source")),
		Publisher:   strings.TrimSpace(r.FormValue("publisher")),
		Description: strings.TrimSpace(r.FormValue("description")),
	}

	if v := r.FormValue("pages"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			meta.Pages = n
		}
	}
	if v := r.FormValue("publish_year"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			meta.PublishYear = n
		}
	}
	if v := r.FormValue("copies"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			meta.Copies = n
		}
	}

	if themesRaw := r.FormValue("themes"); themesRaw != "" {
		meta.Themes = splitTrimmed(themesRaw, ",")
	}

	if s := r.FormValue("started"); s != "" {
		d := &book.Date{}
		if _, err := fmt.Sscanf(s, "%d-%d-%d", &d.Year, &d.Month, &d.Day); err == nil {
			meta.Started = d
		}
	}
	if s := r.FormValue("finished"); s != "" {
		d := &book.Date{}
		if _, err := fmt.Sscanf(s, "%d-%d-%d", &d.Year, &d.Month, &d.Day); err == nil {
			meta.Finished = d
		}
	}
	if s := r.FormValue("acquired"); s != "" {
		d := &book.Date{}
		if _, err := fmt.Sscanf(s, "%d-%d-%d", &d.Year, &d.Month, &d.Day); err == nil {
			meta.Acquired = d
		}
	}

	if rStr := r.FormValue("rating"); rStr != "" {
		if rv, err := strconv.Atoi(rStr); err == nil {
			meta.Rating = &rv
		}
	}

	return meta
}

func splitTrimmed(s, sep string) []string {
	parts := strings.Split(s, sep)
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
