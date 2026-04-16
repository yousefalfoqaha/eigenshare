CREATE TABLE IF NOT EXISTS file (
    id       TEXT    NOT NULL PRIMARY KEY,
    name     TEXT    NOT NULL,
    size     INTEGER NOT NULL,
    bitmask  BLOB    NOT NULL,
    share_id TEXT    NOT NULL,
    FOREIGN KEY (share_id) REFERENCES share(id)
);

CREATE TABLE IF NOT EXISTS share (
    id TEXT NOT NULL PRIMARY KEY,
    handle TEXT NOT NULL UNIQUE
);
