package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// Test that the whitelistCheckHandler returns a descriptive error when the
// whitelist file is missing or contains invalid JSON.
func TestWhitelistCheckHandlerSnapshotErrors(t *testing.T) {
	setupTestDB(t)
	defer db.Close()

	// use a non-existent epoch so the handler attempts to read a missing file
	currentEpoch = 424242
	req := httptest.NewRequest("GET", "/whitelist/check?address=0xabc", nil)
	rr := httptest.NewRecorder()
	whitelistCheckHandler(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for missing snapshot, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "no such file") {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}

	// now create a malformed whitelist file
	os.MkdirAll("data", 0755)
	path := fmt.Sprintf("data/whitelist_epoch_%d.json", currentEpoch)
	os.WriteFile(path, []byte("{"), 0644)
	defer os.Remove(path)

	rr = httptest.NewRecorder()
	whitelistCheckHandler(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for malformed snapshot, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "invalid character") && !strings.Contains(body, "unexpected end") {
		t.Fatalf("unexpected body: %s", body)
	}
}
