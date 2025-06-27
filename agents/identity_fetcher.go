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
	"strconv"
	"strings"
	"time"

	"idenauthgo/checks"
)

// defaultIndexerURL is the local rolling indexer endpoint that returns the
// current whitelist. It is used when no address file override is provided.
const defaultIndexerURL = "http://localhost:8080/api/whitelist/current"

// GetCurrentEpoch queries the node for the current epoch.
func GetCurrentEpoch(nodeURL, apiKey string) (int, error) {
	reqData := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "dna_epoch",
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
			StartBlock     int    `json:"startBlock"`
			Epoch          int    `json:"epoch"`
			NextValidation string `json:"nextValidation"`
			CurrentPeriod  string `json:"currentPeriod"`
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

// ValidationSummary mirrors the node's ValidationSummary REST response.
type ValidationSummary struct {
	State     string  `json:"state"`
	Stake     float64 `json:"stake,string"`
	Approved  bool    `json:"approved"`
	Penalized bool    `json:"penalized"`
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

// getEpochLast fetches the current epoch and discrimination threshold.
func getEpochLast(nodeURL, apiKey string) (int, float64, error) {
	url := strings.TrimRight(nodeURL, "/") + "/api/Epoch/Last"
	if apiKey != "" {
		url += "?apikey=" + apiKey
	}
	resp, err := http.Get(url)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()
	var out struct {
		Result struct {
			Epoch     int     `json:"epoch"`
			Threshold float64 `json:"discriminationStakeThreshold"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, 0, err
	}
	return out.Result.Epoch, out.Result.Threshold, nil
}

// fetchBadAuthors returns a set of bad authors for the given epoch.
func fetchBadAuthors(nodeURL, apiKey string, epoch int) (map[string]struct{}, error) {
	base := strings.TrimRight(nodeURL, "/")
	return checks.BadAuthors(base, apiKey, epoch)
}

// fetchValidationSummary retrieves validation summary for an address.
func fetchValidationSummary(nodeURL, apiKey string, epoch int, addr string) (*ValidationSummary, error) {
	base := strings.TrimRight(nodeURL, "/")
	sum, err := checks.FetchValidationSummary(base, apiKey, epoch, addr)
	if err != nil {
		return nil, err
	}
	stake, _ := strconv.ParseFloat(sum.Stake, 64)
	return &ValidationSummary{
		State:     sum.State,
		Stake:     stake,
		Approved:  sum.Approved,
		Penalized: sum.Penalized,
	}, nil
}

// fetchBlockRPC retrieves block data for the given height via JSON-RPC.
func fetchBlockRPC(nodeURL, apiKey string, height int) (*Block, error) {
	var b Block
	if err := rpcCall(nodeURL, apiKey, "dna_getBlockByHeight", []interface{}{height}, &b); err != nil {
		return nil, err
	}
	return &b, nil
}

// fetchLastBlock obtains the latest block using bcn_lastBlock followed by dna_getBlockByHeight.
func fetchLastBlockRPC(nodeURL, apiKey string) (*Block, error) {
	var last struct {
		Height int `json:"height"`
	}
	if err := rpcCall(nodeURL, apiKey, "bcn_lastBlock", nil, &last); err != nil {
		return nil, err
	}
	return fetchBlockRPC(nodeURL, apiKey, last.Height)
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
		// find the block where ShortSessionStarted flag appears
		last, err := fetchLastBlockRPC(cfg.NodeURL, cfg.ApiKey)
		if err != nil {
			return fmt.Errorf("fetch last block: %w", err)
		}
		shortHeight := 0
		for h := last.Height; h >= 0 && h >= last.Height-2000; h-- {
			blk, err := fetchBlockRPC(cfg.NodeURL, cfg.ApiKey, h)
			if err != nil {
				continue
			}
			if containsFlag(blk, "ShortSessionStarted") {
				shortHeight = h
				break
			}
		}
		if shortHeight == 0 {
			return fmt.Errorf("ShortSessionStarted block not found")
		}

		// helper to fetch all transactions of a block via RPC
		type tx struct {
			From string `json:"from"`
		}
		fetchTxs := func(height int) ([]tx, error) {
			var txs []tx
			if err := rpcCall(cfg.NodeURL, cfg.ApiKey, "dna_getBlockTxs", []interface{}{height}, &txs); err != nil {
				return nil, err
			}
			return txs, nil
		}

		unique := make(map[string]struct{})
		blocks := 0
		h := shortHeight
		for blocks < 7 {
			txs, err := fetchTxs(h)
			if err == nil && len(txs) > 0 {
				blocks++
				for _, t := range txs {
					if t.From != "" {
						unique[strings.ToLower(t.From)] = struct{}{}
					}
				}
			}
			h++
		}
		for a := range unique {
			addresses = append(addresses, a)
		}
	}
	log.Printf("[AGENT][Fetcher] using %d addresses", len(addresses))

	epoch, threshold, err := getEpochLast(cfg.NodeURL, cfg.ApiKey)
	if err != nil {
		return fmt.Errorf("epoch info: %w", err)
	}
	lastEpoch := epoch - 1
	log.Printf("[AGENT][Fetcher] current epoch %d threshold %.4f", epoch, threshold)
	outputPath := fmt.Sprintf("data/whitelist_epoch_%d.json", epoch)
	log.Printf("[AGENT][Fetcher] output file %s", outputPath)

	var whitelist []string
	for _, addr := range addresses {
		addrL := strings.ToLower(addr)
		pen, flip, err := checks.CheckPenaltyFlipForEpoch(cfg.NodeURL, cfg.ApiKey, lastEpoch, addrL)
		if err != nil {
			log.Printf("[AGENT][Fetcher] check %s: %v", addrL, err)
			continue
		}
		if pen || flip {
			continue
		}
		sum, err := fetchValidationSummary(cfg.NodeURL, cfg.ApiKey, lastEpoch, addrL)
		if err != nil {
			log.Printf("[AGENT][Fetcher] summary %s: %v", addrL, err)
			continue
		}
		if sum.State == "Human" && sum.Stake < threshold {
			continue
		}
		if (sum.State == "Newbie" || sum.State == "Verified") && sum.Stake < 10000 {
			continue
		}
		whitelist = append(whitelist, addrL)
	}
	b, err := json.MarshalIndent(whitelist, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal whitelist: %w", err)
	}
	if err := os.WriteFile(outputPath, b, 0644); err != nil {
		return fmt.Errorf("write whitelist: %w", err)
	}
	log.Printf("[AGENT][Fetcher] wrote whitelist with %d addresses", len(whitelist))
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
