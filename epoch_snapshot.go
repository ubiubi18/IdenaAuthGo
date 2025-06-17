package main

import (
	"database/sql"
	"strings"
)

// EpochSnapshot represents eligibility data for one identity within an epoch.
type EpochSnapshot struct {
	Address      string
	State        string
	Stake        float64
	Penalized    bool
	FlipReported bool
}

// ensureEpochSnapshotTable creates the epoch_identity_snapshot table if it does not exist.
func ensureEpochSnapshotTable(db *sql.DB) error {
	_, err := db.Exec(`
        CREATE TABLE IF NOT EXISTS epoch_identity_snapshot (
            epoch INTEGER,
            address TEXT,
            state TEXT,
            stake REAL,
            penalized INTEGER,
            flipReported INTEGER,
            PRIMARY KEY (epoch, address)
        )`)
	return err
}

// upsertEpochSnapshots inserts or updates eligibility records for an epoch.
func upsertEpochSnapshots(db *sql.DB, epoch int, snaps []EpochSnapshot) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO epoch_identity_snapshot(epoch,address,state,stake,penalized,flipReported) VALUES(?,?,?,?,?,?)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	for _, s := range snaps {
		if _, err := stmt.Exec(epoch, strings.ToLower(s.Address), s.State, s.Stake, boolToInt(s.Penalized), boolToInt(s.FlipReported)); err != nil {
			stmt.Close()
			tx.Rollback()
			return err
		}
	}
	stmt.Close()
	return tx.Commit()
}

// queryEpochSnapshot returns the stored eligibility snapshot for an address and epoch.
// The ok flag is false if no record exists.
func queryEpochSnapshot(db *sql.DB, epoch int, addr string) (state string, stake float64, penalized, flip bool, ok bool, err error) {
	row := db.QueryRow(`SELECT state, stake, penalized, flipReported FROM epoch_identity_snapshot WHERE epoch=? AND address=?`, epoch, strings.ToLower(addr))
	var pen, fr int
	if err = row.Scan(&state, &stake, &pen, &fr); err != nil {
		if err == sql.ErrNoRows {
			return "", 0, false, false, false, nil
		}
		return "", 0, false, false, false, err
	}
	penalized = pen != 0
	flip = fr != 0
	ok = true
	return
}
