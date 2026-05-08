package watcher

import (
	"context"
	"log"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/user/bunbu-shelf/internal/book"
	"github.com/user/bunbu-shelf/internal/index"
)

// EventHandler is called when a book file changes. slug is the affected book.
type EventHandler func(slug string)

// Watcher monitors the library books directory and re-indexes changed files.
type Watcher struct {
	store   *index.Store
	booksDir string
	notify  EventHandler
	w       *fsnotify.Watcher
}

// New creates a Watcher that watches booksDir and calls notify on changes.
func New(store *index.Store, booksDir string, notify EventHandler) (*Watcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := w.Add(booksDir); err != nil {
		_ = w.Close()
		return nil, err
	}
	return &Watcher{store: store, booksDir: booksDir, notify: notify, w: w}, nil
}

// Start begins watching for file changes. It blocks until ctx is cancelled.
func (wt *Watcher) Start(ctx context.Context) error {
	defer wt.w.Close()

	// Debounce timer: collapse bursts of events (Neovim write = delete+create).
	debounce := make(map[string]*time.Timer)
	delay := 100 * time.Millisecond

	for {
		select {
		case event, ok := <-wt.w.Events:
			if !ok {
				return nil
			}
			if filepath.Ext(event.Name) != ".md" {
				continue
			}
			slug := book.SlugFromPath(event.Name)
			path := event.Name

			if t, exists := debounce[slug]; exists {
				t.Reset(delay)
				continue
			}
			debounce[slug] = time.AfterFunc(delay, func() {
				delete(debounce, slug)
				wt.handle(event.Op, path, slug)
			})

		case err, ok := <-wt.w.Errors:
			if !ok {
				return nil
			}
			log.Printf("watcher error: %v", err)

		case <-ctx.Done():
			return nil
		}
	}
}

func (wt *Watcher) handle(op fsnotify.Op, path, slug string) {
	if op&(fsnotify.Remove|fsnotify.Rename) != 0 {
		if err := wt.store.DeleteBook(slug); err != nil {
			log.Printf("watcher: delete %s: %v", slug, err)
		}
		if wt.notify != nil {
			wt.notify(slug)
		}
		return
	}

	b, err := book.ParseFile(path)
	if err != nil {
		log.Printf("watcher: parse %s: %v", path, err)
		return
	}
	if err := wt.store.IndexBook(b); err != nil {
		log.Printf("watcher: index %s: %v", path, err)
		return
	}
	if wt.notify != nil {
		wt.notify(slug)
	}
}

// Close stops the watcher.
func (wt *Watcher) Close() error {
	return wt.w.Close()
}
