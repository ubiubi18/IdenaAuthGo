package whitelist

import (
	"bytes"
	"encoding/json"
	"net/http"
)

type identityResp struct {
	State               string   `json:"state"`
	Stake               string   `json:"stake"`
	Penalty             string   `json:"penalty"`
	LastValidationFlags []string `json:"lastValidationFlags"`
}

func fetchIdentity(addr string, epoch int, url, key string) (*identityResp, error) {
	params := []interface{}{addr}
	if epoch > 0 {
		params = append(params, epoch)
	}
	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "dna_identity",
		"params":  params,
		"id":      1,
	}
	if key != "" {
		req["key"] = key
	}
	body, _ := json.Marshal(req)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out struct {
		Result identityResp `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out.Result, nil
}
