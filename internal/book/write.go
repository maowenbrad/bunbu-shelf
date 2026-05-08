package book

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// WriteFile serializes a Book back to its .md file atomically.
// The body is preserved verbatim; only the frontmatter is regenerated.
func WriteFile(b *Book) error {
	return writeToPath(b.FilePath, b.Meta, b.Body)
}

// UpdateMeta re-reads the book at path, replaces the frontmatter with meta,
// and writes the result back atomically. The markdown body is unchanged.
func UpdateMeta(path string, meta Frontmatter) error {
	b, err := ParseFile(path)
	if err != nil {
		return err
	}
	return writeToPath(path, meta, b.Body)
}

func writeToPath(path string, meta Frontmatter, body string) error {
	out, err := marshal(meta, body)
	if err != nil {
		return err
	}

	// Atomic write: write to a temp file then rename.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return fmt.Errorf("write temp file %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename to %s: %w", path, err)
	}
	return nil
}

func marshal(meta Frontmatter, body string) ([]byte, error) {
	yamlBytes, err := yaml.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("marshal frontmatter: %w", err)
	}

	var out []byte
	out = append(out, "---\n"...)
	out = append(out, yamlBytes...)
	out = append(out, "---\n"...)
	if body != "" {
		out = append(out, body...)
	}
	return out, nil
}
