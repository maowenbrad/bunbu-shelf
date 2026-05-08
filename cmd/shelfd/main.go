package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/user/bunbu-shelf/internal/config"
	"github.com/user/bunbu-shelf/internal/index"
	"github.com/user/bunbu-shelf/internal/server"
	"github.com/user/bunbu-shelf/internal/watcher"
	"github.com/user/bunbu-shelf/web"
)

func main() {
	cfgPath := flag.String("config", config.DefaultPath(), "path to config.toml")
	library := flag.String("library", "", "library directory (overrides config)")
	addr := flag.String("addr", "", "listen address (overrides config)")
	dev := flag.Bool("dev", false, "development mode: reload templates from disk")
	flag.Parse()

	cfg, err := config.LoadOrDefault(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if *library != "" {
		cfg.LibraryDir = *library
	}
	if *addr != "" {
		cfg.ServerAddr = *addr
	}
	if err := cfg.ExpandLibraryDir(); err != nil {
		log.Fatalf("expand library dir: %v", err)
	}

	// Ensure the library structure exists.
	for _, dir := range []string{
		cfg.LibraryDir,
		filepath.Join(cfg.LibraryDir, "books"),
		filepath.Join(cfg.LibraryDir, "covers"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Fatalf("create dir %s: %v", dir, err)
		}
	}

	store, err := index.Open(cfg.DBPath())
	if err != nil {
		log.Fatalf("open index: %v", err)
	}
	defer store.Close()

	// Initial index rebuild.
	log.Printf("indexing %s…", cfg.LibraryDir)
	if err := store.Reindex(cfg.LibraryDir); err != nil {
		log.Printf("reindex warning: %v", err)
	}

	// Build server.
	devDir := ""
	if *dev {
		// In dev mode, read templates from the web/ directory on disk.
		// Run shelfd from the repo root for this to work.
		devDir = "web"
	}

	srv, err := server.New(store, cfg, web.FS(), devDir)
	if err != nil {
		log.Fatalf("create server: %v", err)
	}

	// Start file watcher.
	booksDir := filepath.Join(cfg.LibraryDir, "books")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w, err := watcher.New(store, booksDir, func(slug string) {
		srv.Hub().Broadcast("book-updated", slug)
	})
	if err != nil {
		log.Printf("watcher unavailable: %v", err)
	} else {
		go func() {
			if err := w.Start(ctx); err != nil {
				log.Printf("watcher stopped: %v", err)
			}
		}()
		defer w.Close()
	}

	// Listen for OS signals for graceful shutdown.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	httpServer := &http.Server{
		Addr:    cfg.ServerAddr,
		Handler: srv,
	}

	log.Printf("shelfd listening on http://localhost%s", cfg.ServerAddr)
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("http server: %v", err)
		}
	}()

	<-sigCh
	fmt.Fprintln(os.Stderr, "\nshutting down…")
	_ = httpServer.Shutdown(ctx)
}
