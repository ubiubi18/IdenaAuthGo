package main

import (
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

	// no snapshot file should produce an internal error
	req := httptest.NewRequest("GET", "/whitelist/check?address=0xabc", nil)
	rr := httptest.NewRecorder()
	whitelistCheckHandler(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for missing snapshot, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "no such file") {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}

	// now create a malformed snapshot file
	os.MkdirAll("data", 0755)
	path := "data/snapshot.json"
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
