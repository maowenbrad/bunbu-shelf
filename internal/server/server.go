package server

import (
	"io/fs"
	"net/http"

	"github.com/user/bunbu-shelf/internal/config"
	"github.com/user/bunbu-shelf/internal/index"
)

// Server handles HTTP requests for the bunbu-shelf web UI.
type Server struct {
	store    *index.Store
	cfg      *config.Config
	renderer *renderer
	hub      *sseHub
	mux      *http.ServeMux
}

// New creates a Server. If devDir is non-empty, templates are loaded from
// that directory on each request (development mode). Otherwise they are
// loaded from fsys (embedded in the binary).
// New creates a Server. If devDir is non-empty, templates are loaded from disk
// (devDir should be the path to the web/ directory). Otherwise they are read
// from fsys (the embedded filesystem).
func New(store *index.Store, cfg *config.Config, fsys fs.FS, devDir string) (*Server, error) {
	var r *renderer
	if devDir != "" {
		r = newDevRenderer(devDir)
	} else {
		r = newRenderer(fsys)
	}

	s := &Server{
		store:    store,
		cfg:      cfg,
		renderer: r,
		hub:      newSSEHub(),
		mux:      http.NewServeMux(),
	}
	s.registerRoutes(fsys)
	return s, nil
}

func (s *Server) registerRoutes(fsys fs.FS) {
	// Static assets — serve from static/ sub-tree within the embedded FS.
	staticFS, _ := fs.Sub(fsys, "static")
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(staticFS)))

	// Read-only views.
	s.mux.HandleFunc("GET /", s.handleDashboard)
	s.mux.HandleFunc("GET /shelf", s.handleShelf)
	s.mux.HandleFunc("GET /shelf/{status}", s.handleShelfStatus)
	s.mux.HandleFunc("GET /book/{slug}", s.handleBookDetail)
	s.mux.HandleFunc("GET /search", s.handleSearch)
	s.mux.HandleFunc("GET /themes", s.handleThemes)
	s.mux.HandleFunc("GET /log", s.handleLog)
	s.mux.HandleFunc("GET /settings", s.handleSettings)
	s.mux.HandleFunc("GET /events", s.handleSSE)

	// CRUD endpoints.
	s.mux.HandleFunc("POST /book", s.handleCreateBook)
	s.mux.HandleFunc("POST /book/{slug}", s.handleUpdateBook)
	s.mux.HandleFunc("DELETE /book/{slug}", s.handleDeleteBook)
	s.mux.HandleFunc("POST /book/{slug}/status", s.handleUpdateStatus)
	s.mux.HandleFunc("POST /settings", s.handleSaveSettings)
	s.mux.HandleFunc("POST /search/add", s.handleAddFromOpenLibrary)
	s.mux.HandleFunc("GET /book/{slug}/body", s.handleBookBody)
}

// Hub returns the SSE hub for broadcasting events from outside the server.
func (s *Server) Hub() *sseHub {
	return s.hub
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	handler := loggingMiddleware(recoveryMiddleware(s.mux))
	handler.ServeHTTP(w, r)
}
