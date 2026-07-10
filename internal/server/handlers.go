package server

import (
	"net/http"
	"strings"

	"github.com/user/bunbu-shelf/internal/book"
	"github.com/user/bunbu-shelf/internal/index"
)

// ---- Dashboard ----

type dashboardData struct {
	CurrentlyReading []index.SearchResult
	RecentFinishes   []index.SearchResult
	Queue            []index.SearchResult
	ActiveTracks     []string
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	reading, _ := s.store.CurrentlyReading()
	recent, _ := s.store.RecentFinishes(5)
	queue, _ := s.store.Queue()

	s.renderer.render(w, "dashboard.html", dashboardData{
		CurrentlyReading: reading,
		RecentFinishes:   recent,
		Queue:            queue,
		ActiveTracks:     s.cfg.ActiveTracks,
	})
}

// ---- Shelf ----

type shelfData struct {
	Books   []index.SearchResult
	Status  string
	OrderBy string
	Desc    bool
}

func (s *Server) handleShelf(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	opts := index.ListOpts{
		Status:  book.Status(q.Get("status")),
		Track:   q.Get("track"),
		OrderBy: q.Get("order"),
		Desc:    q.Get("desc") == "1",
	}
	books, _ := s.store.List(opts)
	s.renderer.render(w, "shelf.html", shelfData{
		Books:   books,
		Status:  string(opts.Status),
		OrderBy: opts.OrderBy,
		Desc:    opts.Desc,
	})
}

func (s *Server) handleShelfStatus(w http.ResponseWriter, r *http.Request) {
	status := book.Status(r.PathValue("status"))
	if !book.IsValidStatus(status) {
		http.NotFound(w, r)
		return
	}
	books, _ := s.store.ListByStatus(status)
	s.renderer.render(w, "shelf.html", shelfData{
		Books:  books,
		Status: string(status),
	})
}

// ---- Book detail ----

type bookDetailData struct {
	Book          *book.Book
	BodyHTML      string
	BuyLinks      []buyLink
	ValidStatuses []book.Status
}

type buyLink struct {
	Name string
	URL  string
}

func (s *Server) handleBookDetail(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	b, err := s.store.Get(slug)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Re-parse from disk to get the freshest body content.
	if fresh, err := book.ParseFile(b.FilePath); err == nil {
		b = fresh
	}

	bodyHTML := renderMarkdown(b.Body)
	links := s.buildBuyLinks(b)

	s.renderer.render(w, "book.html", bookDetailData{
		Book:          b,
		BodyHTML:      bodyHTML,
		BuyLinks:      links,
		ValidStatuses: book.ValidStatuses,
	})
}

func (s *Server) buildBuyLinks(b *book.Book) []buyLink {
	var links []buyLink
	for _, name := range s.cfg.BuyTargets.Enabled {
		target, ok := s.cfg.BuyTargets.Targets[name]
		if !ok {
			continue
		}
		url := buildBuyURL(target.URL, b.Meta.Title, b.Meta.Author, b.Meta.ISBN)
		links = append(links, buyLink{Name: name, URL: url})
	}
	return links
}

func buildBuyURL(tmpl, title, author, isbn string) string {
	isbnOrTitle := isbn
	if isbnOrTitle == "" {
		isbnOrTitle = strings.ReplaceAll(title+" "+author, " ", "+")
	}
	r := strings.NewReplacer(
		"{isbn}", isbn,
		"{title}", strings.ReplaceAll(title, " ", "+"),
		"{author}", strings.ReplaceAll(author, " ", "+"),
		"{isbn_or_title}", isbnOrTitle,
	)
	return r.Replace(tmpl)
}

// ---- Search ----

type searchData struct {
	Query         string
	LocalResults  []index.SearchResult
	RemoteResults []remoteSearchResult
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	data := searchData{Query: q}

	if q != "" {
		data.LocalResults, _ = s.store.Search(q, 20)
		data.RemoteResults = s.searchRemote(r.Context(), q)
	}

	s.renderer.render(w, "search.html", data)
}

// ---- Themes ----

type themesData struct {
	Themes []index.ThemeCount
}

func (s *Server) handleThemes(w http.ResponseWriter, r *http.Request) {
	themes, _ := s.store.AllThemes()
	s.renderer.render(w, "themes.html", themesData{Themes: themes})
}

// ---- Log ----

type logData struct {
	Books []index.SearchResult
}

func (s *Server) handleLog(w http.ResponseWriter, r *http.Request) {
	books, _ := s.store.RecentFinishes(100)
	s.renderer.render(w, "log.html", logData{Books: books})
}

// ---- Book body partial (for SSE refresh) ----

func (s *Server) handleBookBody(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	b, err := s.store.Get(slug)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if fresh, err := book.ParseFile(b.FilePath); err == nil {
		b = fresh
	}
	bodyHTML := renderMarkdown(b.Body)
	w.Header().Set("Content-Type", "text/html")
	_, _ = w.Write([]byte(bodyHTML))
}

// ---- Settings ----

type settingsData struct {
	Config interface{}
	Saved  bool
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	saved := r.URL.Query().Get("saved") == "1"
	s.renderer.render(w, "settings.html", settingsData{
		Config: s.cfg,
		Saved:  saved,
	})
}
