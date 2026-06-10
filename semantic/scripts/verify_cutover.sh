#!/bin/bash
set -e

# Cutover Verification Script
# Usage: ./verify_cutover.sh

HSME_DB="${HSME_DB_PATH:-/home/gary/dev/hsme/data/engram.db}"
LEGACY_DB="${LEGACY_DB_PATH:-/home/gary/.engram/engram.db}"

# Count active rows in HSME
HSME_COUNT=$(sqlite3 "$HSME_DB" "SELECT count(*) FROM memories WHERE status='active'")

# Count non-deleted rows in Legacy
LEGACY_COUNT=$(sqlite3 "$LEGACY_DB" "SELECT count(*) FROM observations WHERE deleted_at IS NULL")

# Check for 'engram_migration' tags remaining in HSME
MIGRATION_TAGS=$(sqlite3 "$HSME_DB" "SELECT count(*) FROM memories WHERE source_type LIKE 'engram_migration%'")

TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

echo -e "timestamp\thsme_active\tlegacy_active\tmigration_tags_remaining"
echo -e "$TIMESTAMP\t$HSME_COUNT\t$LEGACY_COUNT\t$MIGRATION_TAGS"
