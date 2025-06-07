package agents

import (
	"bytes"
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
}
