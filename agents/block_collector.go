package agents

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

var httpClient = http.DefaultClient

// fetchBlock retrieves a block by height using the bcn_block RPC method.
func fetchBlock(nodeURL, apiKey string, height int) ([]json.RawMessage, error) {
	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "bcn_block",
		"params":  []interface{}{height},
	}
	if apiKey != "" {
		req["key"] = apiKey
	}
	body, _ := json.Marshal(req)
	httpReq, _ := http.NewRequest("POST", nodeURL, bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rpc status %s", resp.Status)
	}
	var out struct {
		Result struct {
			Transactions []json.RawMessage `json:"transactions"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Result.Transactions, nil
}

// CollectShortAnswerTxs fetches blocks in the given height range and returns
// all transactions of type ShortAnswersHashTx found in those blocks.
func CollectShortAnswerTxs(nodeURL, apiKey string, startHeight, endHeight int) ([]json.RawMessage, error) {
	var res []json.RawMessage
	for h := startHeight; h <= endHeight; h++ {
		txs, err := fetchBlock(nodeURL, apiKey, h)
		if err != nil {
			return nil, fmt.Errorf("fetch block %d: %w", h, err)
		}
		for _, raw := range txs {
			var t struct {
				Type     string `json:"type"`
				TypeName string `json:"typeName"`
			}
			if err := json.Unmarshal(raw, &t); err != nil {
				continue
			}
			if t.Type == "ShortAnswersHashTx" || t.TypeName == "ShortAnswersHashTx" {
				// keep raw transaction for later processing
				res = append(res, raw)
			}
		}
	}
	return res, nil
}
