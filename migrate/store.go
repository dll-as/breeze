package migrate

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

// appliedRecord tracks a migration that has been applied.
type appliedRecord struct {
	Version   int
	Name      string
	Checksum  string
	AppliedAt time.Time
}

// ensureVersionTable creates the breeze_migrations table if it does not exist.
// Uses ANSI SQL that works on both Postgres and SQLite.
func ensureVersionTable(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS breeze_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			checksum TEXT NOT NULL,
			applied_at TIMESTAMP NOT NULL
		)
	`)
	return err
}

// appliedVersions reads all applied migrations from the database and returns
// them as a map keyed by version.
func appliedVersions(ctx context.Context, db *sql.DB) (map[int]appliedRecord, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT version, name, checksum, applied_at FROM breeze_migrations ORDER BY version
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query migrations: %w", err)
	}
	defer rows.Close()

	result := make(map[int]appliedRecord)
	for rows.Next() {
		var rec appliedRecord
		if err := rows.Scan(&rec.Version, &rec.Name, &rec.Checksum, &rec.AppliedAt); err != nil {
			return nil, fmt.Errorf("failed to scan migration record: %w", err)
		}
		result[rec.Version] = rec
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating migration records: %w", err)
	}
	return result, nil
}

// recordApplied inserts a migration record into the database within the given transaction.
func recordApplied(ctx context.Context, tx *sql.Tx, version int, name, sqlContent string) error {
	checksum := computeChecksum(sqlContent)
	_, err := tx.ExecContext(ctx, `
		INSERT INTO breeze_migrations (version, name, checksum, applied_at)
		VALUES (?, ?, ?, ?)
	`, version, name, checksum, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("failed to record migration %d: %w", version, err)
	}
	return nil
}

// removeApplied deletes a migration record from the database within the given transaction.
func removeApplied(ctx context.Context, tx *sql.Tx, version int) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM breeze_migrations WHERE version = ?`, version)
	if err != nil {
		return fmt.Errorf("failed to remove migration %d: %w", version, err)
	}
	return nil
}

// computeChecksum returns the SHA-256 hex digest of the given SQL content.
func computeChecksum(sqlContent string) string {
	h := sha256.Sum256([]byte(sqlContent))
	return hex.EncodeToString(h[:])
}
