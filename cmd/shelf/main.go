package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/user/bunbu-shelf/internal/book"
	"github.com/user/bunbu-shelf/internal/config"
	"github.com/user/bunbu-shelf/internal/index"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "shelf:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return nil
	}
	switch args[0] {
	case "reindex":
		return cmdReindex(args[1:])
	case "parse":
		return cmdParse(args[1:])
	case "search":
		return cmdSearch(args[1:])
	case "list":
		return cmdList(args[1:])
	default:
		usage()
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `shelf — bunbu-shelf CLI

Commands:
  reindex               rebuild SQLite index from markdown files
  parse <path>          parse a book file and print as JSON
  search <query>        full-text search the index
  list [--status <s>]   list books, optionally filtered by status`)
}

func loadConfig() (*config.Config, error) {
	path := config.DefaultPath()
	return config.LoadOrDefault(path)
}

func cmdReindex(args []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if err := cfg.ExpandLibraryDir(); err != nil {
		return err
	}

	store, err := index.Open(cfg.DBPath())
	if err != nil {
		return fmt.Errorf("open index: %w", err)
	}
	defer store.Close()

	fmt.Fprintf(os.Stderr, "reindexing %s…\n", cfg.LibraryDir)
	if err := store.Reindex(cfg.LibraryDir); err != nil {
		return fmt.Errorf("reindex: %w", err)
	}
	fmt.Fprintln(os.Stderr, "done.")
	return nil
}

func cmdParse(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: shelf parse <path>")
	}
	b, err := book.ParseFile(args[0])
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(b)
}

func cmdSearch(args []string) error {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	limit := fs.Int("n", 20, "max results")
	if err := fs.Parse(args); err != nil {
		return err
	}
	query := fs.Arg(0)
	if query == "" {
		return fmt.Errorf("usage: shelf search <query>")
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if err := cfg.ExpandLibraryDir(); err != nil {
		return err
	}

	store, err := index.Open(cfg.DBPath())
	if err != nil {
		return fmt.Errorf("open index: %w", err)
	}
	defer store.Close()

	results, err := store.Search(query, *limit)
	if err != nil {
		return err
	}
	if len(results) == 0 {
		fmt.Println("no results")
		return nil
	}
	for _, r := range results {
		fmt.Printf("[%s] %s — %s\n", r.Status, r.Title, r.Author)
		if r.Snippet != "" {
			fmt.Printf("    %s\n", r.Snippet)
		}
	}
	return nil
}

func cmdList(args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	statusStr := fs.String("status", "", "filter by status")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if err := cfg.ExpandLibraryDir(); err != nil {
		return err
	}

	store, err := index.Open(cfg.DBPath())
	if err != nil {
		return fmt.Errorf("open index: %w", err)
	}
	defer store.Close()

	opts := index.ListOpts{Status: book.Status(*statusStr)}
	results, err := store.List(opts)
	if err != nil {
		return err
	}
	if len(results) == 0 {
		fmt.Println("no books")
		return nil
	}
	for _, r := range results {
		fmt.Printf("[%s] %s — %s\n", r.Status, r.Title, r.Author)
	}
	return nil
}
