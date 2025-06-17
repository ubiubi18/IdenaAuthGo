package main

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestEligibilitySnapshotHandlerNoSnapshot(t *testing.T) {
	setupTestDB(t)
	defer db.Close()

	identityFetcher = func(addr string) (string, float64) { return "", 0 }
	defer func() { identityFetcher = getIdentity }()

	req := httptest.NewRequest("GET", "/eligibility?address=0xabc", nil)
	rr := httptest.NewRecorder()
	eligibilitySnapshotHandler(rr, req)

	var out EligibilityResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Reason == "" || out.Epoch != 0 {
		t.Fatalf("unexpected: %+v", out)
	}
}

func TestEligibilitySnapshotHandlerEligible(t *testing.T) {
	setupTestDB(t)
	defer db.Close()

	identityFetcher = func(addr string) (string, float64) { return "Human", 7000 }
	defer func() { identityFetcher = getIdentity }()

	stakeThreshold = 6000
	saveSnapshotMeta(1, 123)
	_, err := db.Exec(`INSERT INTO epoch_identity_snapshot(epoch,address,state,stake,penalized,flipReported) VALUES (1,'0xabc','Human',7000,0,0)`)
	if err != nil {
		t.Fatalf("insert snapshot: %v", err)
	}

	req := httptest.NewRequest("GET", "/eligibility?address=0xabc", nil)
	rr := httptest.NewRecorder()
	eligibilitySnapshotHandler(rr, req)

	var out EligibilityResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !out.Eligible || out.State != "Human" || out.Epoch != 1 || out.Block != 123 || out.Prediction != "eligible both epochs" {
		t.Fatalf("unexpected output: %+v", out)
	}
}
