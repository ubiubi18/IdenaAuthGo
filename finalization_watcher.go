package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"
)

// block represents minimal block data for flag checks.
type block struct {
	Height int      `json:"height"`
	Epoch  int      `json:"epoch"`
	Flags  []string `json:"flags"`
}

type blockResp struct {
	Result block `json:"result"`
}

// fetchLastBlock retrieves the latest block via REST API.
func fetchLastBlock(baseURL, apiKey string) (*block, error) {
	url := strings.TrimRight(baseURL, "/") + "/api/Block/Last"
	if apiKey != "" {
		url += "?apikey=" + apiKey
	}
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out blockResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out.Result, nil
}

func blockHasFlag(b *block, flag string) bool {
	for _, f := range b.Flags {
		if f == flag {
			return true
		}
	}
	return false
}

// watchEpochFinalization waits for the EpochFinalized flag and triggers the
// whitelist snapshot exactly once per epoch.
func watchEpochFinalization() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	wlMu.RLock()
	last := currentEpoch
	wlMu.RUnlock()

	for {
		blk, err := fetchLastBlock(idenaRpcUrl, IDENA_RPC_KEY)
		if err != nil {
			log.Printf("[FINALIZE] block fetch: %v", err)
			<-ticker.C
			continue
		}
		if blockHasFlag(blk, "EpochFinalized") && blk.Epoch > last {
			epoch, thr, err := fetchEpochData()
			if err != nil {
				log.Printf("[FINALIZE] epoch fetch: %v", err)
				<-ticker.C
				continue
			}
			if err := buildEpochWhitelist(epoch, thr); err != nil {
				log.Printf("[FINALIZE] build whitelist: %v", err)
			} else {
				wlMu.Lock()
				currentEpoch = epoch
				wlMu.Unlock()
				setConfigInt("current_epoch", epoch)
				last = epoch
			}
		}
		<-ticker.C
	}
}
