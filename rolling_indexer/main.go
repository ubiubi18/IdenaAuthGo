// Rolling Idena indexer
//
// Setup:
// 1. Create an `addresses.txt` file next to this program with one address per line.
// 2. Set environment variables `RPC_URL` (default http://localhost:9009) and `RPC_KEY` with your node API key.
// 3. Run `go run .` (or build with `go build`).
//
// The indexer stores identity information in `identities_30d.json` and keeps
// only the last 30 days of updates. It exposes a simple HTTP endpoint
// `http://localhost:8080/identities` returning the current records as JSON.
package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	addressesFile = "addresses.txt"
	dataFile      = "identities_30d.json"
	pollInterval  = 15 * time.Minute
	retentionDays = 30
)

// Identity holds the last known state for an address.
type Identity struct {
	Address   string    `json:"address"`
	State     string    `json:"state"`
	Stake     float64   `json:"stake"`
	UpdatedAt time.Time `json:"updated_at"`
}

var (
	rpcURL = getenv("RPC_URL", "http://localhost:9009")
	rpcKey = os.Getenv("RPC_KEY")

	mu         sync.Mutex
	identities = make(map[string]*Identity)
)

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// loadAddresses reads addresses.txt if present.
func loadAddresses() []string {
	data, err := os.ReadFile(addressesFile)
	if err != nil {
		return nil
	}
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// loadIdentities loads existing identity records from disk.
func loadIdentities() {
	data, err := os.ReadFile(dataFile)
	if err != nil {
		return
	}
	var list []Identity
	if err := json.Unmarshal(data, &list); err != nil {
		log.Printf("failed to parse %s: %v", dataFile, err)
		return
	}
	mu.Lock()
	defer mu.Unlock()
	for i := range list {
		rec := list[i]
		identities[rec.Address] = &rec
	}
}

// saveIdentities writes current records to disk.
func saveIdentities() {
	mu.Lock()
	var list []Identity
	for _, rec := range identities {
		list = append(list, *rec)
	}
	mu.Unlock()

	buf, _ := json.MarshalIndent(list, "", "  ")
	_ = ioutil.WriteFile(dataFile, buf, 0644)
}

// fetchIdentity queries the node for identity state and stake.
func fetchIdentity(addr string) (string, float64, error) {
	reqObj := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "dna_identity",
		"params":  []string{addr},
	}
	if rpcKey != "" {
		reqObj["key"] = rpcKey
	}
	body, _ := json.Marshal(reqObj)

	resp, err := http.Post(rpcURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	var rpcResp struct {
		Result struct {
			State string `json:"state"`
			Stake string `json:"stake"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return "", 0, err
	}
	stake, _ := strconv.ParseFloat(rpcResp.Result.Stake, 64)
	return rpcResp.Result.State, stake, nil
}

// pruneOld removes identities not updated within retentionDays.
func pruneOld(now time.Time) {
	cutoff := now.Add(-retentionDays * 24 * time.Hour)
	mu.Lock()
	for addr, rec := range identities {
		if rec.UpdatedAt.Before(cutoff) {
			delete(identities, addr)
			log.Printf("Pruned %s", addr)
		}
	}
	mu.Unlock()
}

func updateCycle() {
	addrs := loadAddresses()
	now := time.Now()
	mu.Lock()
	for _, a := range addrs {
		if _, ok := identities[a]; !ok {
			identities[a] = &Identity{Address: a}
		}
	}
	mu.Unlock()
	for _, addr := range addrs {
		state, stake, err := fetchIdentity(addr)
		if err != nil {
			log.Printf("Error fetching %s: %v", addr, err)
			continue
		}
		mu.Lock()
		rec := identities[addr]
		rec.State = state
		rec.Stake = stake
		rec.UpdatedAt = now
		mu.Unlock()
		log.Printf("Updated %s state=%s stake=%.3f", addr, state, stake)
	}
	pruneOld(now)
	saveIdentities()
}

func handleIdentities(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	var list []Identity
	for _, rec := range identities {
		list = append(list, *rec)
	}
	mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(list)
}

func main() {
	loadIdentities()
	go func() {
		for {
			updateCycle()
			time.Sleep(pollInterval)
		}
	}()

	http.HandleFunc("/identities", handleIdentities)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
