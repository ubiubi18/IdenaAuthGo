package whitelist

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestCheckFlipReports(t *testing.T) {
	addrs := []string{"0x1", "0x2"}
	oldClient := http.DefaultClient
	defer func() { http.DefaultClient = oldClient }()
	http.DefaultClient = &http.Client{Transport: rtFunc(func(req *http.Request) (*http.Response, error) {
		var rpcReq struct {
			Params []string `json:"params"`
		}
		_ = json.NewDecoder(req.Body).Decode(&rpcReq)
		addr := rpcReq.Params[0]
		var resp map[string]interface{}
		if addr == "0x2" {
			resp = map[string]interface{}{"result": map[string]interface{}{"lastValidationFlags": []string{"AtLeastOneFlipReported"}}}
		} else {
			resp = map[string]interface{}{"result": map[string]interface{}{"lastValidationFlags": []string{}}}
		}
		b, _ := json.Marshal(resp)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(b)), Header: make(http.Header)}, nil
	})}

	res, err := CheckFlipReports(addrs, 0, "http://node", "")
	if err != nil {
		t.Fatalf("check error: %v", err)
	}
	if res["0x1"] || !res["0x2"] {
		t.Fatalf("unexpected result %+v", res)
	}
}
