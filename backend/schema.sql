CREATE TABLE IF NOT EXISTS share (
    id TEXT NOT NULL PRIMARY KEY,
    handle TEXT NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS file (
    id TEXT NOT NULL,
    name TEXT NOT NULL,
    expected_size INTEGER NOT NULL,
    `order` INTEGER NOT NULL,
    share_id TEXT NOT NULL,
    PRIMARY KEY (id),
    FOREIGN KEY (share_id) REFERENCES share(id) ON DELETE CASCADE,
    UNIQUE(share_id, `order`)
);