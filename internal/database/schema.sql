CREATE TABLE IF NOT EXISTS share (
    id TEXT NOT NULL PRIMARY KEY,
    handle TEXT NOT NULL UNIQUE,
    visibility TEXT NOT NULL DEFAULT 'private',
    created_at TEXT NOT NULL,
    expires_at TEXT,
    trashed_at TEXT,
    password TEXT NOT NULL DEFAULT '',
    version INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS file (
    id TEXT NOT NULL,
    name TEXT NOT NULL,
    expected_size INTEGER NOT NULL,
    share_id TEXT NOT NULL,
    PRIMARY KEY (id),
    FOREIGN KEY (share_id) REFERENCES share(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS users (
    id TEXT NOT NULL PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    is_admin INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1
);

DROP TABLE IF EXISTS sessions;

CREATE TABLE IF NOT EXISTS share_owner (
    share_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    PRIMARY KEY (share_id, user_id),
    FOREIGN KEY (share_id) REFERENCES share(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
