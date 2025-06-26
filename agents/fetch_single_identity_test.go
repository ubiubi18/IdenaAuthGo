package agents

import (
	"encoding/json"
	"net"
	"net/http"
	"testing"
)

func TestFetchIdentities(t *testing.T) {
	ln, err := net.Listen("tcp", "localhost:9009")
	if err != nil {
		t.Skipf("listen: %v", err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Params []string `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		resp := map[string]interface{}{
			"result": map[string]interface{}{
				"address": req.Params[0],
				"state":   "Human",
				"stake":   "0",
			},
		}
		b, _ := json.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	})}
	go srv.Serve(ln)
	defer srv.Close()

	addrs := []string{"0x1", "0x2"}
	res := fetchIdentities(addrs, "")
	if len(res) != 2 {
		t.Fatalf("expected 2 results, got %d", len(res))
	}
	for _, a := range addrs {
		if r, ok := res[a]; !ok || r.Address != a {
			t.Errorf("missing result for %s", a)
		}
	}
}
