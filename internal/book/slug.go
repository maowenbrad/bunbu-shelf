package book

import (
	"path/filepath"
	"strings"
	"unicode"
)

// SlugFromPath extracts the slug from a file path by stripping the directory
// and the ".md" extension.
func SlugFromPath(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// SlugFromTitle generates a URL-safe slug from a book title.
// It lowercases, replaces non-alphanumeric runs with a single hyphen,
// and trims leading/trailing hyphens.
func SlugFromTitle(title string) string {
	var b strings.Builder
	inSep := false
	for _, r := range strings.ToLower(title) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			inSep = false
		} else {
			if !inSep && b.Len() > 0 {
				b.WriteByte('-')
				inSep = true
			}
		}
	}
	s := b.String()
	return strings.TrimRight(s, "-")
}
