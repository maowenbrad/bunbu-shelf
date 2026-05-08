package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config holds all runtime configuration for bunbu-shelf.
type Config struct {
	LibraryDir   string     `toml:"library_dir"`
	ActiveTracks []string   `toml:"active_tracks"`
	ServerAddr   string     `toml:"server_addr"`
	BuyTargets   BuyTargets `toml:"buy_targets"`
}

// BuyTargets groups the buy-button configuration.
type BuyTargets struct {
	Enabled []string             `toml:"enabled"`
	Targets map[string]BuyTarget `toml:"-"`
}

// BuyTarget holds the URL template for a single buy destination.
type BuyTarget struct {
	URL string `toml:"url"`
}

// BuyLink expands a BuyTarget URL template for a given book.
// Supported tokens: {isbn}, {title}, {author}, {isbn_or_title}
func BuyLink(target BuyTarget, title, author, isbn string) string {
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
	return r.Replace(target.URL)
}

// Defaults fills in zero-value fields with sensible defaults.
func (c *Config) Defaults() {
	if c.LibraryDir == "" {
		home, _ := os.UserHomeDir()
		c.LibraryDir = filepath.Join(home, "commonplace", "library")
	}
	if len(c.ActiveTracks) == 0 {
		c.ActiveTracks = []string{"fiction", "nonfiction"}
	}
	if c.ServerAddr == "" {
		c.ServerAddr = ":8127"
	}
	if c.BuyTargets.Targets == nil {
		c.BuyTargets.Targets = defaultBuyTargets()
	}
	if len(c.BuyTargets.Enabled) == 0 {
		c.BuyTargets.Enabled = []string{"bookshop", "abebooks", "amazon"}
	}
}

// ExpandLibraryDir resolves ~ in LibraryDir.
func (c *Config) ExpandLibraryDir() error {
	if strings.HasPrefix(c.LibraryDir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		c.LibraryDir = filepath.Join(home, c.LibraryDir[2:])
	}
	return nil
}

// DBPath returns the path to the SQLite database file.
func (c *Config) DBPath() string {
	return filepath.Join(c.LibraryDir, "index.db")
}

// Load reads a TOML config file from path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	return parse(data)
}

// LoadOrDefault loads a config file if it exists, otherwise returns defaults.
func LoadOrDefault(path string) (*Config, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		cfg := &Config{}
		cfg.Defaults()
		return cfg, nil
	}
	return Load(path)
}

func parse(data []byte) (*Config, error) {
	// We need a custom decoder to handle [buy_targets.<name>] sub-sections.
	// Decode into a raw map first, then extract known fields.
	var raw map[string]interface{}
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	cfg := &Config{}
	// Re-decode into the Config struct (BuyTargets.Targets handled separately).
	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Extract buy target sub-sections.
	if bt, ok := raw["buy_targets"].(map[string]interface{}); ok {
		cfg.BuyTargets.Targets = make(map[string]BuyTarget)
		for k, v := range bt {
			if k == "enabled" {
				continue
			}
			if m, ok := v.(map[string]interface{}); ok {
				urlVal, _ := m["url"].(string)
				cfg.BuyTargets.Targets[k] = BuyTarget{URL: urlVal}
			}
		}
	}

	cfg.Defaults()
	return cfg, nil
}

// Save writes cfg to path in TOML format.
func Save(cfg *Config, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(cfg)
}

// DefaultPath returns the default config file location.
func DefaultPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "bunbu-shelf", "config.toml")
}

func defaultBuyTargets() map[string]BuyTarget {
	return map[string]BuyTarget{
		"bookshop": {URL: "https://bookshop.org/search?keywords={isbn_or_title}"},
		"abebooks": {URL: "https://www.abebooks.com/servlet/SearchResults?isbn={isbn}&sts=t&kn={title}+{author}"},
		"amazon":   {URL: "https://www.amazon.com/s?k={isbn_or_title}"},
	}
}
