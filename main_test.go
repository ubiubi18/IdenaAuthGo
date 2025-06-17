package main

import (
	"bytes"
	"crypto/ecdsa"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func signMessage(priv *ecdsa.PrivateKey, msg string) string {
	h := crypto.Keccak256(crypto.Keccak256([]byte(msg)))
	sig, _ := crypto.Sign(h, priv)
	return hex.EncodeToString(sig)
}

func TestVerifySignature(t *testing.T) {
	priv, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	addr := crypto.PubkeyToAddress(priv.PublicKey).Hex()
	nonce := "test-nonce"
	sigHex := signMessage(priv, nonce)
	if !verifySignature(nonce, addr, sigHex) {
		t.Fatalf("expected valid signature")
	}
	if verifySignature(nonce, addr, "deadbeef") {
		t.Fatalf("expected invalid signature")
	}
}

func TestGetIdentityRpc(t *testing.T) {
	oldClient := http.DefaultClient
	defer func() { http.DefaultClient = oldClient }()

	http.DefaultClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != idenaRpcUrl {
			t.Fatalf("unexpected url %s", req.URL.String())
		}
		resp := map[string]interface{}{
			"result": map[string]string{
				"state": "Human",
				"stake": "15000",
			},
		}
		b, _ := json.Marshal(resp)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(b)), Header: make(http.Header)}, nil
	})}

	state, stake := getIdentity("0xabc")
	if state != "Human" || stake != 15000 {
		t.Fatalf("unexpected result %s %.f", state, stake)
	}
}

func TestGetIdentityFallback(t *testing.T) {
	calls := 0
	oldClient := http.DefaultClient
	defer func() { http.DefaultClient = oldClient }()

	http.DefaultClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		switch req.URL.String() {
		case idenaRpcUrl:
			return &http.Response{StatusCode: 500, Body: io.NopCloser(bytes.NewReader(nil)), Header: make(http.Header)}, nil
		case fallbackApiUrl + "/api/Identity/0xabc":
			resp := map[string]interface{}{"result": map[string]string{"state": "Verified"}}
			b, _ := json.Marshal(resp)
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(b)), Header: make(http.Header)}, nil
		case fallbackApiUrl + "/api/Address/0xabc":
			resp := map[string]interface{}{"result": map[string]string{"stake": "9000"}}
			b, _ := json.Marshal(resp)
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(b)), Header: make(http.Header)}, nil
		default:
			t.Fatalf("unexpected url %s", req.URL.String())
		}
		return nil, nil
	})}

	state, stake := getIdentity("0xabc")
	if state != "Verified" || stake != 9000 {
		t.Fatalf("unexpected fallback result %s %.f", state, stake)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestStartSessionHandlerHealthCheck(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/auth/v1/start-session", nil)
	rr := httptest.NewRecorder()
	startSessionHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	body := strings.TrimSpace(rr.Body.String())
	if body != "ok" {
		t.Fatalf("expected body 'ok', got %q", body)
	}
}

func TestStartSessionHandlerMissingToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/auth/v1/start-session?address=0xabc", nil)
	rr := httptest.NewRecorder()
	startSessionHandler(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
}

func TestStartSessionHandlerMissingAddress(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/auth/v1/start-session?token=123", nil)
	rr := httptest.NewRecorder()
	startSessionHandler(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
}

func TestSanitizeBaseURL(t *testing.T) {
	httpsURL := sanitizeBaseURL("http://proofofhuman.work")
	if httpsURL != "https://proofofhuman.work" {
		t.Fatalf("expected https URL, got %s", httpsURL)
	}
	localURL := sanitizeBaseURL("http://localhost:3030")
	if localURL != "http://localhost:3030" {
		t.Fatalf("expected localhost unchanged, got %s", localURL)
	}
}

func setupTestDB(t *testing.T) {
	var err error
	db, err = sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	createSessionTable()
	createEpochSnapshotTable()
	createSnapshotMetaTable()
	createPenaltyTable()
	createMerkleRootTable()
	resultTmpl = mustLoadTemplate("templates/result.html")
}

func TestCallbackHandlerSessionNotFound(t *testing.T) {
	setupTestDB(t)
	defer db.Close()

	req := httptest.NewRequest(http.MethodGet, "/callback?token=deadbeef", nil)
	rr := httptest.NewRecorder()
	callbackHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Session not found") {
		t.Fatalf("missing not found message")
	}
	if !strings.Contains(body, "Address:") || !strings.Contains(body, "Status:") || !strings.Contains(body, "Stake:") {
		t.Fatalf("output missing address/status/stake block: %s", body)
	}
}

func TestCallbackHandlerNotEligible(t *testing.T) {
	setupTestDB(t)
	defer db.Close()

	_, err := db.Exec(`INSERT INTO sessions(token,address,authenticated,identity_state,stake,created) VALUES (?,?,?,?,?,?)`,
		"tok123", "0xabc", 0, "Suspended", 1.0, time.Now().Unix())
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/callback?token=tok123", nil)
	rr := httptest.NewRecorder()
	callbackHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Access denied!") {
		t.Fatalf("expected denied headline")
	}
	if !strings.Contains(body, "0xabc") || !strings.Contains(body, "Suspended") {
		t.Fatalf("missing address or status in body: %s", body)
	}
}
