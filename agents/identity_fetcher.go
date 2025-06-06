// agents/identity_fetcher.go
package agents

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type Identity struct {
	Address string  `json:"address"`
	State   string  `json:"state"`
	Stake   float64 `json:"stake"`
	Age     int     `json:"age"`
}

type FetcherConfig struct {
	IntervalMinutes int    `json:"interval_minutes"`
	NodeURL         string `json:"node_url"`
	ApiKey          string `json:"api_key"`
	SnapshotFile    string `json:"snapshot_file"`
	AddressListFile string `json:"address_list_file"`
}

func LoadFetcherConfig(configPath string) (*FetcherConfig, error) {
	f, err := os.Open(configPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var cfg FetcherConfig
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Example: Load list of addresses from file (one per line)
func LoadAddressList(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := []string{}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return lines, nil
}

func FetchIdentity(address, nodeURL, apiKey string) (*Identity, error) {
	reqData := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "dna_identity",
		"params":  []string{address},
		"id":      1,
	}
	if apiKey != "" {
		reqData["key"] = apiKey
	}
	body, _ := json.Marshal(reqData)
	req, _ := http.NewRequest("POST", nodeURL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var rpcResp struct {
		Result Identity `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return nil, err
	}
	return &rpcResp.Result, nil
}

// Main agent loop
func RunIdentityFetcher(configPath string) {
	cfg, err := LoadFetcherConfig(configPath)
	if err != nil {
		log.Fatalf("[AGENT][Fetcher] Failed to load config: %v", err)
	}
	for {
		log.Println("[AGENT][Fetcher] Starting new fetch cycle...")
		addresses, err := LoadAddressList(cfg.AddressListFile)
		if err != nil {
			log.Printf("[AGENT][Fetcher] Could not load addresses: %v", err)
			time.Sleep(time.Duration(cfg.IntervalMinutes) * time.Minute)
			continue
		}
		var snapshot []Identity
		for _, addr := range addresses {
			id, err := FetchIdentity(addr, cfg.NodeURL, cfg.ApiKey)
			if err != nil {
				log.Printf("[AGENT][Fetcher] Error for %s: %v", addr, err)
				continue
			}
			snapshot = append(snapshot, *id)
		}
		snapBytes, _ := json.MarshalIndent(snapshot, "", "  ")
		os.WriteFile(cfg.SnapshotFile, snapBytes, 0644)
		log.Printf("[AGENT][Fetcher] Wrote snapshot to %s (%d identities)", cfg.SnapshotFile, len(snapshot))
		time.Sleep(time.Duration(cfg.IntervalMinutes) * time.Minute)
	}
}
