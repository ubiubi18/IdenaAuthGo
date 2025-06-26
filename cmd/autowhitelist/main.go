package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"idenauthgo/agents"
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

const (
	defaultNodeURL   = "http://localhost:9009"
	addressFile      = "addresses.txt"
	outFile          = "idena_whitelist.jsonl"
	thresholdOutFile = "discriminationStakeThreshold.txt"
	newbieMinStake   = 10000.0
	verifiedMinStake = 10000.0
	requiredBlocks   = 7
)

type epochLastResp struct {
	Result struct {
		Epoch     int    `json:"epoch"`
		Threshold string `json:"discriminationStakeThreshold"`
	} `json:"result"`
}

type epochResp struct {
	Result struct {
		ValidationFirstBlock int `json:"validationFirstBlockHeight"`
	} `json:"result"`
}

type blockFlagsResp struct {
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

func apiGet(baseURL, apiKey, path string, out interface{}) error {
	url := strings.TrimRight(baseURL, "/") + path
	if apiKey != "" {
		if strings.Contains(url, "?") {
			url += "&apikey=" + apiKey
		} else {
			url += "?apikey=" + apiKey
		}
	}
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

func getLatestEpochInfo(nodeURL, apiKey string) (int, float64, error) {
	var res epochLastResp
	if err := apiGet(nodeURL, apiKey, "/api/Epoch/Last", &res); err != nil {
		return 0, 0, err
	}
	thr, _ := strconv.ParseFloat(res.Result.Threshold, 64)
	return res.Result.Epoch, thr, nil
}

func getEpochInfo(nodeURL, apiKey string, epoch int) (int, error) {
	var res epochResp
	if err := apiGet(nodeURL, apiKey, fmt.Sprintf("/api/Epoch/%d", epoch), &res); err != nil {
		return 0, err
	}
	return res.Result.ValidationFirstBlock, nil
}

func getBlockFlags(nodeURL, apiKey string, height int) ([]string, error) {
	var res blockFlagsResp
	if err := apiGet(nodeURL, apiKey, fmt.Sprintf("/api/Block/%d", height), &res); err != nil {
		return nil, err
	}
	return res.Result.Flags, nil
}

func fetchAllTxs(nodeURL, apiKey string, height int) ([]tx, error) {
	var all []tx
	cont := ""
	for {
		path := fmt.Sprintf("/api/Block/%d/Txs?limit=100", height)
		if cont != "" {
			path += "&continuationToken=" + cont
		}
		var res struct {
			Result       []tx   `json:"result"`
			Continuation string `json:"continuationToken"`
		}
		if err := apiGet(nodeURL, apiKey, path, &res); err != nil {
			return all, err
		}
		// The API returns "result": null for empty blocks (and rarely
		// "result": []), so len(Result) indicates whether the block
		// actually contains transactions.
		if len(res.Result) > 0 {
			all = append(all, res.Result...)
		}
		if res.Continuation == "" {
			break
		}
		cont = res.Continuation
		time.Sleep(100 * time.Millisecond)
	}
	return all, nil
}

func findShortSessionBlock(nodeURL, apiKey string, start int) (int, error) {
	for h := start; h < start+20; h++ {
		flags, err := getBlockFlags(nodeURL, apiKey, h)
		if err != nil {
			continue
		}
		for _, f := range flags {
			if f == "ShortSessionStarted" {
				return h, nil
			}
		}
	}
	return 0, fmt.Errorf("ShortSessionStarted not found")
}

func collectAddresses(nodeURL, apiKey string, start int) ([]string, error) {
	unique := make(map[string]struct{})
	blocks := 0
	h := start
	for blocks < requiredBlocks {
		txs, err := fetchAllTxs(nodeURL, apiKey, h)
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
	list := make([]string, 0, len(unique))
	for a := range unique {
		list = append(list, a)
	}
	sort.Strings(list)
	return list, nil
}

func fetchBadAddresses(nodeURL, apiKey string, epoch int) (map[string]struct{}, error) {
	base := strings.TrimRight(nodeURL, "/")
	return checks.BadAuthors(base, apiKey, epoch)
}

func fetchValidationSummary(nodeURL, apiKey string, epoch int, addr string) (*validationSummary, error) {
	base := strings.TrimRight(nodeURL, "/")
	sum, err := checks.FetchValidationSummary(base, apiKey, epoch, addr)
	if err != nil {
		return nil, err
	}
	return &validationSummary{
		State:     sum.State,
		Stake:     sum.Stake,
		Approved:  sum.Approved,
		Penalized: sum.Penalized,
	}, nil
}

func saveLines(path string, lines []string) error {
	data := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(path, []byte(data), 0644)
}

func main() {
	nodeURL := flag.String("node", defaultNodeURL, "Idena node RPC base URL")
	apiKey := flag.String("key", "", "Idena node API key")
	auto := flag.Bool("auto-addresses", false, "discover addresses from recent blocks")
	flag.Parse()

	epoch, threshold, err := getLatestEpochInfo(*nodeURL, *apiKey)
	if err != nil {
		log.Fatalf("latest epoch: %v", err)
	}
	if err := os.WriteFile(thresholdOutFile, []byte(fmt.Sprintf("%f", threshold)), 0644); err != nil {
		log.Printf("write threshold: %v", err)
	}

	var addresses []string
	if *auto {
		lastEpoch := epoch - 1
		firstBlock, err := getEpochInfo(*nodeURL, *apiKey, lastEpoch)
		if err != nil {
			log.Fatalf("epoch info: %v", err)
		}
		ssStart, err := findShortSessionBlock(*nodeURL, *apiKey, firstBlock+15)
		if err != nil {
			log.Fatalf("find short session: %v", err)
		}
		addresses, err = collectAddresses(*nodeURL, *apiKey, ssStart)
		if err != nil {
			log.Fatalf("collect addresses: %v", err)
		}
		if err := saveLines(addressFile, addresses); err != nil {
			log.Printf("write addresses: %v", err)
		}
	} else {
		addresses, err = agents.LoadAddressList(addressFile)
		if err != nil {
			log.Fatalf("load addresses: %v", err)
		}
	}

	lastEpoch := epoch - 1

	out, err := os.Create(outFile)
	if err != nil {
		log.Fatalf("open output: %v", err)
	}
	defer out.Close()

	included := 0
	for i, addr := range addresses {
		addrL := strings.ToLower(addr)
		pen, flip, err := checks.CheckPenaltyFlipForEpoch(*nodeURL, *apiKey, lastEpoch, addrL)
		if err != nil {
			log.Printf("[%d/%d] check %s: %v", i+1, len(addresses), addr, err)
			continue
		}
		sum, err := fetchValidationSummary(*nodeURL, *apiKey, lastEpoch, addrL)
		if err != nil {
			log.Printf("[%d/%d] summary %s: %v", i+1, len(addresses), addr, err)
			continue
		}
		stake, _ := strconv.ParseFloat(sum.Stake, 64)
		if !eligibility.IsEligibleFull(sum.State, stake, pen, flip, threshold) {
			log.Printf("[%d/%d] EXCLUDED %s state=%s stake=%.2f penalized=%v flip=%v", i+1, len(addresses), addr, sum.State, stake, pen, flip)
			continue
		}
		rec := map[string]interface{}{"address": addr, "state": sum.State, "stake": stake}
		b, _ := json.Marshal(rec)
		out.Write(b)
		out.Write([]byte("\n"))
		included++
		log.Printf("[%d/%d] OK %s state=%s stake=%.4f", i+1, len(addresses), addr, sum.State, stake)
		time.Sleep(200 * time.Millisecond)
	}
	log.Printf("Done. Whitelisted: %d addresses", included)
}
