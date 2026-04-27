CREATE TABLE IF NOT EXISTS file (
    name     TEXT    NOT NULL,
    size     INTEGER NOT NULL,
    `index`  INTEGER NOT NULL,
    share_id TEXT    NOT NULL,
    PRIMARY KEY (share_id, `index`),
    FOREIGN KEY (share_id) REFERENCES share(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS share (
    id TEXT NOT NULL PRIMARY KEY,
    handle TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL
);
