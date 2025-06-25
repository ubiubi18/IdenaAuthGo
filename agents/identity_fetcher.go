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

// defaultIndexerURL is the local rolling indexer endpoint that returns the
// current whitelist. It is used when no address file override is provided.
const defaultIndexerURL = "http://localhost:8080/api/whitelist/current"

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

// FetcherConfig specifies how the fetcher connects to the Idena node and how
// often it runs. The list of addresses is discovered automatically from the
// rolling indexer, so no address list is required here.
type FetcherConfig struct {
	IntervalMinutes int    `json:"interval_minutes"`
	NodeURL         string `json:"node_url"`
	ApiKey          string `json:"api_key"`
	IndexerURL      string `json:"indexer_url"`
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

// FetchAddressesFromIndexer retrieves the list of currently eligible addresses
// from the rolling indexer API. The endpoint is expected to return a JSON
// object with an "addresses" field.
func FetchAddressesFromIndexer(url string) ([]string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var data struct {
		Addresses []string `json:"addresses"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return data.Addresses, nil
}

// Main fetcher loop with full logging
func RunIdentityFetcher(configPath string) {
	log.Printf("[AGENT][Fetcher] RunIdentityFetcher called with configPath=%q", configPath)
	cfg, err := LoadFetcherConfig(configPath)
	if err != nil {
		log.Fatalf("[AGENT][Fetcher] Failed to load config: %v", err)
	}
	RunIdentityFetcherWithConfig(cfg, "")
}

// RunIdentityFetcherWithConfig executes the fetcher loop using the provided configuration.
func RunIdentityFetcherWithConfig(cfg *FetcherConfig, overrideFile string) {
	for {
		if err := RunIdentityFetcherOnce(cfg, overrideFile); err != nil {
			log.Printf("[AGENT][Fetcher] cycle error: %v", err)
		}
		time.Sleep(time.Duration(cfg.IntervalMinutes) * time.Minute)
		// only use the override on the first run
		overrideFile = ""
	}
}

// RunIdentityFetcherOnce performs a single snapshot fetch using the provided configuration.
// It returns an error if any step (loading addresses, contacting the node, or writing the file)
// fails.
func RunIdentityFetcherOnce(cfg *FetcherConfig, overrideFile string) error {
	var (
		addresses []string
		err       error
	)
	if overrideFile != "" {
		log.Printf("[AGENT][Fetcher] loading addresses from override %s", overrideFile)
		addresses, err = LoadAddressList(overrideFile)
		if err != nil {
			return fmt.Errorf("load address list: %w", err)
		}
	} else {
		url := cfg.IndexerURL
		if url == "" {
			url = defaultIndexerURL
		}
		log.Printf("[AGENT][Fetcher] fetching addresses from %s", url)
		addresses, err = FetchAddressesFromIndexer(url)
		if err != nil {
			return fmt.Errorf("fetch addresses from indexer: %w", err)
		}
	}
	log.Printf("[AGENT][Fetcher] using %d addresses", len(addresses))

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
func RunIdentityFetcherAutoEpoch(configPath, overrideFile string) error {
	cfg, err := LoadFetcherConfig(configPath)
	if err != nil {
		return err
	}
	return RunIdentityFetcherOnce(cfg, overrideFile)
}
