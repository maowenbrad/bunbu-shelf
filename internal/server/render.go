package server

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"
)

// renderer holds parsed templates and renders them into ResponseWriters.
type renderer struct {
	fsys    fs.FS
	baseDir string // non-empty in dev mode; path to web/ directory
}

func newRenderer(fsys fs.FS) *renderer {
	return &renderer{fsys: fsys}
}

func newDevRenderer(webDir string) *renderer {
	return &renderer{baseDir: webDir}
}

// render executes the named page template with data.
// Each render call builds a fresh template set: base + partials + the page.
// This avoids {{define "content"}} namespace collisions.
func (r *renderer) render(w http.ResponseWriter, name string, data interface{}) {
	t, err := r.loadTemplate(name)
	if err != nil {
		http.Error(w, "template load error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, name, data); err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}

// loadTemplate builds a template set for the named page by combining:
// base.html + all partials + the named page template.
func (r *renderer) loadTemplate(name string) (*template.Template, error) {
	funcs := templateFuncs()

	if r.baseDir != "" {
		// Dev mode: read from disk.
		baseFile := filepath.Join(r.baseDir, "templates", "base.html")
		pageFile := filepath.Join(r.baseDir, "templates", name)
		partialsGlob := filepath.Join(r.baseDir, "templates", "partials", "*.html")

		t := template.New("").Funcs(funcs)
		t, err := t.ParseFiles(baseFile)
		if err != nil {
			return nil, fmt.Errorf("parse base: %w", err)
		}
		matches, _ := filepath.Glob(partialsGlob)
		if len(matches) > 0 {
			if t, err = t.ParseFiles(matches...); err != nil {
				return nil, fmt.Errorf("parse partials: %w", err)
			}
		}
		if t, err = t.ParseFiles(pageFile); err != nil {
			return nil, fmt.Errorf("parse page %s: %w", name, err)
		}
		return t, nil
	}

	// Embedded mode: read from fs.FS.
	baseData, err := fs.ReadFile(r.fsys, "templates/base.html")
	if err != nil {
		return nil, fmt.Errorf("read base.html: %w", err)
	}
	pageData, err := fs.ReadFile(r.fsys, "templates/"+name)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}

	t := template.New("").Funcs(funcs)
	if t, err = t.New("base.html").Parse(string(baseData)); err != nil {
		return nil, fmt.Errorf("parse base: %w", err)
	}

	// Parse partials.
	partialEntries, _ := fs.ReadDir(r.fsys, "templates/partials")
	for _, e := range partialEntries {
		if !strings.HasSuffix(e.Name(), ".html") {
			continue
		}
		data, err := fs.ReadFile(r.fsys, "templates/partials/"+e.Name())
		if err != nil {
			continue
		}
		if _, err = t.New(e.Name()).Parse(string(data)); err != nil {
			return nil, fmt.Errorf("parse partial %s: %w", e.Name(), err)
		}
	}

	// Parse the page last so its {{define "content"}} wins.
	if _, err = t.New(name).Parse(string(pageData)); err != nil {
		return nil, fmt.Errorf("parse %s: %w", name, err)
	}

	return t, nil
}

func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"add": func(a, b int) int { return a + b },
		"join": func(items []string, sep string) string {
			return strings.Join(items, sep)
		},
		"html": func(s string) template.HTML { return template.HTML(s) },
		"stars": func(rating *int) []string {
			if rating == nil {
				return nil
			}
			r := *rating
			out := make([]string, 5)
			for i := range out {
				if i < r {
					out[i] = "★"
				} else {
					out[i] = "☆"
				}
			}
			return out
		},
		"statusLabel": statusLabel,
		"statusColor": statusColorClass,
		"statusDot":   statusDotClass,
	}
}

func statusLabel(s string) string {
	labels := map[string]string{
		"antilibrary": "Antilibrary",
		"queued":      "Queued",
		"reading":     "Reading",
		"finished":    "Finished",
		"abandoned":   "Abandoned",
		"reference":   "Reference",
	}
	if l, ok := labels[s]; ok {
		return l
	}
	return s
}

func statusColorClass(s string) string {
	colors := map[string]string{
		"antilibrary": "text-fog-light",
		"queued":      "text-fog dark:text-fog-light",
		"reading":     "text-teal-dark dark:text-cyan",
		"finished":    "text-deep dark:text-line",
		"abandoned":   "text-coral",
		"reference":   "text-rose",
	}
	if c, ok := colors[s]; ok {
		return c
	}
	return "text-fog dark:text-fog-light"
}

func statusDotClass(s string) string {
	dots := map[string]string{
		"antilibrary": "bg-fog-light",
		"queued":      "bg-cyan",
		"reading":     "bg-teal dark:bg-cyan",
		"finished":    "bg-deep dark:bg-line",
		"abandoned":   "bg-coral",
		"reference":   "bg-rose",
	}
	if d, ok := dots[s]; ok {
		return d
	}
	return "bg-fog-light"
}
