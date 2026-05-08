package index

// schemaVersion is incremented when a new migration is added.
const schemaVersion = 1

// DDL statements for migration v1.
const schemaDDL = `
CREATE TABLE IF NOT EXISTS schema_version (
    version INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS books (
    slug        TEXT PRIMARY KEY,
    file_path   TEXT NOT NULL,
    title       TEXT NOT NULL,
    author      TEXT NOT NULL,
    status      TEXT NOT NULL,
    track       TEXT NOT NULL DEFAULT '',
    started     TEXT,
    finished    TEXT,
    rating      INTEGER,
    themes      TEXT NOT NULL DEFAULT '[]',
    isbn        TEXT NOT NULL DEFAULT '',
    cover       TEXT NOT NULL DEFAULT '',
    acquired    TEXT,
    source      TEXT NOT NULL DEFAULT '',
    body_md     TEXT NOT NULL DEFAULT '',
    mod_time    DATETIME NOT NULL,
    indexed_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE VIRTUAL TABLE IF NOT EXISTS books_fts USING fts5(
    title,
    author,
    themes,
    body_md,
    content=books,
    content_rowid=rowid,
    tokenize='porter unicode61'
);

CREATE TRIGGER IF NOT EXISTS books_ai AFTER INSERT ON books BEGIN
    INSERT INTO books_fts(rowid, title, author, themes, body_md)
    VALUES (new.rowid, new.title, new.author, new.themes, new.body_md);
END;

CREATE TRIGGER IF NOT EXISTS books_ad AFTER DELETE ON books BEGIN
    INSERT INTO books_fts(books_fts, rowid, title, author, themes, body_md)
    VALUES ('delete', old.rowid, old.title, old.author, old.themes, old.body_md);
END;

CREATE TRIGGER IF NOT EXISTS books_au AFTER UPDATE ON books BEGIN
    INSERT INTO books_fts(books_fts, rowid, title, author, themes, body_md)
    VALUES ('delete', old.rowid, old.title, old.author, old.themes, old.body_md);
    INSERT INTO books_fts(rowid, title, author, themes, body_md)
    VALUES (new.rowid, new.title, new.author, new.themes, new.body_md);
END;

CREATE INDEX IF NOT EXISTS idx_books_status   ON books(status);
CREATE INDEX IF NOT EXISTS idx_books_track    ON books(track);
CREATE INDEX IF NOT EXISTS idx_books_author   ON books(author);
CREATE INDEX IF NOT EXISTS idx_books_finished ON books(finished);
`
