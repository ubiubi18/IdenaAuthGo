package agents

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestCheckIdentityStatuses(t *testing.T) {
	dir := t.TempDir()
	addrFile := filepath.Join(dir, "addresses.txt")
	flipFile := filepath.Join(dir, "flips.txt")
	os.WriteFile(addrFile, []byte("0x1\n0x2\n"), 0644)
	os.WriteFile(flipFile, []byte("0x2\n"), 0644)

	oldClient := http.DefaultClient
	defer func() { http.DefaultClient = oldClient }()
	http.DefaultClient = &http.Client{Transport: rtFunc(func(req *http.Request) (*http.Response, error) {
		var reqObj struct {
			Params []string `json:"params"`
		}
		_ = json.NewDecoder(req.Body).Decode(&reqObj)
		respObj := map[string]interface{}{
			"result": map[string]string{"address": reqObj.Params[0], "state": "Human", "stake": "0"},
		}
		b, _ := json.Marshal(respObj)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(b)), Header: make(http.Header)}, nil
	})}

	cfg := &StatusCheckerConfig{
		NodeURL:         "http://example.com",
		AddressListFile: addrFile,
		FlipReportFile:  flipFile,
	}
	list, err := CheckIdentityStatuses(cfg)
	if err != nil {
		t.Fatalf("check failed: %v", err)
	}
	if len(list) != 1 || list[0].Address != "0x1" {
		t.Fatalf("unexpected result: %+v", list)
	}
}
