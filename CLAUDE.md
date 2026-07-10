# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

bunbu-shelf is a local-first personal library and reading tracker. Each book
is a markdown file with a YAML frontmatter block; a Go server (`shelfd`)
watches a directory of these files, indexes them into SQLite for search/list
queries, and serves a server-rendered (htmx-driven) web UI. A companion CLI
(`shelf`) operates on the same index for scripting/terminal use.

There is no database of record — the markdown files under `<library>/books/`
*are* the source of truth. SQLite (`<library>/index.db`) is a derived,
disposable cache that can always be rebuilt via reindex.

## Commands

```sh
make build          # compile CSS, then build bin/shelfd and bin/shelf
make build-no-css    # build both binaries without touching CSS (app.css must already exist)
make test            # go test ./...
make lint            # go vet ./...
make css             # compile Tailwind (web/static/input.css -> web/static/app.css, minified)
make css-watch        # Tailwind in watch mode, for iterating on templates
make setup            # downloads the Tailwind CLI binary into .bin/ (needed once, before `make css`)
make reindex          # go run ./cmd/shelf reindex — rebuild the SQLite index from disk
```

Run a single test:

```sh
go test ./internal/book/ -run TestParseRoundTrip -v
go test -race ./...        # what CI runs
```

Running the server locally, in dev mode (templates reloaded from disk instead
of the embedded FS — run from repo root so `web/` resolves):

```sh
go run ./cmd/shelfd -dev -library /path/to/test-library
```

CI (`.github/workflows/ci.yml`) runs `go vet`, `go test -race ./...`, then
builds both binaries (after compiling CSS via the cached/downloaded Tailwind
binary). Releases are automated by `release-please` (conventional commits —
`feat:`, `fix:`, `chore:`, `ci:` — drive version bumps and CHANGELOG.md) and
GoReleaser (`.goreleaser.yml`), which cross-compiles `shelfd`/`shelf` for
linux/darwin/windows.

## Architecture

**`cmd/shelfd`** — the daemon: loads config, ensures the library directory
tree exists, opens the SQLite index, does a full reindex on startup, builds
the `server.Server`, starts an `fsnotify`-based `watcher.Watcher` on
`<library>/books/`, and serves HTTP until SIGINT/SIGTERM.

**`cmd/shelf`** — a CLI over the same `index.Store`/`book` packages
(`reindex`, `parse <path>`, `search <query>`, `list [--status]`). No HTTP
server; reads the same config and DB path as `shelfd`.

**`internal/book`** — the markdown file format and its only I/O:
- `book.go`: `Frontmatter` struct (the YAML schema — title, author, status,
  track, started/finished/acquired dates, rating, themes, isbn, cover,
  source) and the custom `Date` type (marshals as bare `YYYY-MM-DD`, not an
  RFC3339 timestamp).
- `parse.go`: splits a file into frontmatter + body by locating the `---`
  delimiters manually (not full front-matter library), then YAML-decodes the
  block. The body is preserved byte-for-byte.
- `write.go`: writes atomically (temp file + rename). `UpdateMeta` re-reads
  the file first so it doesn't clobber the body when only metadata changes.
- `validate.go`: field-level `ValidationError`s (title/author non-empty,
  status must be one of the enum, rating 1-5, finished not before started,
  ISBN 10/13 digit check).
- `slug.go`: filename slug ⇄ human title (`SlugFromPath` for reading a file,
  `SlugFromTitle` for creating one).

Book status is a closed enum: `antilibrary`, `queued`, `reading`,
`finished`, `abandoned`, `reference` (`book.ValidStatuses`). Any handler that
accepts a status from user input must check `book.IsValidStatus`.

**`internal/index`** — SQLite cache, wraps `modernc.org/sqlite` (pure-Go,
no cgo). One `books` table plus an FTS5 virtual table (`books_fts`,
porter/unicode61 tokenizer over title/author/themes/body) kept in sync via
`AFTER INSERT/UPDATE/DELETE` triggers defined in the DDL itself
(`schema.go`) — application code never touches `books_fts` directly.
Migrations (`migrations.go`) are a single-version bump-and-reapply scheme:
the whole DDL is idempotent (`CREATE ... IF NOT EXISTS`), so adding a new
table/index/trigger means bumping `schemaVersion` and appending to
`schemaDDL`, not writing a new incremental script. `CREATE TABLE IF NOT
EXISTS`, however, can't retroactively add a column to a `books` table that
already exists from an older version — new columns on an existing table go
through `addMissingColumns`, which checks `PRAGMA table_info(books)` and
`ALTER TABLE ADD COLUMN`s whatever's missing. It runs on every `Open()`
regardless of `schemaVersion`, so it's self-healing for any database that
was upgraded before a given column check existed, not just the version
transition that introduced it.

