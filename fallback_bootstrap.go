package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"idenauthgo/checks"
	"idenauthgo/eligibility"
)

// requiredBlocks defines how many consecutive blocks after short session start
// are scanned for validation transactions. Overridable in tests.
var requiredBlocks = 7

// apiGet performs a GET request to the fallback API and decodes the JSON result
func apiGet(path string, out interface{}) error {
	url := strings.TrimRight(fallbackApiUrl, "/") + path
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// validation summary helper removed; use checks package

func buildEpochWhitelistAPI(epoch int, threshold float64) error {
	lastEpoch := epoch - 1
	var epInfo struct {
		Result struct {
			ValidationFirstBlock int `json:"validationFirstBlockHeight"`
		} `json:"result"`
	}
	if err := apiGet(fmt.Sprintf("/api/Epoch/%d", lastEpoch), &epInfo); err != nil {
		return fmt.Errorf("epoch info: %w", err)
	}
	guess := epInfo.Result.ValidationFirstBlock + 15
	shortStart := 0
	for h := guess; h < guess+20; h++ {
		var blk struct {
			Result struct {
				Flags []string `json:"flags"`
			} `json:"result"`
		}
		if err := apiGet(fmt.Sprintf("/api/Block/%d", h), &blk); err != nil {
			continue
		}
		for _, f := range blk.Result.Flags {
			if f == "ShortSessionStarted" {
				shortStart = h
				break
			}
		}
		if shortStart != 0 {
			break
		}
	}
	if shortStart == 0 {
		return fmt.Errorf("ShortSessionStarted block not found")
	}
	unique := make(map[string]struct{})
	height := shortStart
	blocks := 0
	for blocks < requiredBlocks {
		cont := ""
		hasTx := false
		for {
			path := fmt.Sprintf("/api/Block/%d/Txs?limit=100", height)
			if cont != "" {
				path += "&continuationToken=" + cont
			}
			var txRes struct {
				Result []struct {
					From string `json:"from"`
				} `json:"result"`
				Continuation string `json:"continuationToken"`
			}
			if err := apiGet(path, &txRes); err != nil {
				break
			}
			for _, tx := range txRes.Result {
				if tx.From != "" {
					unique[strings.ToLower(tx.From)] = struct{}{}
					hasTx = true
				}
			}
			if txRes.Continuation == "" {
				break
			}
			cont = txRes.Continuation
			time.Sleep(100 * time.Millisecond)
		}
		if hasTx {
			blocks++
		}
		height++
	}
	addresses := make([]string, 0, len(unique))
	for a := range unique {
		addresses = append(addresses, a)
	}
	var snaps []EpochSnapshot
	var list []string
	for _, addr := range addresses {
		sum, err := checks.FetchValidationSummary(fallbackApiUrl, IDENA_RPC_KEY, lastEpoch, addr)
		if err != nil {
			continue
		}
		penalized, flip, err := checks.CheckPenaltyFlipForEpoch(fallbackApiUrl, IDENA_RPC_KEY, lastEpoch, addr)
		if err != nil {
			continue
		}
		stake, _ := strconv.ParseFloat(sum.Stake, 64)
		snaps = append(snaps, EpochSnapshot{
			Address:      addr,
			State:        sum.State,
			Stake:        stake,
			Penalized:    penalized,
			FlipReported: flip,
		})
		if eligibility.IsEligibleFull(sum.State, stake, penalized, flip, threshold) {
			list = append(list, addr)
		}
	}
	if err := upsertEpochSnapshots(db, epoch, snaps); err != nil {
		return err
	}
	sort.Strings(list)
	root := computeMerkleRoot(list)
	saveMerkleRoot(epoch, root)
	path := fmt.Sprintf("data/whitelist_epoch_%d.json", epoch)
	data, _ := json.MarshalIndent(map[string]interface{}{
		"merkle_root": root,
		"addresses":   list,
	}, "", "  ")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}
	wlMu.Lock()
	currentWhitelist = list
	wlMu.Unlock()
	log.Printf("[WHITELIST] built via official API for epoch %d with %d addresses root=%s", epoch, len(list), root)
	return nil
}
