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

// fetchBadAuthorsAPI returns addresses reported for bad flips in the given epoch
func fetchBadAuthorsAPI(epoch int) (map[string]struct{}, error) {
	bad := make(map[string]struct{})
	cont := ""
	for {
		path := fmt.Sprintf("/api/Epoch/%d/Authors/Bad?limit=100", epoch)
		if cont != "" {
			path += "&continuationToken=" + cont
		}
		var res struct {
			Result []struct {
				Address string `json:"address"`
			} `json:"result"`
			Continuation string `json:"continuationToken"`
		}
		if err := apiGet(path, &res); err != nil {
			return bad, err
		}
		for _, r := range res.Result {
			bad[strings.ToLower(r.Address)] = struct{}{}
		}
		if res.Continuation == "" {
			break
		}
		cont = res.Continuation
		time.Sleep(100 * time.Millisecond)
	}
	return bad, nil
}

// validationSummaryAPI mirrors the ValidationSummary response from the API
type validationSummaryAPI struct {
	State     string `json:"state"`
	Stake     string `json:"stake"`
	Approved  bool   `json:"approved"`
	Penalized bool   `json:"penalized"`
}

func fetchValidationSummaryAPI(epoch int, addr string) (*validationSummaryAPI, error) {
	var out struct {
		Result validationSummaryAPI `json:"result"`
	}
	path := fmt.Sprintf("/api/Epoch/%d/Identity/%s/ValidationSummary", epoch, addr)
	if err := apiGet(path, &out); err != nil {
		return nil, err
	}
	return &out.Result, nil
}

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
	bad, err := fetchBadAuthorsAPI(lastEpoch)
	if err != nil {
		return fmt.Errorf("bad authors: %w", err)
	}
	var snaps []EpochSnapshot
	var list []string
	for _, addr := range addresses {
		sum, err := fetchValidationSummaryAPI(lastEpoch, addr)
		if err != nil {
			continue
		}
		stake, _ := strconv.ParseFloat(sum.Stake, 64)
		flip := false
		if _, ok := bad[addr]; ok {
			flip = true
		}
		snaps = append(snaps, EpochSnapshot{
			Address:      addr,
			State:        sum.State,
			Stake:        stake,
			Penalized:    sum.Penalized,
			FlipReported: flip,
		})
		if eligibility.IsEligibleFull(sum.State, stake, sum.Penalized, flip, threshold) {
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
