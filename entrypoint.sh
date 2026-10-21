#!/bin/sh
set -e

DATA_DIR="${DATA_PATH:-/data}"

if [ "$(id -u)" -ne 0 ]; then
    exec "$@"
fi

PUID=${PUID:-1000}
PGID=${PGID:-1000}

if ! getent group eigenshare-group > /dev/null 2>&1; then
    addgroup -g "$PGID" eigenshare-group 2>/dev/null || true
fi

if ! id -u eigenshare > /dev/null 2>&1; then
    if ! getent passwd "$PUID" > /dev/null 2>&1; then
        adduser -u "$PUID" -G eigenshare-group -D -H -s /sbin/nologin eigenshare 2>/dev/null || true
    fi
fi

mkdir -p "$DATA_DIR"
find "$DATA_DIR" \( ! -group "$PGID" -o ! -user "$PUID" \) -exec chown "$PUID:$PGID" {} + 2>/dev/null || true

exec su-exec "$PUID:$PGID" "$@"
