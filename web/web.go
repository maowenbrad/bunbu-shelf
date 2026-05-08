// Package web embeds the templates and static assets into the binary.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:templates all:static
var embedded embed.FS

// FS returns the embedded filesystem containing templates/ and static/.
func FS() fs.FS {
	return embedded
}