`Reindex(libraryDir)` is the sync entrypoint: it diffs `mod_time` on disk
against what's stored, only re-parses changed files, and deletes rows for
files that disappeared — all inside one transaction. `IndexBook`/`DeleteBook`
are the single-file equivalents used by the watcher and HTTP handlers so a
save doesn't require a full rescan.

**`internal/watcher`** — `fsnotify` on `<library>/books/`, debounced 100ms
per slug (editors like Vim/Neovim emit delete+create pairs for a single
save). On a change it re-parses and calls `store.IndexBook`/`DeleteBook`,
then invokes the caller's `EventHandler(slug)` callback — `cmd/shelfd` wires
this to `srv.Hub().Broadcast("book-updated", slug)` so external edits (e.g.
editing a `.md` file directly) propagate live to open browser tabs.

**`internal/server`** — stdlib `net/http` with Go 1.22+ pattern-based
`ServeMux` (`GET /shelf/{status}`, path values via `r.PathValue`). Split by
concern rather than by resource:
- `server.go`: route table and middleware chain (`loggingMiddleware` →
  `recoveryMiddleware` → mux). Static covers are served from disk
  (`<library>/covers/`) under `/static/covers/`, registered *before* the
  general `/static/` handler (backed by the embedded FS) so the
  more-specific route wins.
- `handlers.go`: read-only GET views (dashboard, shelf, book detail, search,
  themes, log, settings). Book detail always re-`book.ParseFile`s from disk
  even after loading from the index, to guarantee the freshest body.
- `crud.go`: mutating POST/DELETE endpoints. All writes go through
  `book.WriteFile`/`book.UpdateMeta` (disk, the source of truth) *then*
  `store.IndexBook` (cache) — never the reverse.
- `render.go`: template loading. Each `render()` call builds a fresh
  template set (base + all partials + the one named page) rather than
  parsing everything once globally, specifically to avoid `{{define
  "content"}}` collisions between pages. Supports both an embedded `fs.FS`
  (production) and reading straight from `web/` on disk (`-dev` flag, for
  editing templates without rebuilding).
- `sse.go`: a small in-process pub/sub hub (`sseHub`) for `/events`
  (Server-Sent Events); used to push `book-updated` notifications to the
  browser (from the watcher, or from `handleUpdateStatus`'s htmx endpoint).
- `markdown.go`: goldmark with `html.WithUnsafe()` — book bodies are trusted
  local content, not untrusted user input from the web.
- `openlibrary.go`: bridges `internal/openlibrary` search/cover-fetch into
  the "add from search" flow; cover images are fetched synchronously on add
  so the redirect target renders them on first paint.

**`internal/openlibrary`** — thin client for openlibrary.org's search and
cover-image endpoints. No API key. A cover image under 1000 bytes is treated
as Open Library's "no cover" placeholder and discarded rather than saved.

**`internal/config`** — TOML config at `~/.config/bunbu-shelf/config.toml`
(`config.DefaultPath()`), loaded via `LoadOrDefault` (defaults if the file
doesn't exist). `[buy_targets.<name>]` sections are hand-decoded from a raw
map (BurntSushi/toml can't target a `map[string]BuyTarget` directly with
sub-tables), so any new top-level TOML section with dynamic keys needs the
same two-pass raw-map + struct decode in `parse()`. Buy-link URL templates
support `{isbn}`, `{title}`, `{author}`, `{isbn_or_title}` token
substitution (duplicated in `config.BuyLink` and
`server.buildBuyURL` — keep them in sync if the token set changes).

**`web/`** — `web.go` embeds `templates/` and `static/` into the binary via
`go:embed`. `templates/base.html` + `templates/partials/*.html` +
one page template per route. Styling is Tailwind (`web/static/input.css` →
compiled `app.css`, gitignored — must be built via `make css`, not committed)
with a small custom "netflix" dark color palette in `tailwind.config.js`.
Frontend interactivity is htmx (`web/static/htmx.min.js`, vendored) plus SSE
for live updates — there is no JS build step or SPA framework.

## Conventions

- Writes always touch the markdown file first, then the SQLite index —
  disk is authoritative; the index is a rebuildable cache (`shelf reindex`
  is the escape hatch if they diverge).
- New status values, frontmatter fields, or config sections should be added
  in `internal/book`/`internal/config` first, then threaded through
  `internal/index` (schema + scan/insert code) and `internal/server` (forms
  + templates) — the book struct is the schema of record, SQLite mirrors it.
- Commit messages follow Conventional Commits (`feat:`, `fix:`, `chore:`,
  `ci:`, `docs:`) — release-please parses these to version bumps and
  `CHANGELOG.md` entries; don't hand-edit the changelog.
- Table-driven tests with fixtures under `testdata/books/*.md`; prefer
  extending those fixtures over inlining new markdown blobs when a test
  needs another edge case in the frontmatter format.
