package strictlocal

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// IdentityInfo holds minimal identity data for whitelist filtering
// Penalty comes as string in RPC response but is compared as is.
// lastValidationFlags may be nil.
type IdentityInfo struct {
	Address string   `json:"address"`
	Stake   float64  `json:"stake"`
	State   string   `json:"state"`
	Penalty string   `json:"penalty"`
	Flags   []string `json:"lastValidationFlags"`
}

// rpcCall performs a JSON-RPC request against the node.
// Example curl equivalent:
//
//	curl -X POST http://localhost:9009 -H 'Content-Type: application/json' \
//	  -d '{"method":"<method>","params":[],"id":1,"key":"$IDENA_RPC_KEY"}'
func rpcCall(nodeURL, apiKey, method string, params []interface{}, out interface{}) error {
	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
		"id":      1,
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

// filterIdentities applies the strict eligibility rules.
// It mirrors the logic of build_idena_identities_strict.py.
func filterIdentities(list []IdentityInfo, threshold float64) []IdentityInfo {
	var eligible []IdentityInfo
	for _, info := range list {
		if info.Penalty != "0" {
			continue
		}
		flagBad := false
		for _, f := range info.Flags {
			if f == "AtLeastOneFlipReported" {
				flagBad = true
				break
			}
		}
		if flagBad {
			continue
		}
		switch info.State {
		case "Human":
			if info.Stake >= threshold {
				eligible = append(eligible, info)
			}
		case "Newbie", "Verified":
			if info.Stake >= 10000 {
				eligible = append(eligible, info)
			}
		case "Undefined":
			// explicitly excluded
			continue
		}
	}
	sort.Slice(eligible, func(i, j int) bool { return eligible[i].Address < eligible[j].Address })
	return eligible
}

// BuildWhitelist fetches identity data for the given epoch and writes a JSONL whitelist.
// If dbPath is non-empty, identities are loaded from the indexer database using
// `SELECT address, state, stake FROM identity WHERE block_height = {startBlock}`.
// Otherwise dna_identities is used as fallback.
// BuildWhitelist fetches identities from the local node or an indexer snapshot
// and writes a strict whitelist to data/whitelist_epoch_<epoch>.jsonl.
// SQL reference if dbPath is provided:
//
//	SELECT address, state, stake FROM identity WHERE block_height = {startBlock};
func BuildWhitelist(nodeURL, apiKey, dbPath string) error {
	log.Println("starting snapshot build")
	var epochInfo struct {
		Result struct {
			StartBlock int `json:"startBlock"`
			Epoch      int `json:"epoch"`
		} `json:"result"`
	}
	if err := rpcCall(nodeURL, apiKey, "dna_epoch", nil, &epochInfo); err != nil {
		log.Printf("rpc dna_epoch error: %v", err)
		return err
	}
	startBlock := epochInfo.Result.StartBlock

	// Step: obtain address snapshot
	type basic struct {
		Address string `json:"address"`
		State   string `json:"state"`
		Stake   string `json:"stake"`
	}
	var basics []basic
	if dbPath != "" {
		log.Printf("loading snapshot from %s", dbPath)
		db, err := sql.Open("sqlite3", dbPath)
		if err == nil {
			rows, err2 := db.Query(`SELECT address, state, stake FROM identity WHERE block_height = ?`, startBlock)
			if err2 == nil {
				for rows.Next() {
					var addr, state string
					var stake float64
					if err := rows.Scan(&addr, &state, &stake); err == nil {
						basics = append(basics, basic{Address: addr, State: state, Stake: fmt.Sprintf("%f", stake)})
					}
				}
				rows.Close()
				db.Close()
			} else {
				log.Printf("sql query error: %v", err2)
			}
		} else {
			log.Printf("open db error: %v", err)
		}
	}
	if len(basics) == 0 {
		// fallback to live node
		var resp struct {
			Result []basic `json:"result"`
		}
		if err := rpcCall(nodeURL, apiKey, "dna_identities", nil, &resp); err != nil {
			log.Printf("rpc dna_identities error: %v", err)
			return err
		}
		basics = resp.Result
	}

	// Step: fetch global state for threshold
	var gs struct {
		Result struct {
			Threshold float64 `json:"discriminationStakeThreshold,string"`
		} `json:"result"`
	}
	if err := rpcCall(nodeURL, apiKey, "dna_globalState", nil, &gs); err != nil {
		log.Printf("rpc dna_globalState error: %v", err)
		return err
	}
	threshold := gs.Result.Threshold
	os.WriteFile("data/discriminationStakeThreshold.txt", []byte(fmt.Sprintf("%.8f", threshold)), 0644)

	// Step: fetch full identity info
	var list []IdentityInfo
	var mu sync.Mutex
	sem := make(chan struct{}, 5) // TODO: make concurrency configurable
	var wg sync.WaitGroup
	for _, b := range basics {
		addr := b.Address
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			var id struct {
				Result struct {
					Address string   `json:"address"`
					Stake   string   `json:"stake"`
					State   string   `json:"state"`
					Penalty string   `json:"penalty"`
					Flags   []string `json:"lastValidationFlags"`
				} `json:"result"`
			}
			if err := rpcCall(nodeURL, apiKey, "dna_identity", []interface{}{addr}, &id); err != nil {
				log.Printf("rpc dna_identity %s: %v", addr, err)
				return
			}
			stake, _ := strconv.ParseFloat(id.Result.Stake, 64)
			info := IdentityInfo{
				Address: strings.ToLower(id.Result.Address),
				Stake:   stake,
				State:   id.Result.State,
				Penalty: id.Result.Penalty,
				Flags:   id.Result.Flags,
			}
			mu.Lock()
			list = append(list, info)
			mu.Unlock()
		}()
	}
	wg.Wait()

	// Step: filtering
	eligible := filterIdentities(list, threshold)

	// Step: write JSONL
	outPath := fmt.Sprintf("data/whitelist_epoch_%d.jsonl", epochInfo.Result.Epoch)
	log.Printf("writing whitelist %s for epoch %d", outPath, epochInfo.Result.Epoch)
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, e := range eligible {
		if err := enc.Encode(e); err != nil {
			return err
		}
	}
	log.Printf("snapshot complete: %d eligible identities", len(eligible))
	return nil
}
