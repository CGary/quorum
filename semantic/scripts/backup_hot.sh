#!/bin/bash
set -e

# SQLite Hot Backup Script
# Usage: SQLITE_DB_PATH=path/to/db.db ./backup_hot.sh

DB_PATH="${SQLITE_DB_PATH:-/home/gary/dev/hsme/data/engram.db}"
BACKUP_DIR="${BACKUP_DIR:-/home/gary/dev/hsme/backups}"
TIMESTAMP=$(date -u +"%Y%m%dT%H%M%SZ")
DB_NAME=$(basename "$DB_PATH")
BACKUP_PATH="$BACKUP_DIR/${DB_NAME%.db}-$TIMESTAMP.db"

mkdir -p "$BACKUP_DIR"

echo "Backing up $DB_PATH to $BACKUP_PATH..."
sqlite3 "$DB_PATH" ".backup '$BACKUP_PATH'"

echo "Backup completed: $BACKUP_PATH"
echo "Size: $(stat -c%s "$BACKUP_PATH") bytes"
