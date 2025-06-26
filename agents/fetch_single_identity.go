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

// BalanceResult holds balance and stake fields from the dna_getBalance RPC response.
type BalanceResult struct {
    Stake   string `json:"stake"`
    Balance string `json:"balance"`
}

// AccountInfo combines identity and balance info for an address.
type AccountInfo struct {
    Address             string
    State               string
    Stake               string
    Balance             string
    LastValidationFlags []string
    Penalty             string
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

// fetchBalance retrieves balance information for a single address from the local
// Idena node using the dna_getBalance RPC method.
func fetchBalance(address, apiKey string) (*BalanceResult, error) {
    reqData := map[string]interface{}{
        "jsonrpc": "2.0",
        "method":  "dna_getBalance",
        "params":  []interface{}{address},
        "id":      2,
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
        Result BalanceResult `json:"result"`
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

// fetchAccountInfos retrieves identity and balance info for multiple addresses concurrently.
// It spawns a goroutine per address using fetchIdentity and fetchBalance and returns a map
// of address to combined result. Failed lookups are logged and omitted from the map.
// Concurrency is limited via a buffered channel acting as a semaphore.
func fetchAccountInfos(addresses []string, apiKey string) map[string]AccountInfo {
        results := make(map[string]AccountInfo)
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

                        idRes, err := fetchIdentity(addr, apiKey)
                        if err != nil {
                                log.Printf("[fetchAccountInfos] identity %s: %v", addr, err)
                                return
                        }
                        balRes, err := fetchBalance(addr, apiKey)
                        if err != nil {
                                log.Printf("[fetchAccountInfos] balance %s: %v", addr, err)
                                return
                        }
                        mu.Lock()
                        results[addr] = AccountInfo{
                                Address:             idRes.Address,
                                State:               idRes.State,
                                Stake:               balRes.Stake,
                                Balance:             balRes.Balance,
                                LastValidationFlags: idRes.LastValidationFlags,
                                Penalty:             idRes.Penalty,
                        }
                        mu.Unlock()
                }()
        }

        wg.Wait()
        return results
}
