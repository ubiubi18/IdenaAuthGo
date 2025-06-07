package agents

import (
	"encoding/json"
	"fmt"
	"net/http"
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

// fetchBlock retrieves block data by height using the node's REST API.
func fetchBlock(baseURL string, apiKey string, height int) (*Block, error) {
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
