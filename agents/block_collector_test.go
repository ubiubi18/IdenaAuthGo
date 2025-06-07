package agents

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestCollectShortAnswerTxs(t *testing.T) {
	blocks := map[int][]map[string]interface{}{
		10: {
			{"type": "ShortAnswersHashTx", "hash": "0x1"},
			{"type": "SendTx", "hash": "0x2"},
		},
		11: {},
	}

	oldClient := httpClient
	defer func() { httpClient = oldClient }()

	httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		bodyBytes, _ := io.ReadAll(req.Body)
		var rpcReq struct {
			Params []int `json:"params"`
		}
		_ = json.Unmarshal(bodyBytes, &rpcReq)
		height := rpcReq.Params[0]
		txs := blocks[height]
		respBody, _ := json.Marshal(map[string]interface{}{
			"result": map[string]interface{}{
				"transactions": txs,
			},
		})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(respBody)), Header: make(http.Header)}, nil
	})}

	txs, err := CollectShortAnswerTxs("http://node", "", 10, 11)
	if err != nil {
		t.Fatalf("collect error: %v", err)
	}
	if len(txs) != 1 {
		t.Fatalf("expected 1 tx, got %d", len(txs))
	}
	var tx map[string]interface{}
	if err := json.Unmarshal(txs[0], &tx); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if tx["hash"] != "0x1" {
		t.Fatalf("unexpected tx hash %v", tx["hash"])
	}
}
