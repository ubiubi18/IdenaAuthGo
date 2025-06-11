package agents

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

// SessionFinderConfig defines RPC connection settings
// for RunSessionBlockFinder.
type SessionFinderConfig struct {
	NodeURL string `json:"node_url"`
	ApiKey  string `json:"api_key"`
}

// LoadSessionFinderConfig reads configuration from a JSON file.
func LoadSessionFinderConfig(path string) (*SessionFinderConfig, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var cfg SessionFinderConfig
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func rpcCall(nodeURL, apiKey, method string, params []interface{}, out interface{}) error {
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
	return json.NewDecoder(resp.Body).Decode(out)
}

// WaitForSessionBlocks polls the node until it observes both
// ShortSessionStarted and LongSessionStarted flags. It returns the
// corresponding block heights.
func WaitForSessionBlocks(nodeURL, apiKey string) (int, int, error) {
	var shortStart, longStart int
	for {
		var last struct {
			Result struct {
				Height int `json:"height"`
			} `json:"result"`
		}
		if err := rpcCall(nodeURL, apiKey, "bcn_lastBlock", nil, &last); err != nil {
			return 0, 0, err
		}
		var block struct {
			Result struct {
				Height int      `json:"height"`
				Flags  []string `json:"flags"`
			} `json:"result"`
		}
		if err := rpcCall(nodeURL, apiKey, "bcn_block", []interface{}{last.Result.Height}, &block); err != nil {
			return 0, 0, err
		}
		for _, f := range block.Result.Flags {
			if shortStart == 0 && f == "ShortSessionStarted" {
				shortStart = block.Result.Height
			}
			if longStart == 0 && f == "LongSessionStarted" {
				longStart = block.Result.Height
			}
		}
		if shortStart != 0 && longStart != 0 {
			return shortStart, longStart, nil
		}
		time.Sleep(10 * time.Second)
	}
}

// RunSessionBlockFinder loads config and prints the session block range.
func RunSessionBlockFinder(configPath string) {
	cfg, err := LoadSessionFinderConfig(configPath)
	if err != nil {
		log.Fatalf("[SessionFinder] config error: %v", err)
	}
	short, long, err := WaitForSessionBlocks(cfg.NodeURL, cfg.ApiKey)
	if err != nil {
		log.Fatalf("[SessionFinder] %v", err)
	}
	log.Printf("[SessionFinder] Short session started at block %d", short)
	log.Printf("[SessionFinder] Long session started at block %d", long)
	log.Printf("[SessionFinder] Short answer window: %d-%d", short, short+5)
=======
	"strings"
	"time"
)

// Block represents a minimal subset of Idena block data used by the session finder.
type Block struct {
	Height int      `json:"height"`
	Flags  []string `json:"flags"`
}

// blockResponse models the response format for /api/Block endpoints.
type blockResponse struct {
	Result Block `json:"result"`
}

// fetchBlockREST retrieves block data by height using the node's REST API.
func fetchBlockREST(baseURL string, apiKey string, height int) (*Block, error) {
	url := strings.TrimRight(baseURL, "/") + fmt.Sprintf("/api/Block/%d", height)
	if apiKey != "" {
		url += "?apikey=" + apiKey
	}
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var br blockResponse
	if err := json.NewDecoder(resp.Body).Decode(&br); err != nil {
		return nil, err
	}
	return &br.Result, nil
}

// fetchLastBlock returns the latest block from the node.
func fetchLastBlock(baseURL string, apiKey string) (*Block, error) {
	url := strings.TrimRight(baseURL, "/") + "/api/Block/Last"
	if apiKey != "" {
		url += "?apikey=" + apiKey
	}
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var br blockResponse
	if err := json.NewDecoder(resp.Body).Decode(&br); err != nil {
		return nil, err
	}
	return &br.Result, nil
}

// containsFlag checks whether a block has a specific flag set.
func containsFlag(b *Block, flag string) bool {
	for _, f := range b.Flags {
		if f == flag {
			return true
		}
	}
	return false
}

// FindSessionBlocks polls the node until it detects the ShortSessionStarted and
// LongSessionStarted flags. It returns the block heights for both events.
// The pollInterval controls how often the node is queried for new blocks.
func FindSessionBlocks(baseURL, apiKey string, pollInterval time.Duration) (int, int, error) {
	if pollInterval <= 0 {
		pollInterval = 10 * time.Second
	}

	// Wait for ShortSessionStarted
	var shortHeight int
	for {
		blk, err := fetchLastBlock(baseURL, apiKey)
		if err != nil {
			return 0, 0, err
		}
		if containsFlag(blk, "ShortSessionStarted") {
			shortHeight = blk.Height
			break
		}
		time.Sleep(pollInterval)
	}

	// After the short session block is seen, the long session flag should
	// appear within the next few blocks.
	var longHeight int
	for {
		blk, err := fetchLastBlock(baseURL, apiKey)
		if err != nil {
			return 0, 0, err
		}
		if blk.Height >= shortHeight && containsFlag(blk, "LongSessionStarted") {
			longHeight = blk.Height
			break
		}
		time.Sleep(pollInterval)
	}

	return shortHeight, longHeight, nil
}

// SessionFinderConfig defines settings for RunSessionBlockFinder.
type SessionFinderConfig struct {
	NodeURL             string `json:"node_url"`
	ApiKey              string `json:"api_key"`
	PollIntervalSeconds int    `json:"poll_interval_seconds"`
}

// LoadSessionFinderConfig reads config from JSON file.
func LoadSessionFinderConfig(path string) (*SessionFinderConfig, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var cfg SessionFinderConfig
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// RunSessionBlockFinder waits for the session start blocks and logs them.
func RunSessionBlockFinder(configPath string) {
	cfg, err := LoadSessionFinderConfig(configPath)
	if err != nil {
		log.Fatalf("[SessionFinder] load config: %v", err)
	}
	poll := time.Duration(cfg.PollIntervalSeconds) * time.Second
	short, long, err := FindSessionBlocks(cfg.NodeURL, cfg.ApiKey, poll)
	if err != nil {
		log.Fatalf("[SessionFinder] find blocks: %v", err)
	}
	log.Printf("[SessionFinder] ShortSessionStarted at height %d", short)
	log.Printf("[SessionFinder] LongSessionStarted at height %d", long)
}

// RunSessionBlockFinderWithConfig runs the session finder using an already loaded config.
func RunSessionBlockFinderWithConfig(cfg *SessionFinderConfig) {
	poll := time.Duration(cfg.PollIntervalSeconds) * time.Second
	short, long, err := FindSessionBlocks(cfg.NodeURL, cfg.ApiKey, poll)
	if err != nil {
		log.Fatalf("[SessionFinder] find blocks: %v", err)
	}
	log.Printf("[SessionFinder] ShortSessionStarted at height %d", short)
	log.Printf("[SessionFinder] LongSessionStarted at height %d", long)
}
