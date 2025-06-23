// agents/identity_fetcher.go
package agents

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

// GetCurrentEpoch queries the node for the current epoch.
func GetCurrentEpoch(nodeURL, apiKey string) (int, error) {
	reqData := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "bcn_epoch",
		"params":  []interface{}{},
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
		return 0, err
	}
	defer resp.Body.Close()
	var rpcResp struct {
		Result struct {
			Epoch int `json:"epoch"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return 0, err
	}
	return rpcResp.Result.Epoch, nil
}

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
		if err := RunIdentityFetcherOnce(cfg); err != nil {
			log.Printf("[AGENT][Fetcher] cycle error: %v", err)
		}
		time.Sleep(time.Duration(cfg.IntervalMinutes) * time.Minute)
	}
}

// RunIdentityFetcherOnce performs a single snapshot fetch using the provided configuration.
// It returns an error if any step (loading addresses, contacting the node, or writing the file)
// fails.
func RunIdentityFetcherOnce(cfg *FetcherConfig) error {
	log.Printf("[AGENT][Fetcher] loading addresses from %s", cfg.AddressListFile)
	addresses, err := LoadAddressList(cfg.AddressListFile)
	if err != nil {
		return fmt.Errorf("load address list: %w", err)
	}
	log.Printf("[AGENT][Fetcher] loaded %d addresses", len(addresses))

	epoch, err := GetCurrentEpoch(cfg.NodeURL, cfg.ApiKey)
	if err != nil {
		return fmt.Errorf("get current epoch: %w", err)
	}
	log.Printf("[AGENT][Fetcher] current epoch %d", epoch)
	outputPath := fmt.Sprintf("data/whitelist_epoch_%d.json", epoch)
	log.Printf("[AGENT][Fetcher] output file %s", outputPath)

	var snapshot []Identity
	for _, addr := range addresses {
		id, err := FetchIdentity(addr, cfg.NodeURL, cfg.ApiKey)
		if err != nil {
			log.Printf("[AGENT][Fetcher] fetch %s: %v", addr, err)
			continue
		}
		snapshot = append(snapshot, *id)
	}
	snapBytes, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}
	if err := os.WriteFile(outputPath, snapBytes, 0644); err != nil {
		return fmt.Errorf("write snapshot: %w", err)
	}
	log.Printf("[AGENT][Fetcher] wrote snapshot with %d addresses", len(snapshot))
	return nil
}

// RunIdentityFetcherAutoEpoch loads the config and executes a single fetch for the current epoch.
func RunIdentityFetcherAutoEpoch(configPath string) error {
	cfg, err := LoadFetcherConfig(configPath)
	if err != nil {
		return err
	}
	return RunIdentityFetcherOnce(cfg)
}
