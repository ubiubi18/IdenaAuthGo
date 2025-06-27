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
)

func hasFlag(flags []string, f string) bool {
	for _, v := range flags {
		if v == f {
			return true
		}
	}
	return false
}

func getThreshold(nodeURL, apiKey string, epoch int) (float64, error) {
	url := fmt.Sprintf("%s/api/Epoch/%d", strings.TrimRight(nodeURL, "/"), epoch)
	if apiKey != "" {
		url += "?apikey=" + apiKey
	}
	resp, err := http.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	var out struct {
		Result struct {
			Threshold float64 `json:"discriminationStakeThreshold"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, err
	}
	return out.Result.Threshold, nil
}

func main() {
	input := flag.String("input", "accounts.json", "input JSON file with account info")
	output := flag.String("output", "whitelist.json", "output whitelist file")
	nodeURL := flag.String("node", "http://localhost:9009", "Idena node RPC base URL")
	apiKey := flag.String("key", "", "Idena node API key")
	jsonl := flag.Bool("jsonl", false, "output JSONL instead of JSON array")
	flag.Parse()

	data, err := os.ReadFile(*input)
	if err != nil {
		log.Fatalf("read input: %v", err)
	}
	var infos []agents.AccountInfo
	if err := json.Unmarshal(data, &infos); err != nil {
		log.Fatalf("decode input: %v", err)
	}

	epoch, err := agents.GetCurrentEpoch(*nodeURL, *apiKey)
	if err != nil {
		log.Fatalf("get epoch: %v", err)
	}
	thr, err := getThreshold(*nodeURL, *apiKey, epoch)
	if err != nil {
		log.Fatalf("get threshold: %v", err)
	}

	var addrs []string
	for _, info := range infos {
		stake, _ := strconv.ParseFloat(info.Stake, 64)
		bal, _ := strconv.ParseFloat(info.Balance, 64)
		if hasFlag(info.LastValidationFlags, "AllFlipsNotQualified") {
			continue
		}
		if info.State == "Human" && stake < thr {
			continue
		}
		if (info.State == "Newbie" || info.State == "Verified") && stake+bal < 10000 {
			continue
		}
		addrs = append(addrs, strings.ToLower(info.Address))
	}
	sort.Strings(addrs)

	var out []byte
	if *jsonl {
		out = []byte(strings.Join(addrs, "\n") + "\n")
	} else {
		b, _ := json.MarshalIndent(addrs, "", "  ")
		out = b
	}
	if err := os.WriteFile(*output, out, 0644); err != nil {
		log.Fatalf("write output: %v", err)
	}
}
