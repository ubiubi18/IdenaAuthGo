package main

import (
	"database/sql"
	"testing"
)

func TestEpochSnapshotInsertQuery(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := ensureEpochSnapshotTable(db); err != nil {
		t.Fatalf("create table: %v", err)
	}

	snaps := []EpochSnapshot{
		{Address: "0xabc", State: "Human", Stake: 7000},
		{Address: "0xdef", State: "Suspended", Stake: 5000, Penalized: true},
	}
	if err := upsertEpochSnapshots(db, 1, snaps); err != nil {
		t.Fatalf("insert: %v", err)
	}

	st, stk, pen, flip, ok, err := queryEpochSnapshot(db, 1, "0xabc")
	if err != nil || !ok {
		t.Fatalf("query ok: %v %v", ok, err)
	}
	if st != "Human" || stk != 7000 || pen || flip {
		t.Fatalf("unexpected values: %s %.f %v %v", st, stk, pen, flip)
	}

	_, _, _, _, ok, err = queryEpochSnapshot(db, 1, "0xffff")
	if err != nil {
		t.Fatalf("query missing: %v", err)
	}
	if ok {
		t.Fatalf("expected not found")
	}
}
