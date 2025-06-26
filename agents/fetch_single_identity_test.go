package agents

import (
	"encoding/json"
	"net"
	"net/http"
	"testing"
)

func TestFetchAccountInfos(t *testing.T) {
	ln, err := net.Listen("tcp", "localhost:9009")
	if err != nil {
		t.Skipf("listen: %v", err)
	}
        srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                var req struct {
                        Method string        `json:"method"`
                        Params []string      `json:"params"`
                }
                _ = json.NewDecoder(r.Body).Decode(&req)
                var resp map[string]interface{}
                switch req.Method {
                case "dna_identity":
                        resp = map[string]interface{}{
                                "result": map[string]interface{}{
                                        "address": req.Params[0],
                                        "state":   "Human",
                                        "stake":   "0",
                                        "lastValidationFlags": []string{},
                                        "penalty": "0",
                                },
                        }
                case "dna_getBalance":
                        resp = map[string]interface{}{
                                "result": map[string]interface{}{
                                        "stake":   "1",
                                        "balance": "2",
                                },
                        }
                }
                b, _ := json.Marshal(resp)
                w.Header().Set("Content-Type", "application/json")
                w.Write(b)
        })}
	go srv.Serve(ln)
	defer srv.Close()

	addrs := []string{"0x1", "0x2"}
        res := fetchAccountInfos(addrs, "")
        if len(res) != 2 {
                t.Fatalf("expected 2 results, got %d", len(res))
        }
        for _, a := range addrs {
                if r, ok := res[a]; !ok || r.Address != a || r.Balance != "2" {
                        t.Errorf("missing result for %s", a)
                }
        }
}
