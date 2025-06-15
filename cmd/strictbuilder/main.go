package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	addressFile           = "data/allAddresses.txt"
	outFile               = "data/idena_whitelist.jsonl"
	stakeThresholdFile    = "data/discriminationStakeThreshold.txt"
	newbieMinStake        = 10000.0
	verifiedMinStake      = 10000.0
	requiredBlocksWithTxs = 7
)

type epochInfo struct {
	Epoch                int    `json:"epoch"`
	Threshold            string `json:"discriminationStakeThreshold"`
	ValidationFirstBlock int    `json:"validationFirstBlockHeight"`
}

type blockResp struct {
	Result struct {
		Flags []string `json:"flags"`
	} `json:"result"`
}

type tx struct {
	From string `json:"from"`
}

type validationSummary struct {
	State     string `json:"state"`
	Stake     string `json:"stake"`
	Approved  bool   `json:"approved"`
	Penalized bool   `json:"penalized"`
}

func getJSON(url string, out interface{}) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func getLatestEpochInfo() (int, float64, error) {
	var data struct {
		Result struct {
			Epoch     int    `json:"epoch"`
			Threshold string `json:"discriminationStakeThreshold"`
		} `json:"result"`
	}
	if err := getJSON("https://api.idena.io/api/Epoch/Last", &data); err != nil {
		return 0, 0, err
	}
	thr, _ := strconv.ParseFloat(data.Result.Threshold, 64)
	return data.Result.Epoch, thr, nil
}

func getEpochInfo(epoch int) (int, error) {
	var data struct {
		Result struct {
			ValidationFirstBlock int `json:"validationFirstBlockHeight"`
		} `json:"result"`
	}
	if err := getJSON(fmt.Sprintf("https://api.idena.io/api/Epoch/%d", epoch), &data); err != nil {
		return 0, err
	}
	return data.Result.ValidationFirstBlock, nil
}

func getBlockFlags(height int) ([]string, error) {
	var br blockResp
	err := getJSON(fmt.Sprintf("https://api.idena.io/api/Block/%d", height), &br)
	if err != nil {
		return nil, err
	}
	return br.Result.Flags, nil
}

func fetchAllTxs(height int) ([]tx, error) {
	url := fmt.Sprintf("https://api.idena.io/api/Block/%d/Txs", height)
	var all []tx
	cont := ""
	for {
		full := url
		if cont != "" {
			full += "?continuationToken=" + cont
		}
		var resp struct {
			Result       []tx   `json:"result"`
			Continuation string `json:"continuationToken"`
		}
		if err := getJSON(full, &resp); err != nil {
			return all, err
		}
		if resp.Result != nil {
			all = append(all, resp.Result...)
		}
		if resp.Continuation == "" {
			break
		}
		cont = resp.Continuation
		time.Sleep(100 * time.Millisecond)
	}
	return all, nil
}

func findShortSessionBlock(start int) (int, error) {
	for h := start; h < start+20; h++ {
		flags, err := getBlockFlags(h)
		if err != nil {
			continue
		}
		for _, f := range flags {
			if f == "ShortSessionStarted" {
				return h, nil
			}
		}
	}
	return 0, fmt.Errorf("not found")
}

func fetchBadAddresses(epoch int) (map[string]struct{}, error) {
	bad := make(map[string]struct{})
	next := ""
	for {
		url := fmt.Sprintf("https://api.idena.io/api/Epoch/%d/Authors/Bad?limit=100", epoch)
		if next != "" {
			url += "&continuationToken=" + next
		}
		var resp struct {
			Result []struct {
				Address string `json:"address"`
			} `json:"result"`
			Continuation string `json:"continuationToken"`
		}
		if err := getJSON(url, &resp); err != nil {
			return bad, err
		}
		for _, r := range resp.Result {
			bad[strings.ToLower(r.Address)] = struct{}{}
		}
		if resp.Continuation == "" {
			break
		}
		next = resp.Continuation
		time.Sleep(100 * time.Millisecond)
	}
	return bad, nil
}

