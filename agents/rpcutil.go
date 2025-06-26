package agents

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

// rpcCall sends a JSON-RPC request to nodeURL using the given method and
// parameters. The result field of the response is decoded into the provided
// result pointer.
func rpcCall(nodeURL, apiKey, method string, params []interface{}, result interface{}) error {
	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  params,
	}
	if apiKey != "" {
		req["key"] = apiKey
	}
	body, _ := json.Marshal(req)
	resp, err := http.Post(nodeURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("rpc status %s", resp.Status)
	}
	var wrapper struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return err
	}
	if len(wrapper.Result) == 0 {
		return fmt.Errorf("empty result")
	}
	return json.Unmarshal(wrapper.Result, result)
}
