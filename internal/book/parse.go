package book

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

var delimiter = []byte("---")

// ParseFile reads a markdown file and returns a Book.
func ParseFile(path string) (*Book, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	slug := SlugFromPath(path)
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	return ParseBytes(slug, abs, data, info.ModTime())
}

// ParseBytes is the testable core: operates on raw bytes rather than a file path.
func ParseBytes(slug, path string, data []byte, modTime time.Time) (*Book, error) {
	// Expect the file to start with "---\n"
	if !bytes.HasPrefix(data, []byte("---\n")) && !bytes.HasPrefix(data, []byte("---\r\n")) {
		return nil, fmt.Errorf("%s: missing YAML frontmatter (file must start with ---)", path)
	}

	// Find the closing delimiter after the opening one.
	// Search starting after the first "---\n".
	rest := data[4:]
	closeIdx := bytes.Index(rest, []byte("\n---"))
	if closeIdx < 0 {
		return nil, fmt.Errorf("%s: unclosed YAML frontmatter (no closing ---)", path)
	}

	yamlBlock := rest[:closeIdx]
	body := ""
	afterClose := rest[closeIdx+4:] // skip "\n---"
	if len(afterClose) > 0 {
		// skip the newline after the closing ---
		if afterClose[0] == '\r' && len(afterClose) > 1 && afterClose[1] == '\n' {
			afterClose = afterClose[2:]
		} else if afterClose[0] == '\n' {
			afterClose = afterClose[1:]
		}
		body = string(afterClose)
	}

	var meta Frontmatter
	dec := yaml.NewDecoder(bytes.NewReader(yamlBlock))
	dec.KnownFields(false)
	if err := dec.Decode(&meta); err != nil {
		return nil, fmt.Errorf("%s: YAML parse error: %w", path, err)
	}

	_ = delimiter // suppress unused import

	return &Book{
		Slug:     slug,
		FilePath: path,
		Meta:     meta,
		Body:     body,
		ModTime:  modTime,
	}, nil
}
