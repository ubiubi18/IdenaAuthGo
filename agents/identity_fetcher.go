// agents/identity_fetcher.go
package agents

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type Identity struct {
	Address string  `json:"address"`
	State   string  `json:"state"`
	Stake   float64 `json:"stake,string"`
	Age     int     `json:"age"`
}

type FetcherConfig struct {
	IntervalMinutes int    `json:"interval_minutes"`
	NodeURL         string `json:"node_url"`
	ApiKey          string `json:"api_key"`
	SnapshotFile    string `json:"snapshot_file"`
	AddressListFile string `json:"address_list_file"`
}

// Load fetcher configuration from JSON file, print debug info
func LoadFetcherConfig(configPath string) (*FetcherConfig, error) {
	log.Printf("[AGENT][Fetcher] Loading fetcher config from %s", configPath)
	f, err := os.Open(configPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var cfg FetcherConfig
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, err
	}
	log.Printf("[AGENT][Fetcher] Config loaded: %+v", cfg)
	return &cfg, nil
}

// Load list of addresses from file (one per line), print debug info
func LoadAddressList(path string) ([]string, error) {
	log.Printf("[AGENT][Fetcher] Loading address list from: %s", path)
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("[AGENT][Fetcher] Error reading address list: %v", err)
		return nil, err
	}
	lines := []string{}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	log.Printf("[AGENT][Fetcher] Loaded %d addresses", len(lines))
	return lines, nil
}

// Fetch identity details from node; log raw response and decoded result
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
		log.Printf("[AGENT][Fetcher] HTTP error for %s: %v", address, err)
		return nil, err
	}
	defer resp.Body.Close()

	// Log raw body for debugging
	bodyBytes, _ := io.ReadAll(resp.Body)
	log.Printf("[AGENT][Fetcher] Raw RPC response for %s: %s", address, string(bodyBytes))
	resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes)) // Reset for decoder

	var rpcResp struct {
		Result Identity `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		log.Printf("[AGENT][Fetcher] Failed to decode JSON for %s: %v", address, err)
		return nil, err
	}
	log.Printf("[AGENT][Fetcher] Decoded identity for %s: %+v", address, rpcResp.Result)
	return &rpcResp.Result, nil
}

// Main fetcher loop with full logging
func RunIdentityFetcher(configPath string) {
	log.Printf("[AGENT][Fetcher] RunIdentityFetcher called with configPath=%q", configPath)
	cfg, err := LoadFetcherConfig(configPath)
	if err != nil {
		log.Fatalf("[AGENT][Fetcher] Failed to load config: %v", err)
	}
	RunIdentityFetcherWithConfig(cfg)
}

// RunIdentityFetcherWithConfig executes the fetcher loop using the provided configuration.
func RunIdentityFetcherWithConfig(cfg *FetcherConfig) {
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
