#!/usr/bin/env bash
# dosedu.kz — nightly PostgreSQL backup
# Intended to run via cron: 0 3 * * * /opt/dosedu/scripts/backup.sh

set -euo pipefail

DB_NAME="${POSTGRES_DB:-dosedu}"
DB_USER="${POSTGRES_USER:-dosedu}"
DB_HOST="${POSTGRES_HOST:-localhost}"
BACKUP_DIR="${BACKUP_DIR:-/var/backups/dosedu}"
RETENTION_DAYS="${RETENTION_DAYS:-14}"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

mkdir -p "$BACKUP_DIR"

pg_dump -h "$DB_HOST" -U "$DB_USER" -F c -f "$BACKUP_DIR/dosedu_${TIMESTAMP}.dump" "$DB_NAME"

# prune backups older than RETENTION_DAYS
find "$BACKUP_DIR" -name "dosedu_*.dump" -mtime +"$RETENTION_DAYS" -delete

echo "[backup] completed at $(date -Iseconds) -> dosedu_${TIMESTAMP}.dump"
