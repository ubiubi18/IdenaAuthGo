package agents

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
)

const localNodeURL = "http://localhost:9009"

// IdentityResult holds selected identity fields from the node RPC response.
type IdentityResult struct {
	Address             string   `json:"address"`
	State               string   `json:"state"`
	Stake               string   `json:"stake"`
	LastValidationFlags []string `json:"lastValidationFlags"`
	Penalty             string   `json:"penalty"`
}

// fetchIdentity retrieves identity information for a single address from a local
// Idena node using the dna_identity RPC method. The API key is optional; if
// provided it will be included in the request body under the "key" field.
func fetchIdentity(address, apiKey string) (*IdentityResult, error) {
	reqData := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "dna_identity",
		"params":  []interface{}{address},
		"id":      1,
	}
	if apiKey != "" {
		reqData["key"] = apiKey
	}

	body, err := json.Marshal(reqData)
	if err != nil {
		return nil, err
	}

	resp, err := http.Post(localNodeURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rpc status %s", resp.Status)
	}

	var out struct {
		Result IdentityResult `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if out.Error != nil {
		return nil, fmt.Errorf("rpc error %d: %s", out.Error.Code, out.Error.Message)
	}
	return &out.Result, nil
}

// fetchIdentities retrieves identity info for multiple addresses concurrently.
// It spawns a goroutine per address using fetchIdentity and returns a map
// of address to result. Failed lookups are logged and omitted from the map.
// Concurrency is limited via a buffered channel acting as a semaphore.
func fetchIdentities(addresses []string, apiKey string) map[string]IdentityResult {
	results := make(map[string]IdentityResult)
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 5)

	for _, addr := range addresses {
		addr := addr
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			res, err := fetchIdentity(addr, apiKey)
			if err != nil {
				log.Printf("[fetchIdentities] %s: %v", addr, err)
				return
			}
			mu.Lock()
			results[addr] = *res
			mu.Unlock()
		}()
	}

	wg.Wait()
	return results
}
