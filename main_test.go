package main

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
