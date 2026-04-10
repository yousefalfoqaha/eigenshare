CREATE TABLE IF NOT EXISTS file (
    id      text 	not null primary key,
    name    text    not null unique,
    size    INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS share (
	id TEXT NOT NULL PRIMARY_KEY
);

