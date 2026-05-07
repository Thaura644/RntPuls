#!/usr/bin/env bash
set -euo pipefail

BACKUP_DIR="${BACKUP_DIR:-/backups}"
DB_USER="${DB_USER:-rentpulse}"
DB_NAME="${DB_NAME:-rentpulse}"
DB_HOST="${DB_HOST:-db}"
RETENTION_DAYS="${RETENTION_DAYS:-7}"

mkdir -p "$BACKUP_DIR"

TIMESTAMP=$(date +%Y%m%d_%H%M%S)
BACKUP_FILE="$BACKUP_DIR/rentpulse_$TIMESTAMP.sql.gz"

pg_dump -h "$DB_HOST" -U "$DB_USER" -d "$DB_NAME" | gzip > "$BACKUP_FILE"

echo "Backup created: $BACKUP_FILE"

find "$BACKUP_DIR" -name "rentpulse_*.sql.gz" -mtime +"$RETENTION_DAYS" -delete
echo "Cleaned up backups older than $RETENTION_DAYS days"