func collectShortSessionAddresses(required int) ([]string, float64, error) {
	latest, thr, err := getLatestEpochInfo()
	if err != nil {
		return nil, 0, err
	}
	os.WriteFile(stakeThresholdFile, []byte(fmt.Sprintf("%.8f", thr)), 0644)
	lastEpoch := latest - 1
	firstBlock, err := getEpochInfo(lastEpoch)
	if err != nil {
		return nil, 0, err
	}
	ssBlock, err := findShortSessionBlock(firstBlock + 15)
	if err != nil {
		return nil, 0, err
	}
	unique := make(map[string]struct{})
	blocksFound := 0
	cur := ssBlock
	for blocksFound < required {
		txs, err := fetchAllTxs(cur)
		if err != nil {
			cur++
			continue
		}
		if len(txs) > 0 {
			blocksFound++
			for _, t := range txs {
				if t.From != "" {
					unique[strings.ToLower(t.From)] = struct{}{}
				}
			}
		}
		cur++
	}
	var list []string
	for a := range unique {
		list = append(list, a)
	}
	os.WriteFile(addressFile, []byte(strings.Join(list, ",")), 0644)
	return list, thr, nil
}

func validationSummaryFor(epoch int, addr string) (validationSummary, error) {
	var data struct {
		Result validationSummary `json:"result"`
	}
	url := fmt.Sprintf("https://api.idena.io/api/Epoch/%d/Identity/%s/ValidationSummary", epoch, addr)
	err := getJSON(url, &data)
	return data.Result, err
}

func main() {
	addrs, threshold, err := collectShortSessionAddresses(requiredBlocksWithTxs)
	if err != nil {
		fmt.Println("error collecting addresses:", err)
		return
	}
	latest, _, err := getLatestEpochInfo()
	if err != nil {
		fmt.Println("epoch info error:", err)
		return
	}
	lastEpoch := latest - 1
	bad, err := fetchBadAddresses(lastEpoch)
	if err != nil {
		fmt.Println("fetch bad addresses:", err)
		return
	}
	out, err := os.Create(outFile)
	if err != nil {
		fmt.Println("open output:", err)
		return
	}
	defer out.Close()
	whitelisted := 0
	for i, addr := range addrs {
		addrL := strings.ToLower(addr)
		if _, ok := bad[addrL]; ok {
			fmt.Printf("[%d/%d] excluded bad author %s\n", i+1, len(addrs), addr)
			continue
		}
		sum, err := validationSummaryFor(lastEpoch, addr)
		if err != nil {
			fmt.Printf("[%d/%d] error %s: %v\n", i+1, len(addrs), addr, err)
			continue
		}
		stake, _ := strconv.ParseFloat(sum.Stake, 64)
		reason := ""
		if sum.Penalized || !sum.Approved {
			reason = "not approved or penalized"
		} else if sum.State == "Human" {
			if stake < threshold {
				reason = fmt.Sprintf("Human stake %.4f below threshold %.4f", stake, threshold)
			}
		} else if sum.State == "Newbie" {
			if stake < newbieMinStake {
				reason = fmt.Sprintf("Newbie stake %.4f below %f", stake, newbieMinStake)
			}
		} else if sum.State == "Verified" {
			if stake < verifiedMinStake {
				reason = fmt.Sprintf("Verified stake %.4f below %f", stake, verifiedMinStake)
			}
		} else {
			reason = "wrong state " + sum.State
		}
		if reason != "" {
			fmt.Printf("[%d/%d] EXCLUDED: %s - %s\n", i+1, len(addrs), addr, reason)
			continue
		}
		data, _ := json.Marshal(map[string]interface{}{
			"address": addr,
			"state":   sum.State,
			"stake":   stake,
		})
		out.Write(data)
		out.Write([]byte("\n"))
		whitelisted++
		fmt.Printf("[%d/%d] OK: %s state=%s stake=%.4f\n", i+1, len(addrs), addr, sum.State, stake)
		time.Sleep(200 * time.Millisecond)
	}
	fmt.Printf("Done. Whitelisted: %d addresses\n", whitelisted)
}
