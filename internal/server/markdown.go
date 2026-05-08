package server

import (
	"bytes"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
)

var md = goldmark.New(
	goldmark.WithExtensions(
		extension.Table,
		extension.Strikethrough,
		extension.Typographer,
	),
	goldmark.WithRendererOptions(
		html.WithUnsafe(), // trusted content — local files only
	),
)

// renderMarkdown converts markdown to an HTML string.
// Returns empty string on error.
func renderMarkdown(src string) string {
	var buf bytes.Buffer
	if err := md.Convert([]byte(src), &buf); err != nil {
		return ""
	}
	return buf.String()
}
