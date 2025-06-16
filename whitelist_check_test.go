package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWhitelistCheckHandlerEligible(t *testing.T) {
	setupTestDB(t)
	defer db.Close()

	identityFetcher = func(addr string) (string, float64) {
		return "Human", 7000
	}
	stakeThreshold = 6000
	defer func() { identityFetcher = getIdentity }()

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
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !out.Eligible || out.State != "Human" || out.Stake != 7000 || out.Rule == "" {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestWhitelistCheckHandlerNotEligible(t *testing.T) {
	setupTestDB(t)
	defer db.Close()

	identityFetcher = func(addr string) (string, float64) {
		return "Suspended", 5000
	}
	defer func() { identityFetcher = getIdentity }()

	req := httptest.NewRequest("GET", "/whitelist/check?address=0xdef", nil)
	rr := httptest.NewRecorder()
	whitelistCheckHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var out struct {
		Eligible bool   `json:"eligible"`
		Reason   string `json:"reason"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Eligible || out.Reason == "" {
		t.Fatalf("unexpected output: %+v", out)
	}
}
