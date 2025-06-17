package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWhitelistCheckHandlerEligible(t *testing.T) {
	setupTestDB(t)
	defer db.Close()

	stakeThreshold = 6000
	currentEpoch = 1
	identityFetcher = func(addr string) (string, float64) {
		return "Human", 7000
	}
	defer func() { identityFetcher = getIdentity }()
	_, err := db.Exec(`INSERT INTO epoch_identity_snapshot(epoch,address,state,stake,penalized,flipReported) VALUES (1,'0xabc','Human',7000,0,0)`)
	if err != nil {
		t.Fatalf("insert snapshot: %v", err)
	}

	req := httptest.NewRequest("GET", "/whitelist/check?address=0xabc", nil)
	rr := httptest.NewRecorder()
	whitelistCheckHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var out struct {
		Eligible bool    `json:"eligible"`
		State    string  `json:"state"`
		Stake    float64 `json:"stake"`
		Rule     string  `json:"rule"`
		Hint     string  `json:"hint"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !out.Eligible || out.State != "Human" || out.Stake != 7000 || out.Rule != "snapshot" || !strings.Contains(out.Hint, "6000") {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestWhitelistCheckHandlerNotEligible(t *testing.T) {
	setupTestDB(t)
	defer db.Close()

	currentEpoch = 1
	identityFetcher = func(addr string) (string, float64) {
		return "Suspended", 5000
	}
	defer func() { identityFetcher = getIdentity }()
	_, err := db.Exec(`INSERT INTO epoch_identity_snapshot(epoch,address,state,stake,penalized,flipReported) VALUES (1,'0xdef','Suspended',5000,0,0)`)
	if err != nil {
		t.Fatalf("insert snapshot: %v", err)
	}

	req := httptest.NewRequest("GET", "/whitelist/check?address=0xdef", nil)
	rr := httptest.NewRecorder()
	whitelistCheckHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var out struct {
		Eligible bool   `json:"eligible"`
		Reason   string `json:"reason"`
		Hint     string `json:"hint"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Eligible || out.Reason == "" || !strings.Contains(out.Hint, "6000") {
		t.Fatalf("unexpected output: %+v", out)
	}
}
